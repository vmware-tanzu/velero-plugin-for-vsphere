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
	"encoding/json"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/vmware-tanzu/astrolabe/pkg/astrolabe"
	"github.com/vmware-tanzu/astrolabe/pkg/ivd"
	"github.com/vmware-tanzu/astrolabe/pkg/s3repository"
	v1api "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/veleroplugin/v1"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/builder"
	plugin_clientset "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned"
	"github.com/vmware-tanzu/velero/pkg/generated/clientset/versioned"
	"io/ioutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"net/url"
	"os"
	"strings"
	"time"
)

type SnapshotManager struct {
	logrus.FieldLogger
	localMode bool
	ivdPETM   *ivd.IVDProtectedEntityTypeManager
	s3PETM    *s3repository.ProtectedEntityTypeManager
}

func NewSnapshotManagerFromCluster(logger logrus.FieldLogger) (*SnapshotManager, error) {
	params := make(map[string]interface{})
	err := retrieveVcConfigSecret(params, logger)
	if err != nil {
		logger.Errorf("Could not retrieve vsphere credential from k8s secret with error message: %v", err)
		return nil, err
	}

	err = retrieveBackupStorageLocation(params, logger)
	if err != nil {
		logger.Errorf("Could not retrieve velero default backup location with error message: %v", err)
		return nil, err
	}

	snapMgr, err := NewSnapshotManagerFromParamsMap(params, logger)
	if err != nil {
		logger.Errorf("Failed to new a snapshot manager from params map")
		return nil, err
	}
	return snapMgr, nil
}

func retrieveBackupStorageLocation(params map[string]interface{}, logger logrus.FieldLogger) error {
	config, err := rest.InClusterConfig()
	if err != nil {
		return err
	}

	veleroClient, err := versioned.NewForConfig(config)
	if err != nil {
		return err
	}
	defaultBackupLocation := "default"
	var backupStorageLocation *v1.BackupStorageLocation
	backupStorageLocation, err = veleroClient.VeleroV1().BackupStorageLocations("velero").
		Get(defaultBackupLocation, metav1.GetOptions{})
	if err != nil {
		logger.Infof("Failed to get Velero default backup storage location with error message: %v", err)
		backupStorageLocationList, err := veleroClient.VeleroV1().BackupStorageLocations("velero").List(metav1.ListOptions{})
		if err != nil || len(backupStorageLocationList.Items) <= 0 {
			logger.Errorf("Failed to list Velero default backup storage location with error message: %v", err)
			return err
		}
		// Select the first BackupStorageLocation from the list as
		// the placeholder if there is no default BackupStorageLocation.
		logger.Infof("Picked up the first BackupStorageLocation from the BackupStorageLocationList")
		backupStorageLocation = &backupStorageLocationList.Items[0]
	}

	region, ok := backupStorageLocation.Spec.Config["region"];
	if !ok {
		params["region"] = "VOID_REGION"
	} else {
		params["region"] = region
	}

	params["bucket"] = backupStorageLocation.Spec.ObjectStorage.Bucket

	logger.Infof("Velero Backup Storage Location is retrieved, region=%v, bucket=%v", params["region"], params["bucket"])
	return nil
}

func verifyLocalMode(logger logrus.FieldLogger) (bool, error) {
	isLocalMode := false

	config, err := rest.InClusterConfig()
	if err != nil {
		logger.Errorf("Failed to get k8s inClusterConfig")
		return isLocalMode, err
	}

	// retrieve default volume snapshot location from k8s deployment api
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		logger.Errorf("Failed to get k8s clientset with the given config")
		return isLocalMode, err
	}

	deployment, err := clientset.AppsV1().Deployments("velero").Get("velero", metav1.GetOptions{})
	if err != nil {
		logger.Errorf("Failed to get velero deployment using k8s client")
		return isLocalMode, err
	}

	var snapshotLocation string
	for _, container := range deployment.Spec.Template.Spec.Containers {
		if container.Name != "velero" {
			continue
		}
		for _, arg := range container.Args {
			if strings.Contains(arg, "velero.io/vsphere:") {
				parts := strings.Split(arg, ":")
				snapshotLocation = strings.TrimSpace(parts[1])
				break
			}
		}
	}

	if snapshotLocation == "" {
		logger.Infof("No snapshot location for vSphere plugin can be found")
		return isLocalMode, nil
	}

	logger.Infof("Default snapshot location from velero deployment: %v", snapshotLocation)

	// retrieve specific config in snapshot location from velero api
	veleroClient, err := versioned.NewForConfig(config)
	if err != nil {
		logger.Errorf("Failed to get velero clientset with the given config")
		return isLocalMode, err
	}

	volumeSnapshot, err := veleroClient.VeleroV1().VolumeSnapshotLocations("velero").Get(snapshotLocation, metav1.GetOptions{})
	if err != nil {
		logger.Errorf("Failed to get velero snapshot location using velero api client")
		return isLocalMode, err
	}

	configStringMap := volumeSnapshot.Spec.Config
	if len(configStringMap) != 0 {
		if configStringMap["LocalMode"] == "TRUE" || configStringMap["LocalMode"] == "true" {
			logger.Infof("The local mode of Velero plugin for vSphere is turned on")
			isLocalMode = true
		}
	}

	return isLocalMode, nil
}

/*
 * In the CSI setup, VC credential is stored as a secret
 * under the kube-system namespace.
 */
func retrieveVcConfigSecret(params map[string]interface{}, logger logrus.FieldLogger) error {
	config, err := rest.InClusterConfig()
	if err != nil {
		logger.Errorf("Failed to get k8s inClusterConfig")
		return err
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		logger.Errorf("Failed to get k8s clientset with the given config")
		return err
	}

	ns := "kube-system"
	secretApis := clientset.CoreV1().Secrets(ns)
	vsphere_secret := "vsphere-config-secret"
	secret, err := secretApis.Get(vsphere_secret, metav1.GetOptions{})
	if err != nil {
		logger.Errorf("Failed to get k8s secret, %s", vsphere_secret)
		return err
	}
	sEnc := string(secret.Data["csi-vsphere.conf"])
	lines := strings.Split(sEnc, "\n")

	for _, line := range lines {
		if strings.Contains(line, "VirtualCenter") {
			parts := strings.Split(line, "\"")
			params["VirtualCenter"] = parts[1]
		} else if strings.Contains(line, "=") {
			parts := strings.Split(line, "=")
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			params[key] = value[1 : len(value)-1]
		}
	}

	logger.Infof("vSphere VC credential is retrieved")
	return nil
}

func readConfigFile(confFile string) (map[string]interface{}, error) {
	jsonFile, err := os.Open(confFile)
	if err != nil {
		return nil, errors.Wrap(err, "Could not open conf file "+confFile)
	}
	defer jsonFile.Close()
	jsonBytes, err := ioutil.ReadAll(jsonFile)
	if err != nil {
		return nil, errors.Wrap(err, "Could not read conf file "+confFile)
	}
	var result map[string]interface{}
	err = json.Unmarshal([]byte(jsonBytes), &result)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to unmarshal JSON from "+confFile)
	}
	return result, nil
}

func NewSnapshotManagerFromConfigFile(configFilePath string, logger logrus.FieldLogger) (*SnapshotManager, error) {
	params, err := readConfigFile(configFilePath)
	if err != nil {
		logger.Errorf("Could not read conf file, %s, with error message: %v", configFilePath, err)
		return nil, err
	}
	snapMgr, err := NewSnapshotManagerFromParamsMap(params, logger)
	if err != nil {
		logger.Errorf("Failed to new a snapshot manager from params map")
		return nil, err
	}
	return snapMgr, nil
}

func NewSnapshotManagerFromParamsMap(params map[string]interface{}, logger logrus.FieldLogger) (*SnapshotManager, error) {
	var vcUrl url.URL
	vcUrl.Scheme = "https"
	vcHostStr, ok := params["VirtualCenter"].(string)
	if !ok {
		return nil, errors.New("Missing vcHost param, cannot initialize SnapshotManager")
	}
	vcHostPortStr, ok := params["port"].(string)
	if !ok {
		return nil, errors.New("Missing port param, cannot initialize SnapshotManager")
	}

	vcUrl.Host = fmt.Sprintf("%s:%s", vcHostStr, vcHostPortStr)

	vcUser, ok := params["user"].(string)
	if !ok {
		return nil, errors.New("Missing vcUser param, cannot initialize SnapshotManager")
	}
	vcPassword, ok := params["password"].(string)
	if !ok {
		return nil, errors.New("Missing vcPassword param, cannot initialize SnapshotManager")
	}
	vcUrl.User = url.UserPassword(vcUser, vcPassword)
	vcUrl.Path = "/sdk"

	insecure := false
	insecureStr, ok := params["insecure-flag"].(string)
	if ok && (insecureStr == "TRUE" || insecureStr == "true") {
		insecure = true
	}

	region := params["region"].(string)
	bucket := params["bucket"].(string)

	s3URLBase := fmt.Sprintf("https://s3-%s.amazonaws.com/%s/", region, bucket)
	serviceType := "ivd"

	// init ivd PETM
	ivdPETM, err := ivd.NewIVDProtectedEntityTypeManagerFromURL(&vcUrl, s3URLBase, insecure, logger)
	if err != nil {
		logger.Errorf("Error at calling ivd API, NewIVDProtectedEntityTypeManagerFromURL")
		return nil, err
	}

	sess := session.Must(session.NewSession(&aws.Config{
		Region: aws.String(region),
	}))

	s3PETM, err := s3repository.NewS3RepositoryProtectedEntityTypeManager(serviceType, *sess, bucket, logger)
	if err != nil {
		logger.Errorf("Error at creating new ProtectedEntityTypeManager for s3 repository")
		return nil, err
	}

	isLocalMode, err := verifyLocalMode(logger)
	if err != nil {
		logger.Errorf("Error at verifying whether the plugin is in vSphere local mode")
		return nil, err
	}

	snapMgr := SnapshotManager{
		FieldLogger: logger,
		localMode:   isLocalMode,
		ivdPETM:     ivdPETM,
		s3PETM:      s3PETM,
	}

	logger.Infof("Snapshot Manager is initialized in vSphere local mode: %v", snapMgr.localMode)
	return &snapMgr, nil
}

func (this *SnapshotManager) CreateSnapshot(peID astrolabe.ProtectedEntityID, tags map[string]string) (astrolabe.ProtectedEntityID, error) {
	this.Infof("SnapshotManager.CreateSnapshot Called")
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

	if this.localMode == true {
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
	// TODO: Remove hardcode for node
	upload := builder.ForUpload("velero", "upload-"+peSnapID.GetID()).BackupTimestamp(time.Now()).SnapshotID(updatedPeID.String()).Phase(v1api.UploadPhaseNew).Result()
	pluginClient.VeleropluginV1().Uploads("velero").Create(upload)
	return updatedPeID, nil
}

func (this *SnapshotManager) DeleteSnapshot(peID astrolabe.ProtectedEntityID) error {
	this.Infof("Step 1: Deleting the local snapshot, %s", peID.String())
	err := this.DeleteProtectedEntitySnapshot(peID, false)
	if err != nil {
		this.Errorf("Failed to delete the local snapshot")
	}
	this.Infof("Deleted the local snapshot, %s", peID.String())

	if this.localMode == true {
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
	// TODO: Get the node name by api instead of hardcoding
	download := builder.ForDownload("velero", "download-"+peID.GetSnapshotID().GetID()).RestoreTimestamp(time.Now()).SnapshotID(peID.String()).Phase(v1api.DownloadPhaseNew).Result()
	pluginClient.VeleropluginV1().Downloads("velero").Create(download)
	// TODO: Suitable length of timeout
	err = wait.PollImmediate(time.Second, time.Minute, func() (bool, error) {
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
