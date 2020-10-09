/*
 * Copyright 2019 the Velero contributors
 * SPDX-License-Identifier: Apache-2.0
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package snapshotmgr

import (
	"context"
	"encoding/base64"
	"fmt"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/backuprepository"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/constants"
	v1 "k8s.io/api/core/v1"
	"os"
	"strings"
	"time"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/vmware-tanzu/astrolabe/pkg/astrolabe"
	"github.com/vmware-tanzu/astrolabe/pkg/ivd"
	astrolabe_pvc "github.com/vmware-tanzu/astrolabe/pkg/pvc"
	"github.com/vmware-tanzu/astrolabe/pkg/server"
	backupdriverv1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/backupdriver/v1alpha1"
	v1api "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/datamover/v1alpha1"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/builder"
	plugin_clientset "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/paravirt"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"
	"k8s.io/utils/clock"
)

type SnapshotManager struct {
	logrus.FieldLogger
	config        map[string]string
	Pem           astrolabe.ProtectedEntityManager
	s3PETM        astrolabe.ProtectedEntityTypeManager
	clusterFlavor constants.ClusterFlavor
}

// TODO - remove in favor of NewSnapshotManagerFromConfig when callers have been converted
func NewSnapshotManagerFromCluster(params map[string]interface{}, config map[string]string, logger logrus.FieldLogger) (*SnapshotManager, error) {

	// Split incoming params out into configInfo and s3RepoParams

	s3RepoParams := make(map[string]interface{})
	ivdParams := make(map[string]interface{})

	_, isRegionExist := params["region"]
	if isRegionExist {
		s3RepoParams["region"] = params["region"]
		s3RepoParams["bucket"] = params["bucket"]
		s3RepoParams["s3ForcePathStyle"] = params["s3ForcePathStyle"]
		s3RepoParams["s3Url"] = params["s3Url"]
	}

	_, isVcHostExist := params[ivd.HostVcParamKey]
	if isVcHostExist {
		ivdParams[ivd.HostVcParamKey] = params[ivd.HostVcParamKey]
		ivdParams[ivd.UserVcParamKey] = params[ivd.UserVcParamKey]
		ivdParams[ivd.PasswordVcParamKey] = params[ivd.PasswordVcParamKey]
		ivdParams[ivd.PortVcParamKey] = params[ivd.PortVcParamKey]
		ivdParams[ivd.DatacenterVcParamKey] = params[ivd.DatacenterVcParamKey]
		ivdParams[ivd.InsecureFlagVcParamKey] = params[ivd.InsecureFlagVcParamKey]
		ivdParams[ivd.ClusterVcParamKey] = params[ivd.ClusterVcParamKey]
	}

	peConfigs := make(map[string]map[string]interface{})
	peConfigs["ivd"] = ivdParams // Even an empty map here will force NewSnapshotManagerFromConfig to use the default VC config
	// Initialize dummy s3 config.
	s3Config := astrolabe.S3Config{
		URLBase: "VOID_URL",
	}
	configInfo := server.NewConfigInfo(peConfigs, s3Config)

	return NewSnapshotManagerFromConfig(configInfo, s3RepoParams, config, nil, logger)
}

func NewSnapshotManagerFromConfig(configInfo server.ConfigInfo, s3RepoParams map[string]interface{},
	config map[string]string, k8sRestConfig *rest.Config, logger logrus.FieldLogger) (*SnapshotManager, error) {
	clusterFlavor, err := utils.GetClusterFlavor(k8sRestConfig)
	if err != nil {
		logger.Errorf("Failed to get cluster flavor")
		return nil, err
	}
	if clusterFlavor != constants.TkgGuest {
		if _, ok := configInfo.PEConfigs["ivd"]; !ok {
			configInfo.PEConfigs["ivd"] = make(map[string]interface{})
		}
	}

	// Retrieve VC configuration from the cluster only if it has not been set by the caller
	// An empty ivd map must be passed to use the IVD
	// TODO - Move this code out to the caller - this assumes we always want IVD
	if ivdParams, ok := configInfo.PEConfigs["ivd"]; ok {
		if _, ok := ivdParams[ivd.HostVcParamKey]; !ok {
			err := utils.RetrieveVcConfigSecret(ivdParams, k8sRestConfig, logger)
			if err != nil {
				logger.WithError(err).Errorf("Could not retrieve vsphere credential from k8s secret")
				return nil, err
			}
			logger.Infof("SnapshotManager: vSphere VC credential is retrieved")
		}
	}

	// TODO: Remove the use of s3RepoParams. Do not use BackupStorageLocation,
	//  as data movement will use BackupRepositories for each upload/download job
	// Check whether local mode is disabled or not
	isLocalMode := utils.GetBool(config[constants.VolumeSnapshotterLocalMode], false)
	initRemoteStorage := clusterFlavor == constants.VSphere

	// Initialize the DirectProtectedEntityManager
	dpem := server.NewDirectProtectedEntityManagerFromParamMap(configInfo, logger)

	snapMgr := SnapshotManager{
		FieldLogger:   logger,
		config:        config,
		Pem:           dpem,
		clusterFlavor: clusterFlavor,
		//s3PETM:      s3PETM,
	}
	logger.Infof("SnapshotManager is initialized with the configuration: %v", config)
	// if so, check whether there is any specification about remote storage location in config
	// otherwise, retrieve from velero BSLs.
	if !isLocalMode && initRemoteStorage {
		region, isRegionExist := config["region"]
		bucket, isBucketExist := config["bucket"]

		if isRegionExist && isBucketExist {
			s3RepoParams["region"] = region
			s3RepoParams["bucket"] = bucket
		} else {
			err = utils.RetrieveVSLFromVeleroBSLs(s3RepoParams, constants.DefaultS3BackupLocation, k8sRestConfig, logger)
			if err != nil {
				logger.WithError(err).Errorf("Could not retrieve velero default backup location")
				return nil, err
			}
		}

		logger.Infof("SnapshotManager: Velero Backup Storage Location is retrieved, region=%v, bucket=%v", s3RepoParams["region"], s3RepoParams["bucket"])

		intS3PETM, err := utils.GetS3PETMFromParamsMap(s3RepoParams, logger)
		if err != nil {
			logger.WithError(err).Errorf("Failed to get s3PETM from params map: region=%v, bucket=%v",
				s3RepoParams["region"], s3RepoParams["bucket"])
			return nil, err
		}
		// Wrapper the S3PETM to delegate copy/overwrite to data manager
		snapMgr.s3PETM = NewDataManagerProtectedEntityTypeManager(intS3PETM, &snapMgr, false)
		logger.Infof("SnapshotManager: Get s3PETM from the params map")
	}

	// Register external PETMs if any
	if clusterFlavor == constants.TkgGuest {
		logger.Infof("SnapshotManager: Cluster flavor: TKG Guest")
		paravirtParams := make(map[string]interface{})
		paravirtParams["restConfig"] = configInfo.PEConfigs[astrolabe.PvcPEType]["restConfig"]
		paravirtParams["entityType"] = paravirt.ParaVirtEntityTypePersistentVolume
		paravirtParams["svcConfig"] = configInfo.PEConfigs[astrolabe.PvcPEType]["svcConfig"]
		paravirtParams["svcNamespace"] = configInfo.PEConfigs[astrolabe.PvcPEType]["svcNamespace"]
		paraVirtPETM, err := paravirt.NewParaVirtProtectedEntityTypeManagerFromConfig(paravirtParams, configInfo.S3Config, logger)
		if err != nil {
			logger.Errorf("Failed to initialize paravirtualized PETM: %v", err)
			return nil, err
		}
		dpem.RegisterExternalProtectedEntityTypeManagers([]astrolabe.ProtectedEntityTypeManager{paraVirtPETM})
	}

	ivdPETM := dpem.GetProtectedEntityTypeManager("ivd")
	if ivdPETM != nil {
		dmIVDPE := NewDataManagerProtectedEntityTypeManager(ivdPETM, &snapMgr, true)
		// We use a DataManagerPE to delegate data copy to the data manager
		dpem.RegisterExternalProtectedEntityTypeManagers([]astrolabe.ProtectedEntityTypeManager{dmIVDPE})
	}

	logger.Infof("SnapshotManager is initialized with the configuration: %v", config)

	return &snapMgr, nil
}

func (this *SnapshotManager) CreateSnapshot(peID astrolabe.ProtectedEntityID, tags map[string]string) (astrolabe.ProtectedEntityID, error) {
	this.Infof("SnapshotManager.CreateSnapshot Called with peID %s, tags %v", peID.String(), tags)
	peID, _, err := this.createSnapshot(peID, tags, constants.WithoutBackupRepository, "", "")
	return peID, err
}

func (this *SnapshotManager) CreateSnapshotWithBackupRepository(peID astrolabe.ProtectedEntityID, tags map[string]string,
	backupRepositoryName string, snapshotRef string, backupName string) (astrolabe.ProtectedEntityID, string, error) {
	this.Infof("SnapshotManager.CreateSnapshotWithBackupRepository Called with peID %s, tags %v, BackupRepository %s",
		peID.String(), tags, backupRepositoryName)
	return this.createSnapshot(peID, tags, backupRepositoryName, snapshotRef, backupName)
}

func (this *SnapshotManager) createSnapshot(peID astrolabe.ProtectedEntityID, tags map[string]string,
	backupRepositoryName string, snapshotRef string, backupName string) (astrolabe.ProtectedEntityID, string, error) {
	this.Infof("Step 1: Creating a snapshot in local repository")
	var snapshotPEID astrolabe.ProtectedEntityID
	ctx := context.Background()
	pe, err := this.Pem.GetProtectedEntity(ctx, peID)
	if err != nil {
		this.WithError(err).Errorf("Failed to GetProtectedEntity for %s", peID.String())
		return astrolabe.ProtectedEntityID{}, "", err
	}

	// Snapshot params
	snapshotParams := make(map[string]map[string]interface{})
	guestSnapshotParams := make(map[string]interface{})
	// Pass the backup repository name as snapshot param.
	guestSnapshotParams[paravirt.SnapshotParamBackupRepository] = backupRepositoryName
	guestSnapshotParams[paravirt.SnapshotParamBackupName] = backupName

	snapshotParams[peID.GetPeType()] = guestSnapshotParams

	var peSnapID astrolabe.ProtectedEntitySnapshotID
	this.Infof("Ready to call astrolabe Snapshot API. Will retry on InvalidState error once per second for an hour at maximum")
	err = wait.PollImmediate(time.Second, time.Hour, func() (bool, error) {
		peSnapID, err = pe.Snapshot(ctx, snapshotParams)

		if err != nil {
			if strings.Contains(err.Error(), "The operation is not allowed in the current state") {
				this.Warnf("Keep retrying on InvalidState error")
				return false, nil
			} else {
				return false, err
			}
		}
		return true, nil
	})
	this.Infof("Return from the call of astrolabe Snapshot API for PE %s", peID.String())

	if err != nil {
		this.WithError(err).Errorf("Failed to Snapshot PE for %s", peID.String())
		return astrolabe.ProtectedEntityID{}, "", err
	}

	// Set the supervisor snapshot name is exists
	svcSnapshotName := ""
	if _, ok := guestSnapshotParams[paravirt.SnapshotParamSvcSnapshotName]; ok {
		svcSnapshotName = fmt.Sprintf("%v", guestSnapshotParams[paravirt.SnapshotParamSvcSnapshotName])
		this.Infof("Supervisor snapshot is created, %s", svcSnapshotName)
	}

	this.Infof("constructing the returned PE snapshot id, %s", peSnapID.GetID())
	snapshotPEID = peID.IDWithSnapshot(peSnapID)

	this.Infof("Local snapshot is created, %s", snapshotPEID.String())

	var isLocalMode bool
	if backupRepositoryName == constants.WithoutBackupRepository {
		// This occurs only if the volume snapshotter plugin is registered
		// In this scenario we explicitly read the feature flags to determine local mode.
		isLocalMode = utils.IsFeatureEnabled(constants.VSphereLocalModeFlag, false, this.FieldLogger)
	} else if backupRepositoryName == "" {
		// If the br name is not set its implied that its in local mode.
		isLocalMode = true
	}
	if isLocalMode {
		this.Infof("Skipping the remote copy as the local mode is inferred for Velero plugin for vSphere")
		return snapshotPEID, svcSnapshotName, nil
	}
	snapshotPE, err := this.Pem.GetProtectedEntity(ctx, snapshotPEID)
	_, err = this.UploadSnapshot(snapshotPE, ctx, backupRepositoryName, snapshotRef)
	if err != nil {
		return snapshotPEID, svcSnapshotName, err
	}

	return snapshotPEID, svcSnapshotName, nil
}

/*
Creates an Upload CR
*/
func (this *SnapshotManager) UploadSnapshot(uploadPE astrolabe.ProtectedEntity, ctx context.Context, backupRepositoryName string,
	snapshotRef string) (*v1api.Upload, error) {
	this.Info("Start creating Upload CR")
	config, err := rest.InClusterConfig()
	if err != nil {
		this.WithError(err).Errorf("Failed to get k8s inClusterConfig")
		return nil, err
	}
	pluginClient, err := plugin_clientset.NewForConfig(config)
	if err != nil {
		this.WithError(err).Errorf("Failed to get k8s clientset from the given config: %v ", config)
		return nil, err
	}

	// look up velero namespace from the env variable in container
	veleroNs, exist := os.LookupEnv("VELERO_NAMESPACE")
	if !exist {
		this.WithError(err).Errorf("CreateSnapshot: Failed to lookup the env variable for velero namespace")
		return nil, err
	}
	var uploadSnapshotPEID astrolabe.ProtectedEntityID

	// For PVC types, we do not upload the PVC itself, rather we upload what it points to.  This may be a ParaVirt PE
	// or an IVC currently
	if uploadPE.GetID().GetPeType() == astrolabe.PvcPEType {
		components, err := uploadPE.GetComponents(ctx)
		if err != nil {
			this.WithError(err).Errorf("Failed to retrieve subcomponents for %s", uploadPE.GetID().String())
			return nil, err
		}
		if len(components) != 1 {
			return nil, errors.New(fmt.Sprintf("Expected 1 component, %s has %d", uploadPE.GetID().String(), len(components)))
		}

		uploadSnapshotPEID = components[0].GetID()
		this.Infof("UploadSnapshot: componentPEID %s", uploadSnapshotPEID.String())

	} else {
		uploadSnapshotPEID = uploadPE.GetID()
	}

	uploadName, err := uploadCRNameForSnapshotPEID(uploadSnapshotPEID)
	if err != nil {
		this.WithError(err).Errorf("Failed to get uploadCR")
		return nil, err
	}
	this.Infof("Creating Upload CR: %s / %s", veleroNs, uploadName)
	uploadBuilder := builder.ForUpload(veleroNs, uploadName).
		BackupTimestamp(time.Now()).
		NextRetryTimestamp(time.Now()).
		Phase(v1api.UploadPhaseNew).
		SnapshotReference(snapshotRef).
		SnapshotID(uploadSnapshotPEID.String())

	if backupRepositoryName != "" {
		this.Infof("Create upload CR with backup repository %s", backupRepositoryName)
		uploadBuilder = uploadBuilder.BackupRepositoryName(backupRepositoryName)
	}
	upload := uploadBuilder.Result()
	this.Infof("Ready to create upload CR. Will retry on network issue every 5 seconds for 5 retries at maximum")
	var retUpload *v1api.Upload

	err = wait.PollImmediate(constants.RetryInterval*time.Second, constants.RetryInterval*constants.RetryMaximum*time.Second, func() (bool, error) {
		retUpload, err = pluginClient.DatamoverV1alpha1().Uploads(veleroNs).Create(context.TODO(), upload, metav1.CreateOptions{})
		if err != nil {
			return false, err
		}
		return true, nil
	})
	return retUpload, err
}

func (this *SnapshotManager) DeleteSnapshotWithBackupRepository(peID astrolabe.ProtectedEntityID, backupRepository string) error {
	this.WithField("peID", peID.String()).Info("SnapshotManager.DeleteSnapshotWithBackupRepository was called.")
	return this.deleteSnapshot(peID, backupRepository)
}

func (this *SnapshotManager) DeleteSnapshot(peID astrolabe.ProtectedEntityID) error {
	return this.deleteSnapshot(peID, constants.WithoutBackupRepository)
}

func (this *SnapshotManager) deleteSnapshot(peID astrolabe.ProtectedEntityID, backupRepositoryName string) error {
	log := this.WithField("peID", peID.String())
	log.Info("SnapshotManager.deleteSnapshot was called.")
	ctx := context.Background()
	config, err := rest.InClusterConfig()
	if err != nil {
		this.WithError(err).Errorf("Failed to get k8s inClusterConfig")
		return err
	}
	pluginClient, err := plugin_clientset.NewForConfig(config)
	if err != nil {
		this.WithError(err).Errorf("Failed to get k8s clientset from the given config: %v ", config)
		return err
	}
	clusterFlavor, err := utils.GetClusterFlavor(config)
	if err != nil {
		log.WithError(err).Errorf("Failed to determine cluster flavour")
		return err
	}
	if clusterFlavor == constants.Supervisor || clusterFlavor == constants.VSphere {
		log.Infof("Detected non-Guest Cluster, checking for on-going upload.")
		log.Infof("Step 0: Cancel on-going upload.")

		// look up velero namespace from the env variable in container
		veleroNs, exist := os.LookupEnv("VELERO_NAMESPACE")
		if !exist {
			log.WithError(err).Errorf("DeleteSnapshot: Failed to lookup the env variable for velero namespace")
			return err
		}
		snapIDDecoded, err := decodeSnapshotID(peID.GetSnapshotID(), this.FieldLogger)
		if err != nil {
			log.WithError(err).Errorf("Failed to retrieve decoded snapshot id to search for on-going uploads, original pe-id :%s", peID.String())
			return err
		}
		uploadName := "upload-" + snapIDDecoded
		log.Infof("Searching for Upload CR: %s", uploadName)
		uploadCR, err := pluginClient.DatamoverV1alpha1().Uploads(veleroNs).Get(context.TODO(), uploadName, metav1.GetOptions{})
		if err != nil {
			log.WithError(err).Errorf(" Error while retrieving the upload CR %v", uploadName)
			if k8serrors.IsNotFound(err) {
				log.Errorf("The upload CR: %s was not found, assuming terminal stage", uploadName)
			}
			uploadCR = nil
		}
		uploadCompleted := this.isTerminalState(uploadCR)
		if uploadCR != nil && !uploadCompleted {
			log.Infof("Found the Upload CR: %v, updating spec to indicate cancel upload.", uploadName)
			timeNow := clock.RealClock{}
			mutate := func(r *v1api.Upload) {
				uploadCR.Spec.UploadCancel = true
				uploadCR.Status.StartTimestamp = &metav1.Time{Time: timeNow.Now()}
				uploadCR.Status.Message = "Canceling on going upload to repository."
			}
			_, err = utils.PatchUpload(uploadCR, mutate, pluginClient.DatamoverV1alpha1().Uploads(veleroNs), log)
			if err != nil {
				log.WithError(err).Error("Failed to patch ongoing Upload")
				return err
			} else {
				log.Infof("Upload status updated to UploadCancel")
			}
			return nil
		} else {
			log.Infof("The upload %v was in terminal stage, proceeding with snapshot deletes", uploadName)
		}
	}
	pe, err := this.Pem.GetProtectedEntity(ctx, peID)
	if err != nil {
		log.WithError(err).Errorf("Failed to GetProtectedEntity for %s", peID.String())
		return err
	}
	// Snapshot params
	snapshotParams := make(map[string]map[string]interface{})
	guestSnapshotParams := make(map[string]interface{})
	// Pass the backup repository name as snapshot param.
	guestSnapshotParams["BackupRepositoryName"] = backupRepositoryName
	snapshotParams[peID.GetPeType()] = guestSnapshotParams
	log.Infof("Step 1: Deleting the local snapshot")
	delSnapshotStatus, err := pe.DeleteSnapshot(ctx, pe.GetID().GetSnapshotID(), snapshotParams)
	if err != nil {
		log.WithError(err).Errorf("Failed to Delete Local Snapshot for %s", peID.String())
		return err
	}
	if !delSnapshotStatus {
		log.Infof("No local snapshots found for the PE: %s", pe.GetID().String())
	} else {
		log.Infof("Successfully deleted local snapshot for PE: %s", pe.GetID().String())
	}
	if clusterFlavor == constants.TkgGuest {
		log.Infof("Guest Cluster detected during delete snapshot, nothing more to do.")
	}
	// Trigger remote snapshot deletion in Supervisor/Vanilla setup.
	var isLocalMode bool
	if backupRepositoryName == constants.WithoutBackupRepository {
		// This occurs only if the volume snapshotter plugin is registered
		// In this scenario we explicitly read the feature flags to determine local mode.
		isLocalMode = utils.IsFeatureEnabled(constants.VSphereLocalModeFlag, false, this.FieldLogger)
	} else if backupRepositoryName == "" {
		// If the br name is not set its implied that its in local mode.
		isLocalMode = true
	}
	if isLocalMode {
		log.Infof("Local Mode detected, skipping remote delete snapshot for PE: %s", peID)
		return nil
	}
	// Retrieve the component pe-id
	components, err := pe.GetComponents(ctx)
	if err != nil {
		return errors.Wrap(err, "Could not retrieve components")
	}
	if len(components) != 1 {
		return errors.New(fmt.Sprintf("Expected 1 component, %s has %d", pe.GetID().String(), len(components)))
	}
	log.Infof("Retrieved components to delete remote snapshot component ID: %s", components[0].GetID().String())

	log.Infof("Step 2: Deleting the durable snapshot from s3")
	if backupRepositoryName != "" && backupRepositoryName != constants.WithoutBackupRepository {
		backupRepositoryCR, err := pluginClient.BackupdriverV1alpha1().BackupRepositories().Get(context.TODO(), backupRepositoryName, metav1.GetOptions{})
		if err != nil {
			log.WithError(err).Errorf("Error while retrieving the backup repository CR %v", backupRepositoryName)
			return err
		}
		err = this.DeleteRemoteSnapshotFromRepo(components[0].GetID(), backupRepositoryCR)
	} else {
		err = this.DeleteRemoteSnapshot(components[0].GetID())
	}
	if err != nil {
		log.WithError(err).Errorf("Failed to delete the durable snapshot for PEID")
		return err
	}
	log.Infof("Deleted the durable snapshot from the durable repository")
	log.Infof("Finished processing the DeleteSnapshot request.")
	return nil
}

func (this *SnapshotManager) isTerminalState(uploadCR *v1api.Upload) bool {
	if uploadCR == nil {
		return true
	}
	return uploadCR.Status.Phase == v1api.UploadPhaseCompleted || uploadCR.Status.Phase == v1api.UploadPhaseCleanupFailed || uploadCR.Status.Phase == v1api.UploadPhaseCanceled
}

func (this *SnapshotManager) DeleteLocalSnapshot(peID astrolabe.ProtectedEntityID) error {
	this.WithField("peID", peID.String()).Infof("SnapshotManager.deleteLocalSnapshot Called")
	return this.deleteSnapshotFromRepo(peID, this.Pem.GetProtectedEntityTypeManager(peID.GetPeType()))
}

// Will be deleted eventually. Use S3PETM created from backup repository instead of hard coding S3PETM which is initialized when NewSnapshotManagerFromCluster
// Eventually will use DeleteRemoteSnapshotFromRepo instead of this
func (this *SnapshotManager) DeleteRemoteSnapshot(peID astrolabe.ProtectedEntityID) error {
	this.WithField("peID", peID.String()).Infof("SnapshotManager.deleteRemoteSnapshot Called")
	var s3PETM astrolabe.ProtectedEntityTypeManager
	logger := this.FieldLogger
	s3PETM, err := utils.GetDefaultS3PETM(logger)
	if err != nil {
		logger.Errorf("DeleteRemoteSnapshot: Failed to get Default S3 repository")
		return err
	}
	return this.deleteSnapshotFromRepo(peID, s3PETM)
}

func (this *SnapshotManager) DeleteRemoteSnapshotFromRepo(peID astrolabe.ProtectedEntityID, backupRepository *backupdriverv1.BackupRepository) error {
	this.WithField("peID", peID.String()).Infof("SnapshotManager.deleteRemoteSnapshotFromRepo Called")
	s3PETM, err := backuprepository.GetRepositoryFromBackupRepository(backupRepository, this.FieldLogger)
	if err != nil {
		this.WithError(err).Errorf("Failed to create s3PETM from backup repository %s", backupRepository.Name)
	}
	return this.deleteSnapshotFromRepo(peID, s3PETM)
}

func (this *SnapshotManager) deleteSnapshotFromRepo(peID astrolabe.ProtectedEntityID, petm astrolabe.ProtectedEntityTypeManager) error {
	this.WithField("peID", peID.String()).Infof("SnapshotManager.deleteProtectedEntitySnapshot Called")

	if !peID.HasSnapshot() {
		this.Errorf("No snapshot is associated with PE %s", peID.String())
		return nil
	}

	ctx := context.Background()
	pe, err := petm.GetProtectedEntity(ctx, peID)
	if err != nil {
		this.WithError(err).Errorf("Failed to get the ProtectedEntity from peID %s", peID.String())
		return err
	}
	snapshotIds, err := pe.ListSnapshots(ctx)
	if err != nil {
		return errors.Wrap(err, "Failed to list snapshots")
	}

	if len(snapshotIds) == 0 {
		this.Warningf("There are no snapshots for PE %s from the perspective of ProtectedEntityTypeManager. Skipping the deleteSnapshot operation.", pe.GetID().String())
		return nil
	}

	this.Infof("Ready to call astrolabe DeleteSnapshot API. Will retry on InvalidState error once per second for an hour at maximum")
	err = wait.PollImmediate(time.Second, time.Hour, func() (bool, error) {
		_, err = pe.DeleteSnapshot(ctx, peID.GetSnapshotID(), make(map[string]map[string]interface{}))

		if err != nil {
			if strings.Contains(err.Error(), "The operation is not allowed in the current state") {
				this.Warnf("Keep retrying on InvalidState error")
				return false, nil
			} else {
				return false, err
			}
		}
		return true, nil
	})
	this.Infof("Return from the call of astrolabe DeleteSnapshot API for snapshot %s", peID.GetSnapshotID().String())

	if err != nil {
		this.WithError(err).Errorf("Failed to delete the snapshot %s", peID.GetSnapshotID().String())
		return err
	}

	return nil
}

const PollLogInterval = time.Minute

func (this *SnapshotManager) CreateVolumeFromSnapshot(sourcePEID astrolabe.ProtectedEntityID, destinationPEID astrolabe.ProtectedEntityID, params map[string]map[string]interface{}) (updatedID astrolabe.ProtectedEntityID, err error) {
	this.Infof("Start creating Download CR for %s", sourcePEID.String())
	config, err := rest.InClusterConfig()
	if err != nil {
		this.WithError(err).Errorf("Failed to get k8s inClusterConfig")
		return
	}
	pluginClient, err := plugin_clientset.NewForConfig(config)
	if err != nil {
		this.WithError(err).Errorf("Failed to get k8s clientset with the given config: %v", config)
		return
	}

	veleroNs, exist := os.LookupEnv("VELERO_NAMESPACE")
	if !exist {
		this.Errorf("CreateVolumeFromSnapshot: Failed to lookup the env variable for velero namespace")
		return
	}

	uuID, err := uuid.NewRandom()
	if err != nil {
		this.Errorf("CreateVolumeFromSnapshot: Failed to generate random UUID.")
		return
	}
	var cloneFromSnapshotNamespaceExists, cloneFromSnapshotNameExists bool
	cloneParams := params["CloneFromSnapshotReference"]
	cloneFromSnapshotNamespace, cloneFromSnapshotNamespaceExists := cloneParams["CloneFromSnapshotNamespace"].(string)
	cloneFromSnapshotName, cloneFromSnapshotNameExists := cloneParams["CloneFromSnapshotName"].(string)
	backupRepositoryName, ok := cloneParams["BackupRepositoryName"].(string)
	if !ok {
		backupRepositoryName = constants.WithoutBackupRepository
	}
	cloneRef := fmt.Sprintf("%s/%s", cloneFromSnapshotNamespace, cloneFromSnapshotName)
	this.Infof("CloneFromSnapshotReference: %s", cloneRef)
	downloadRecordName := "download-" + sourcePEID.GetSnapshotID().GetID() + "-" + uuID.String()
	this.Infof("Creating Download CR: %s/%s", veleroNs, downloadRecordName)
	downloadBuilder := builder.ForDownload(veleroNs, downloadRecordName).
		RestoreTimestamp(time.Now()).
		NextRetryTimestamp(time.Now()).
		SnapshotID(sourcePEID.String()).
		Phase(v1api.DownloadPhaseNew).
		CloneFromSnapshotReference(cloneRef).
		BackupRepositoryName(backupRepositoryName)
	if destinationPEID != (astrolabe.ProtectedEntityID{}) {
		downloadBuilder = downloadBuilder.ProtectedEntityID(destinationPEID.String())
	}
	download := downloadBuilder.Result()
	this.Infof("Ready to create download CR. Will retry on network issue every 5 seconds for 5 retries at maximum")
	err = wait.PollImmediate(constants.RetryInterval*time.Second, constants.RetryInterval*constants.RetryMaximum*time.Second, func() (bool, error) {
		_, err = pluginClient.DatamoverV1alpha1().Downloads(veleroNs).Create(context.TODO(), download, metav1.CreateOptions{})
		if err != nil {
			return false, nil
		}
		return true, nil
	})

	if err != nil {
		this.WithError(err).Errorf("CreateVolumeFromSnapshot: Failed to create Download CR for %s", sourcePEID.String())
		return
	}

	lastPollLogTime := time.Now()
	// TODO: Suitable length of timeout
	err = wait.PollImmediateInfinite(time.Second, func() (bool, error) {
		infoLog := false
		if time.Now().Sub(lastPollLogTime) > PollLogInterval {
			infoLog = true
			this.Infof("Polling download record %s", downloadRecordName)
			lastPollLogTime = time.Now()
		}
		download, err = pluginClient.DatamoverV1alpha1().Downloads(veleroNs).Get(context.TODO(), downloadRecordName, metav1.GetOptions{})
		if err != nil {
			this.Errorf("Retrieve download record %s failed with err %v", downloadRecordName, err)
			return false, errors.Wrapf(err, "Failed to retrieve download record %s", downloadRecordName)
		}
		if download.Status.Phase == v1api.DownloadPhaseCompleted {
			this.Infof("Download record %s completed", downloadRecordName)
			return true, nil
		} else if download.Status.Phase == v1api.DownloadPhaseFailed {
			return false, errors.Errorf("Create download cr failed.")
		} else {
			if infoLog {
				this.Infof("Retrieve phase %s for download record %s", download.Status.Phase, downloadRecordName)
			}
			return false, nil
		}
	})
	if err != nil {
		return
	}
	updatedID, err = astrolabe.NewProtectedEntityIDFromString(download.Status.VolumeID)
	if err != nil {
		this.WithError(err).Errorf("Failed to create PE-ID from %s volumeID", download.Status.VolumeID)
		return
	}

	if cloneFromSnapshotNameExists && cloneFromSnapshotNamespaceExists {
		// TODO(xyang): Watch for Download status and update CloneFromSnapshot status accordingly in Backupdriver
		cloneFromSnap, err := pluginClient.BackupdriverV1alpha1().CloneFromSnapshots(cloneFromSnapshotNamespace).Get(context.TODO(), cloneFromSnapshotName, metav1.GetOptions{})
		if err != nil {
			this.WithError(err).Errorf("CreateVolumeFromSnapshot: Failed to get CloneFromSnapshot %s/%s", cloneFromSnapshotNamespace, cloneFromSnapshotName)
			return updatedID, err
		}
		clone := cloneFromSnap.DeepCopy()
		// Since we wait until download is complete here,
		// download.Status.Phase should be up to date.
		clone.Status.Phase = backupdriverv1.ClonePhase(download.Status.Phase)
		clone.Status.Message = download.Status.Message
		apiGroup := ""
		clone.Status.ResourceHandle = &v1.TypedLocalObjectReference{
			APIGroup: &apiGroup,
			Kind:     "PersistentVolumeClaim",
			Name:     download.Status.VolumeID,
		}

		_, err = pluginClient.BackupdriverV1alpha1().CloneFromSnapshots(cloneFromSnapshotNamespace).UpdateStatus(context.TODO(), clone, metav1.UpdateOptions{})
		if err != nil {
			this.WithError(err).Errorf("CreateVolumeFromSnapshot: Failed to update status of CloneFromSnapshot %s/%s to %v", cloneFromSnapshotNamespace, cloneFromSnapshotName, clone.Status.Phase)
			return updatedID, err
		}
	}
	return
}

func (this *SnapshotManager) CreateVolumeFromSnapshotWithMetadata(peID astrolabe.ProtectedEntityID, metadata []byte,
	snapshotIDStr string, backupRepositoryName string, cloneFromSnapshotNamespace string, cloneFromSnapshotName string) (astrolabe.ProtectedEntityID, error) {
	this.Infof("CreateVolumeFromSnapshotWithMetadata: Start creating restore for %s, snapshot ID %s, backupRepositoryName %s, cloneFromSnapshot %s/%s", peID.String(), snapshotIDStr, backupRepositoryName, cloneFromSnapshotNamespace, cloneFromSnapshotName)
	snapshotID, err := astrolabe.NewProtectedEntityIDFromString(snapshotIDStr)
	if err != nil {
		this.WithError(err).Errorf("Error creating volume from metadata")
		return astrolabe.ProtectedEntityID{}, errors.Wrap(err, "Error creating volume from metadata")
	}
	this.Infof("CreateVolumeFromSnapshotWithMetadata: snapshot ID %s", snapshotID.String())

	var snapshotRepo astrolabe.ProtectedEntityTypeManager
	ctx := context.Background()
	if this.clusterFlavor != constants.TkgGuest {
		this.Infof("CreateVolumeFromSnapshotWithMetadata: not a TKG Guest Cluster")
		if backupRepositoryName != "" && backupRepositoryName != constants.WithoutBackupRepository {
			backupRepository, err := backuprepository.GetBackupRepositoryFromBackupRepositoryName(backupRepositoryName)
			if err != nil {
				this.WithError(err).Errorf("Failed to retrieve backup repository %s", backupRepositoryName)
				return astrolabe.ProtectedEntityID{}, err
			}
			snapshotRepo, err = backuprepository.GetRepositoryFromBackupRepository(backupRepository, this)
			if err != nil {
				this.WithError(err).Errorf("Failed to get S3 repository from backup repository %s", backupRepository.Name)
				return astrolabe.ProtectedEntityID{}, err
			}

			peTM := this.Pem.GetProtectedEntityTypeManager(peID.GetPeType())
			pvcPETM := peTM.(*astrolabe_pvc.PVCProtectedEntityTypeManager)

			this.Infof("Ready to call astrolabe CreateFromMetadata API: snapshot ID %s", snapshotID.String())
			pe, err := pvcPETM.CreateFromMetadata(ctx, metadata, snapshotID, snapshotRepo, cloneFromSnapshotNamespace, cloneFromSnapshotName, backupRepositoryName)
			if err != nil {
				this.WithError(err).Errorf("Error creating volume from metadata")
				return astrolabe.ProtectedEntityID{}, errors.Wrap(err, "Error creating volume from metadata")
			}
			this.Infof("CreateVolumeFromSnapshotWithMetadata: PE returned by CreateFromMetadata: %s", pe.GetID().String())
			return pe.GetID(), err
		} else {
			errMsg := "BackupRepository unset during restore, local mode set during restore is unsupported."
			this.Errorf(errMsg)
			return astrolabe.ProtectedEntityID{}, errors.Errorf(errMsg)
		}
	}

	peTM := this.Pem.GetProtectedEntityTypeManager("paravirt-pv")
	paravirtPETM := peTM.(*paravirt.ParaVirtProtectedEntityTypeManager)
	this.Infof("CreateVolumeFromSnapshotWithMetadata: TKG Guest Cluster")

	this.Infof("Ready to call astrolabe CreateFromMetadata API: snapshot ID %s", snapshotID.String())
	pe, err := paravirtPETM.CreateFromMetadata(ctx, metadata, snapshotID, snapshotRepo, cloneFromSnapshotNamespace, cloneFromSnapshotName, backupRepositoryName)
	if err != nil {
		this.WithError(err).Errorf("Failed to CreateFromMetadata for PE %s", peID.String())
		return astrolabe.ProtectedEntityID{}, errors.Wrap(err, "Error creating volume from metadata")
	}
	this.Infof("Return from the call of astrolabe CreateFromMetadata API for PE %s", peID.String())
	this.Infof("CreateVolumeFromSnapshotWithMetadata: PE returned by CreateFromMetadata: %s", pe.GetID().String())

	return pe.GetID(), err
}

func uploadCRNameForSnapshotPEID(snapshotPEID astrolabe.ProtectedEntityID) (string, error) {
	if !snapshotPEID.HasSnapshot() {
		return "", errors.New(fmt.Sprintf("snapshotPEID %s does not have a snapshot ID", snapshotPEID.String()))
	}
	return "upload-" + snapshotPEID.GetSnapshotID().String(), nil
}

func decodeSnapshotID(snapshotID astrolabe.ProtectedEntitySnapshotID, logger logrus.FieldLogger) (string, error) {
	// Decode incoming snapshot ID until we retrieve the ivd snapshot-id.
	snapshotID64Str := snapshotID.String()
	snapshotIDBytes, err := base64.RawStdEncoding.DecodeString(snapshotID64Str)
	if err != nil {
		errorMsg := fmt.Sprintf("Could not decode snapshot ID encoded string %s", snapshotID64Str)
		logger.WithError(err).Error(errorMsg)
		return "", errors.Wrap(err, errorMsg)
	}
	decodedPEID, err := astrolabe.NewProtectedEntityIDFromString(string(snapshotIDBytes))
	if err != nil {
		errorMsg := fmt.Sprintf("Could not translate decoded snapshotID %s into pe-id", string(snapshotIDBytes))
		logger.WithError(err).Error(errorMsg)
		return "", errors.Wrap(err, errorMsg)
	}
	logger.Infof("Successfully translated snapshotID %s into pe-id: %s", string(snapshotIDBytes), decodedPEID.String())
	if decodedPEID.HasSnapshot() && decodedPEID.GetPeType() != "ivd" {
		logger.Infof("The translated pe-id is not ivd type, recurring for further decode")
		return decodeSnapshotID(decodedPEID.GetSnapshotID(), logger)
	}
	return decodedPEID.GetSnapshotID().String(), nil
}
