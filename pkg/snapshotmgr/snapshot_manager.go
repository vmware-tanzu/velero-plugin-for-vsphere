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
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/vmware-tanzu/astrolabe/pkg/astrolabe"
	"github.com/vmware-tanzu/astrolabe/pkg/ivd"
	"github.com/vmware-tanzu/astrolabe/pkg/s3repository"
	v1api "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/veleroplugin/v1"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/builder"
	plugin_clientset "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"
	"k8s.io/utils/clock"
	"os"
	"strings"
	"time"

)

type SnapshotManager struct {
	logrus.FieldLogger
	config  map[string]string
	ivdPETM *ivd.IVDProtectedEntityTypeManager
	s3PETM  *s3repository.ProtectedEntityTypeManager
}

func NewSnapshotManagerFromCluster(params map[string]interface{}, config map[string]string, logger logrus.FieldLogger) (*SnapshotManager, error) {
	// Retrieve VC configuration from the cluster only of it has not been passed by the caller
	if _, ok := params[ivd.HostVcParamKey]; !ok {
		err := utils.RetrieveVcConfigSecret(params, logger)
		if err != nil {
			logger.WithError(err).Errorf("Could not retrieve vsphere credential from k8s secret")
			return nil, err
		}
		logger.Infof("SnapshotManager: vSphere VC credential is retrieved")
	}

	var s3PETM *s3repository.ProtectedEntityTypeManager
	var ivdPETM *ivd.IVDProtectedEntityTypeManager
	var err error

	// firstly, check whether local mode is disabled or not.
	isLocalMode := utils.GetBool(config[utils.VolumeSnapshotterLocalMode], false)

	// if so, check whether there is any specification about remote storage location in config
	// otherwise, retrieve from velero BSLs.
	if !isLocalMode {
		region, isRegionExist := config["region"]
		bucket, isBucketExist := config["bucket"]

		if isRegionExist && isBucketExist {
			params["region"] = region
			params["bucket"] = bucket
		} else {
			err = utils.RetrieveVSLFromVeleroBSLs(params, logger)
			if err != nil {
				logger.WithError(err).Errorf("Could not retrieve velero default backup location")
				return nil, err
			}
		}

		logger.Infof("SnapshotManager: Velero Backup Storage Location is retrieved, region=%v, bucket=%v", params["region"], params["bucket"])

		s3PETM, err = utils.GetS3PETMFromParamsMap(params, logger)
		if err != nil {
			logger.WithError(err).Errorf("Failed to get s3PETM from params map: region=%v, bucket=%v",
				params["region"], params["bucket"])
			return nil, err
		}
		logger.Infof("SnapshotManager: Get s3PETM from the params map")
	}

	// If the local mode is enabled, we don't need to specify remote storage location
	// for persistence since it has been taken care of 3rd party backup solution.
	ivdPETM, err = utils.GetIVDPETMFromParamsMap(params, logger)
	if err != nil {
		logger.WithError(err).Errorf("Failed to get ivdPETM from params map: VirtualCenter=%v, port=%v",
			params["VirtualCenter"], params["port"])
		return nil, err
	}
	logger.Infof("SnapshotManager: Get ivdPETM from the params map, VirtualCenter=%v, port=%v", params["VirtualCenter"], params["port"])

	snapMgr := SnapshotManager{
		FieldLogger: logger,
		config:      config,
		ivdPETM:     ivdPETM,
		s3PETM:      s3PETM,
	}
	logger.Infof("SnapshotManager is initialized with the configuration: %v", config)

	return &snapMgr, nil
}

func (this *SnapshotManager) CreateSnapshot(peID astrolabe.ProtectedEntityID, tags map[string]string) (astrolabe.ProtectedEntityID, error) {
	this.Infof("SnapshotManager.CreateSnapshot Called with peID %s, tags %v", peID.String(), tags)
	this.Infof("Step 1: Creating a snapshot in local repository")
	var updatedPeID astrolabe.ProtectedEntityID
	ctx := context.Background()
	pe, err := this.ivdPETM.GetProtectedEntity(ctx, peID)
	if err != nil {
		this.WithError(err).Errorf("Failed to GetProtectedEntity for %s", peID.String())
		return astrolabe.ProtectedEntityID{}, err
	}

	var peSnapID astrolabe.ProtectedEntitySnapshotID
	this.Infof("Ready to call astrolabe Snapshot API. Will retry on InvalidState error once per second for an hour at maximum")
	err = wait.PollImmediate(time.Second, time.Hour, func() (bool, error) {
		peSnapID, err = pe.Snapshot(ctx)

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
	this.Debugf("Return from the call of astrolabe Snapshot API for PE %s", peID.String())

	if err != nil {
		this.WithError(err).Errorf("Failed to Snapshot PE for %s", peID.String())
		return astrolabe.ProtectedEntityID{}, err
	}

	this.Debugf("constructing the returned PE snapshot id, %s", peSnapID.GetID())
	updatedPeID = astrolabe.NewProtectedEntityIDWithSnapshotID(peID.GetPeType(), peID.GetID(), peSnapID)

	this.Infof("Local IVD snapshot is created, %s", updatedPeID.String())

	isLocalMode := utils.GetBool(this.config[utils.VolumeSnapshotterLocalMode], false)

	if isLocalMode {
		this.Infof("Skipping the remote copy in the local mode of Velero plugin for vSphere")
		return updatedPeID, nil
	}

	this.Info("Start creating Upload CR")
	config, err := rest.InClusterConfig()
	if err != nil {
		this.WithError(err).Errorf("Failed to get k8s inClusterConfig")
		return updatedPeID, err
	}
	pluginClient, err := plugin_clientset.NewForConfig(config)
	if err != nil {
		this.WithError(err).Errorf("Failed to get k8s clientset from the given config: %v ", config)
		return updatedPeID, err
	}

	// look up velero namespace from the env variable in container
	veleroNs, exist := os.LookupEnv("VELERO_NAMESPACE")
	if !exist {
		this.WithError(err).Errorf("CreateSnapshot: Failed to lookup the env variable for velero namespace")
		return updatedPeID, err
	}

	upload := builder.ForUpload(veleroNs, "upload-"+peSnapID.GetID()).BackupTimestamp(time.Now()).NextRetryTimestamp(time.Now()).SnapshotID(updatedPeID.String()).Phase(v1api.UploadPhaseNew).Result()
	_, err = pluginClient.VeleropluginV1().Uploads(veleroNs).Create(upload)
	if err != nil {
		this.WithError(err).Errorf("CreateSnapshot: Failed to create Upload CR for PE %s", updatedPeID.String())
		return updatedPeID, err
	}

	return updatedPeID, nil
}

func (this *SnapshotManager) DeleteSnapshot(peID astrolabe.ProtectedEntityID) error {
	log := this.WithField("peID", peID.String())
	log.Infof("Step 0: Cancel on-going upload.")
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

	// look up velero namespace from the env variable in container
	veleroNs, exist := os.LookupEnv("VELERO_NAMESPACE")
	if !exist {
		this.WithError(err).Errorf("DeleteSnapshot: Failed to lookup the env variable for velero namespace")
		return err
	}
	uploadName := "upload-" + peID.GetSnapshotID().GetID()
	log.Infof("Searching for Upload CR: %v", uploadName)
	uploadCR, err := pluginClient.VeleropluginV1().Uploads(veleroNs).Get(uploadName, metav1.GetOptions{})
	if err != nil {
		log.WithError(err).Errorf(" Error while retrieving the upload CR %v", uploadName)
	}
	// An upload is considered done when it's in either of the terminal stages- Completed, CleanupFailed, Canceled
	uploadCompleted := this.isTerminalState(uploadCR)
	if uploadCR != nil && !uploadCompleted {
		log.Infof("Found the Upload CR: %v, updating spec to indicate cancel upload.", uploadName)
		timeNow := clock.RealClock{}
		mutate := func(r *v1api.Upload) {
			uploadCR.Spec.UploadCancel = true
			uploadCR.Status.StartTimestamp = &metav1.Time{Time: timeNow.Now()}
			uploadCR.Status.Message = "Canceling on going upload to repository."
		}
		_, err = utils.PatchUpload(uploadCR, mutate, pluginClient.VeleropluginV1().Uploads(veleroNs), log)

		if err != nil {
			log.WithError(err).Error("Failed to patch ongoing Upload")
			return err
		} else {
			log.Infof("Upload status updated to UploadCancel")
		}
		return nil
	} else {
		if uploadCompleted {
			log.Infof("The upload %v was in terminal stage, proceeding with snapshot deletes", uploadName)
		}
		log.Infof("Step 1: Deleting the local snapshot")
		err = this.DeleteLocalSnapshot(peID)
		if err != nil {
			log.WithError(err).Errorf("Failed to delete the local snapshot for peID")
		} else {
			log.Infof("Deleted the local snapshot")
		}

		isLocalMode := utils.GetBool(this.config[utils.VolumeSnapshotterLocalMode], false)

		if isLocalMode {
			return nil
		}

		log.Infof("Step 2: Deleting the durable snapshot from s3")
		err = this.DeleteRemoteSnapshot(peID)
		if err != nil {
			if !strings.Contains(err.Error(), "The specified key does not exist") {
				log.WithError(err).Errorf("Failed to delete the durable snapshot for PEID")
				return err
			}
		}
		log.Infof("Deleted the durable snapshot from the durable repository")
		return nil
	}
}

func (this *SnapshotManager) isTerminalState(uploadCR *v1api.Upload) bool {
	return uploadCR.Status.Phase == v1api.UploadPhaseCompleted || uploadCR.Status.Phase == v1api.UploadPhaseCleanupFailed || uploadCR.Status.Phase == v1api.UploadPhaseCanceled
}

func (this *SnapshotManager) DeleteLocalSnapshot(peID astrolabe.ProtectedEntityID) error {
	this.WithField("peID", peID.String()).Infof("SnapshotManager.deleteLocalSnapshot Called")
	return this.deleteSnapshotFromRepo(peID, this.ivdPETM)
}

func (this *SnapshotManager) DeleteRemoteSnapshot(peID astrolabe.ProtectedEntityID) error {
	this.WithField("peID", peID.String()).Infof("SnapshotManager.deleteRemoteSnapshot Called")
	return this.deleteSnapshotFromRepo(peID, this.s3PETM)
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
		_, err = pe.DeleteSnapshot(ctx, peID.GetSnapshotID())

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
	this.Debugf("Return from the call of astrolabe DeleteSnapshot API for snapshot %s", peID.GetSnapshotID().String())

	if err != nil {
		this.WithError(err).Errorf("Failed to delete the snapshot %s", peID.GetSnapshotID().String())
		return err
	}

	return nil
}

const PollLogInterval = time.Minute

func (this *SnapshotManager) CreateVolumeFromSnapshot(peID astrolabe.ProtectedEntityID) (updatedID astrolabe.ProtectedEntityID, err error) {
	this.Infof("Start creating Download CR for %s", peID.String())
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

	uuid, _ := uuid.NewRandom()
	downloadRecordName := "download-" + peID.GetSnapshotID().GetID() + "-" + uuid.String()
	download := builder.ForDownload(veleroNs, downloadRecordName).
		RestoreTimestamp(time.Now()).NextRetryTimestamp(time.Now()).SnapshotID(peID.String()).Phase(v1api.DownloadPhaseNew).Result()
	_, err = pluginClient.VeleropluginV1().Downloads(veleroNs).Create(download)
	if err != nil {
		this.WithError(err).Errorf("CreateVolumeFromSnapshot: Failed to create Download CR for %s", peID.String())
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
		download, err = pluginClient.VeleropluginV1().Downloads(veleroNs).Get(downloadRecordName, metav1.GetOptions{})
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
	return
}
