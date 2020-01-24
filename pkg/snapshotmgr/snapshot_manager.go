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
	"os"
	"time"
)

type SnapshotManager struct {
	logrus.FieldLogger
	config    map[string]string
	ivdPETM   *ivd.IVDProtectedEntityTypeManager
	s3PETM    *s3repository.ProtectedEntityTypeManager
}

func NewSnapshotManagerFromCluster(config map[string]string, logger logrus.FieldLogger) (*SnapshotManager, error) {
	params := make(map[string]interface{})
	err := utils.RetrieveVcConfigSecret(params, logger)
	if err != nil {
		logger.Errorf("Could not retrieve vsphere credential from k8s secret with error message: %v", err)
		return nil, err
	}
	logger.Infof("SnapshotManager: vSphere VC credential is retrieved")

	var s3PETM *s3repository.ProtectedEntityTypeManager
	var ivdPETM *ivd.IVDProtectedEntityTypeManager

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
				logger.Errorf("Could not retrieve velero default backup location with error message: %v", err)
				return nil, err
			}
		}

		logger.Infof("SnapshotManager: Velero Backup Storage Location is retrieved, region=%v, bucket=%v", params["region"], params["bucket"])

		s3PETM, err = utils.GetS3PETMFromParamsMap(params, logger)
		if err != nil {
			logger.Errorf("Failed to get s3PETM from params map")
			return nil, err
		}
		logger.Infof("SnapshotManager: Get s3PETM from the params map")
	}

	// If the local mode is enabled, we don't need to specify remote storage location
	// for persistence since it has been taken care of 3rd party backup solution.
	ivdPETM, err = utils.GetIVDPETMFromParamsMap(params, logger)
	if err != nil {
		logger.Errorf("Failed to get ivdPETM from params map")
		return nil, err
	}
	logger.Infof("SnapshotManager: Get ivdPETM from the params map")

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
	this.Infof("SnapshotManager.CreateSnapshot Called with tags, %v", tags)
	this.Infof("Step 1: Creating a snapshot in local repository")
	var updatedPeID astrolabe.ProtectedEntityID
	ctx := context.Background()
	pe, err := this.ivdPETM.GetProtectedEntity(ctx, peID)
	if err != nil {
		this.Errorf("Failed to GetProtectedEntity for, %s, with error message, %v", peID.String(), err)
		return astrolabe.ProtectedEntityID{}, err
	}
	this.Debugf("Ready to call PE snapshot API")
	peSnapID, err := pe.Snapshot(ctx)
	this.Debugf("Return from the call of PE snapshot API")
	if err != nil {
		this.Errorf("Failed to Snapshot PE for, %s, with error message, %v", peID.String(), err)
		return astrolabe.ProtectedEntityID{}, err
	}

	this.Debugf("constructing the returned PE snapshot id, ", peSnapID.GetID())
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
		this.Errorf("Failed to get k8s inClusterConfig")
		return updatedPeID, err
	}
	pluginClient, err := plugin_clientset.NewForConfig(config)
	if err != nil {
		this.Errorf("Failed to get k8s clientset with the given config")
		return updatedPeID, err
	}

	// look up velero namespace from the env variable in container
	veleroNs, exist := os.LookupEnv("VELERO_NAMESPACE")
	if !exist {
		this.Errorf("CreateSnapshot: Failed to lookup the env variable for velero namespace")
		return updatedPeID, err
	}

	upload := builder.ForUpload(veleroNs, "upload-"+peSnapID.GetID()).BackupTimestamp(time.Now()).SnapshotID(updatedPeID.String()).Phase(v1api.UploadPhaseNew).Result()
	_, err = pluginClient.VeleropluginV1().Uploads(veleroNs).Create(upload)
	if err != nil {
		this.Errorf("CreateSnapshot: Failed to create Upload CR")
		return updatedPeID, err
	}

	return updatedPeID, nil
}

func (this *SnapshotManager) DeleteSnapshot(peID astrolabe.ProtectedEntityID) error {
	this.Infof("Step 1: Deleting the local snapshot, %s", peID.String())
	err := this.DeleteProtectedEntitySnapshot(peID, false)
	if err != nil {
		this.Errorf("Failed to delete the local snapshot")
	}
	this.Infof("Deleted the local snapshot, %s", peID.String())

	isLocalMode := utils.GetBool(this.config[utils.VolumeSnapshotterLocalMode], false)

	if isLocalMode {
		return nil
	}

	this.Infof("Step 2: Deleting the durable snapshot, %s, from s3", peID.String())
	err = this.DeleteProtectedEntitySnapshot(peID, true)
	if err != nil {
		this.Errorf("Failed to delete the durable snapshot")
		return err
	}
	this.Infof("Deleted the durable snapshot, %s, from the durable repository", peID.String())
	return nil
}

func (this *SnapshotManager) DeleteProtectedEntitySnapshot(peID astrolabe.ProtectedEntityID, toRemote bool) error {
	this.Infof("SnapshotManager.deleteProtectedEntitySnapshot Called")
	this.Infof("Input arguemnts: peID = %s, toRemote = %v", peID.String(), toRemote)

	if !peID.HasSnapshot() {
		this.Errorf("No snapshot is associated with this Protected Entity")
		return nil
	}

	ctx := context.Background()
	var petm astrolabe.ProtectedEntityTypeManager
	if toRemote {
		petm = this.s3PETM
	} else {
		petm = this.ivdPETM
	}

	pe, err := petm.GetProtectedEntity(ctx, peID)
	if err != nil {
		this.Errorf("Failed to get the ProtectedEntity")
		return err
	}
	snapshotIds, err := pe.ListSnapshots(ctx)
	if err != nil {
		return errors.Wrap(err, "Failed to list snapshots")
	}

	if len(snapshotIds) == 0 {
		this.Infof("There is no snapshots from the perspective of ProtectedEntityTypeManager. Skipping the deleteSnapshot operation")
		return nil
	}

	this.Debugf("Calling PE.DeleteSnapshot API")
	success, err := pe.DeleteSnapshot(ctx, peID.GetSnapshotID())
	if err != nil {
		this.Errorf("Failed to delete the snapshot: success=%t", success)
		return err
	}
	this.Infof("Return from the call of PE deleteSnapshot API")

	return nil
}

func (this *SnapshotManager) CreateVolumeFromSnapshot(peID astrolabe.ProtectedEntityID) (astrolabe.ProtectedEntityID, error) {
	this.Info("Start creating Download CR")
	config, err := rest.InClusterConfig()
	if err != nil {
		this.Errorf("Failed to get k8s inClusterConfig")
		return peID, err
	}
	pluginClient, err := plugin_clientset.NewForConfig(config)
	if err != nil {
		this.Errorf("Failed to get k8s clientset with the given config")
		return peID, err
	}

	veleroNs, exist := os.LookupEnv("VELERO_NAMESPACE")
	if !exist {
		this.Errorf("CreateVolumeFromSnapshot: Failed to lookup the env variable for velero namespace")
		return peID, err
	}

	download := builder.ForDownload(veleroNs, "download-"+peID.GetSnapshotID().GetID()).RestoreTimestamp(time.Now()).SnapshotID(peID.String()).Phase(v1api.DownloadPhaseNew).Result()
	_, err = pluginClient.VeleropluginV1().Downloads(veleroNs).Create(download)
	if err != nil {
		this.Errorf("CreateVolumeFromSnapshot: Failed to create Download CR")
		return peID, err
	}

	// TODO: Suitable length of timeout
	err = wait.PollImmediateInfinite(time.Second, func() (bool, error) {
		download, _ = pluginClient.VeleropluginV1().Downloads(veleroNs).Get("download-"+peID.GetSnapshotID().GetID(), metav1.GetOptions{})
		if download.Status.Phase == v1api.DownloadPhaseCompleted {
			return true, nil
		} else if download.Status.Phase == v1api.DownloadPhaseFailed {
			return false, errors.Errorf("Create download cr failed.")
		} else {
			return false, nil
		}
	})
	updatedID, err := astrolabe.NewProtectedEntityIDFromString(download.Status.VolumeID)
	return updatedID, err
}
