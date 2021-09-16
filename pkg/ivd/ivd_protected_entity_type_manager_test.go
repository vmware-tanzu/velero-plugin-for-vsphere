/*
 * Copyright 2019 the Astrolabe contributors
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

package ivd

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/vmware-tanzu/astrolabe/pkg/astrolabe"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/common/vsphere"
	"github.com/vmware-tanzu/astrolabe/pkg/util"
	"github.com/vmware/govmomi/cns"
	cnstypes "github.com/vmware/govmomi/cns/types"
	vim25types "github.com/vmware/govmomi/vim25/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func TestProtectedEntityTypeManager(t *testing.T) {
	vcUrlStr := os.Getenv("VC_URL")
	if vcUrlStr == "" {
		t.Skip("VC_URL not provided, skipping test")
	}
	// Example VC_URL "https://administrator%40vsphere.local:Admin%2123@csm-wdc-vc-0.eng.vmware.com:443/sdk"
	vcUrl, err := url.Parse(vcUrlStr)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s\n", vcUrl.String())

	params := make(map[string]interface{})
	params[vsphere.HostVcParamKey] = vcUrl.Hostname()
	params[vsphere.PortVcParamKey] = vcUrl.Port()
	params[vsphere.UserVcParamKey] = vcUrl.User.Username()
	password, _ := vcUrl.User.Password()
	params[vsphere.PasswordVcParamKey] = password
	params[vsphere.InsecureFlagVcParamKey] = "true"
	params[vsphere.ClusterVcParamKey] = ""

	ivdPETM, err := NewIVDProtectedEntityTypeManager(params, astrolabe.S3Config{URLBase: "/ivd"}, logrus.New())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	pes, err := ivdPETM.GetProtectedEntities(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("# of PEs returned = %d\n", len(pes))
}

func getVcConfigFromParams(params map[string]interface{}) (*url.URL, bool, error) {
	var vcUrl url.URL
	vcUrl.Scheme = "https"
	vcHostStr, err := vsphere.GetVirtualCenterFromParamsMap(params)
	if err != nil {
		return nil, false, err
	}
	vcHostPortStr, err := vsphere.GetPortFromParamsMap(params)
	if err != nil {
		return nil, false, err
	}

	vcUrl.Host = fmt.Sprintf("%s:%s", vcHostStr, vcHostPortStr)

	vcUser, err := vsphere.GetUserFromParamsMap(params)
	if err != nil {
		return nil, false, err
	}
	vcPassword, err := vsphere.GetPasswordFromParamsMap(params)
	if err != nil {
		return nil, false, err
	}
	vcUrl.User = url.UserPassword(vcUser, vcPassword)
	vcUrl.Path = "/sdk"

	insecure, err := vsphere.GetInsecureFlagFromParamsMap(params)

	return &vcUrl, insecure, nil
}

func GetVcUrlFromConfig(config *rest.Config) (*url.URL, bool, error) {
	params := make(map[string]interface{})

	err := util.RetrievePlatformInfoFromConfig(config, params)
	if err != nil {
		return nil, false, errors.Errorf("Failed to retrieve VC config secret: %+v", err)
	}

	vcUrl, insecure, err := getVcConfigFromParams(params)
	if err != nil {
		return nil, false, errors.Errorf("Failed to get VC config from params: %+v", err)
	}

	return vcUrl, insecure, nil
}

func GetParamsFromConfig(config *rest.Config) (map[string]interface{}, error) {
	params := make(map[string]interface{})

	err := util.RetrievePlatformInfoFromConfig(config, params)
	if err != nil {
		return nil, errors.Errorf("Failed to retrieve VC config secret: %+v", err)
	}

	return params, nil
}

func verifyMdIsRestoredAsExpected(md metadata, version string, logger logrus.FieldLogger) bool {
	var reservedLabels []string
	if strings.Contains(version, "6.7U3") {
		reservedLabels = []string{
			"cns.clusterID",
			"cns.clusterType",
			"cns.vSphereUser",
			"cns.k8s.pvName",
			"cns.tag",
		}
	} else if strings.HasPrefix(version, "7.0") {
		reservedLabels = []string{
			"cns.containerCluster.clusterFlavor",
			"cns.containerCluster.clusterId",
			"cns.containerCluster.clusterType",
			"cns.containerCluster.vSphereUser",
			"cns.k8s.pv.name",
			"cns.tag",
			"cns.version",
		}
	} else {
		logger.Debug("Newer VC version than what we expect. Skip the verification.")
		return true
	}

	extendedMdMap := make(map[string]string)

	for _, label := range md.ExtendedMetadata {
		extendedMdMap[label.Key] = label.Value
	}

	for _, key := range reservedLabels {
		_, ok := extendedMdMap[key]
		if !ok {
			return false
		}
	}

	return true
}

func TestCreateCnsVolume(t *testing.T) {
	path := os.Getenv("KUBECONFIG")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// path/to/whatever does not exist
		t.Skipf("The KubeConfig file, %v, is not exist", path)
	}

	config, err := clientcmd.BuildConfigFromFlags("", path)
	if err != nil {
		t.Fatalf("Failed to build k8s config from kubeconfig file: %+v ", err)
	}

	ctx := context.Background()

	// Step 1: To create the IVD PETM, get all PEs and select one as the reference.
	params := make(map[string]interface{})

	err = util.RetrievePlatformInfoFromConfig(config, params)
	if err != nil {
		t.Fatalf("Failed to retrieve VC config secret: %+v", err)
	}

	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	ivdPETM := getIVDProtectedEntityTypeManager(t, err, params, astrolabe.S3Config{URLBase: "/ivd"}, logger)

	virtualCenter := ivdPETM.vcenter
	version := virtualCenter.Client.Version
	logger.Debugf("vcUrl = %v, version = %v", ivdPETM.vcenterConfig.Host, version)

	var queryFilter cnstypes.CnsQueryFilter
	var volumeIDList []cnstypes.CnsVolumeId

	// construct a dummy metadata object
	md := metadata{
		vim25types.VStorageObject{
			DynamicData: vim25types.DynamicData{},
			Config: vim25types.VStorageObjectConfigInfo{
				BaseConfigInfo: vim25types.BaseConfigInfo{
					DynamicData:                 vim25types.DynamicData{},
					Id:                          vim25types.ID{},
					Name:                        "xyz",
					CreateTime:                  time.Time{},
					KeepAfterDeleteVm:           nil,
					RelocationDisabled:          nil,
					NativeSnapshotSupported:     nil,
					ChangedBlockTrackingEnabled: nil,
					Backing:                     nil,
					Iofilter:                    nil,
				},
				CapacityInMB:    10,
				ConsumptionType: nil,
				ConsumerId:      nil,
			},
		},
		vim25types.ManagedObjectReference{},
		nil,
	}

	logger.Debugf("IVD md: %v", md.ExtendedMetadata)

	t.Logf("PE name, %v", md.VirtualStorageObject.Config.Name)
	md = FilterLabelsFromMetadataForCnsAPIs(md, "cns", logger)

	ivdParams := make(map[string]interface{})
	err = util.RetrievePlatformInfoFromConfig(config, ivdParams)
	if err != nil {
		t.Fatalf("Failed to retrieve VC config secret: %+v", err)
	}

	volumeId, err := createCnsVolumeWithClusterConfig(ctx, ivdPETM.vcenterConfig, config, virtualCenter.Client, ivdPETM.cnsManager, md, logger)
	if err != nil {
		t.Fatal("Fail to provision a new volume")
	}

	t.Logf("CNS volume, %v, created", volumeId)
	var volumeIDListToDelete []cnstypes.CnsVolumeId
	volumeIDList = append(volumeIDListToDelete, cnstypes.CnsVolumeId{Id: volumeId})

	defer func() {
		// Always delete the newly created volume at the end of test
		t.Logf("Deleting volume: %+v", volumeIDList)
		deleteTask, err := ivdPETM.cnsManager.DeleteVolume(ctx, volumeIDList, true)
		if err != nil {
			t.Errorf("Failed to delete volume. Error: %+v \n", err)
			t.Fatal(err)
		}
		deleteTaskInfo, err := cns.GetTaskInfo(ctx, deleteTask)
		if err != nil {
			t.Errorf("Failed to delete volume. Error: %+v \n", err)
			t.Fatal(err)
		}
		deleteTaskResult, err := cns.GetTaskResult(ctx, deleteTaskInfo)
		if err != nil {
			t.Errorf("Failed to detach volume. Error: %+v \n", err)
			t.Fatal(err)
		}
		if deleteTaskResult == nil {
			t.Fatalf("Empty delete task results")
		}
		deleteVolumeOperationRes := deleteTaskResult.GetCnsVolumeOperationResult()
		if deleteVolumeOperationRes.Fault != nil {
			t.Fatalf("Failed to delete volume: fault=%+v", deleteVolumeOperationRes.Fault)
		}
		t.Logf("Volume deleted sucessfully")
	}()

	// Step 4: Query the volume result for the newly created protected entity/volume
	queryFilter.VolumeIds = volumeIDList
	queryResult, err := ivdPETM.cnsManager.QueryVolume(ctx, queryFilter)
	if err != nil {
		t.Errorf("Failed to query volume. Error: %+v \n", err)
		t.Fatal(err)
	}
	logger.Debugf("Sucessfully Queried Volumes. queryResult: %+v", queryResult)

	newPE, err := newIVDProtectedEntity(ivdPETM, newProtectedEntityID(NewIDFromString(volumeId)))
	if err != nil {
		t.Fatalf("Failed to get a new PE: %v", err)
	}

	newMD, err := newPE.getMetadata(ctx)
	if err != nil {
		t.Fatalf("Failed to get the metadata: %v", err)
	}

	logger.Debugf("IVD md: %v", newMD.ExtendedMetadata)

	// Verify the test result between the actual and expected
	if md.VirtualStorageObject.Config.Name != queryResult.Volumes[0].Name {
		t.Errorf("Volume names mismatch, src: %v, dst: %v", md.VirtualStorageObject.Config.Name, queryResult.Volumes[0].Name)
	} else {
		t.Logf("Volume names match, name: %v", md.VirtualStorageObject.Config.Name)
	}
}

func TestRestoreCnsVolumeFromSnapshot(t *testing.T) {
	path := os.Getenv("KUBECONFIG")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// path/to/whatever does not exist
		t.Skipf("The KubeConfig file, %v, is not exist", path)
	}

	config, err := clientcmd.BuildConfigFromFlags("", path)
	if err != nil {
		t.Fatalf("Failed to build k8s config from kubeconfig file: %+v ", err)
	}

	ctx := context.Background()

	// Step 1: To create the IVD PETM, get all PEs and select one as the reference.
	vcUrl, insecure, err := GetVcUrlFromConfig(config)
	if err != nil {
		t.Fatalf("Failed to get VC config from params: %+v", err)
	}

	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	params := make(map[string]interface{})
	params[vsphere.HostVcParamKey] = vcUrl.Host
	params[vsphere.PortVcParamKey] = vcUrl.Port()
	params[vsphere.UserVcParamKey] = vcUrl.User.Username()
	password, _ := vcUrl.User.Password()
	params[vsphere.PasswordVcParamKey] = password
	params[vsphere.InsecureFlagVcParamKey] = insecure
	params[vsphere.ClusterVcParamKey] = ""

	ivdPETM := getIVDProtectedEntityTypeManager(t, err, params, astrolabe.S3Config{URLBase: "/ivd"}, logger)

	virtualCenter := ivdPETM.vcenter
	version := virtualCenter.Client.Version
	logger.Debugf("vcUrl = %v, version = %v", vcUrl, version)

	peIDs, err := ivdPETM.GetProtectedEntities(ctx)
	if err != nil {
		t.Fatalf("Failed to get all PEs: %+v", err)
	}
	t.Logf("# of PEs returned = %d\n", len(peIDs))

	var md metadata
	var queryFilter cnstypes.CnsQueryFilter
	var volumeIDList []cnstypes.CnsVolumeId

	peID := peIDs[0]
	t.Logf("Selected PE ID: %v", peID.String())

	// Get general govmomi client and cns client
	// Step 2: Query the volume result for the selected protected entity/volume
	volumeIDList = append(volumeIDList, cnstypes.CnsVolumeId{Id: peID.GetID()})

	queryFilter.VolumeIds = volumeIDList
	queryResult, err := ivdPETM.cnsManager.QueryVolume(ctx, queryFilter)
	if err != nil {
		t.Errorf("Failed to query volume. Error: %+v \n", err)
		t.Fatal(err)
	}
	logger.Debugf("Sucessfully Queried Volumes. queryResult: %+v", queryResult)

	// Step 3: Create a new volume with the same metadata as the selected one
	pe, err := newIVDProtectedEntity(ivdPETM, peID)
	if err != nil {
		t.Fatalf("Failed to get a new PE from the peID, %v: %v", peID.String(), err)
	}

	md, err = pe.getMetadata(ctx)
	if err != nil {
		t.Fatalf("Failed to get the metadata of the PE, %v: %v", pe.id.String(), err)
	}
	logger.Debugf("IVD md: %v", md.ExtendedMetadata)

	ivdParams := make(map[string]interface{})
	err = util.RetrievePlatformInfoFromConfig(config, ivdParams)
	if err != nil {
		t.Fatalf("Failed to retrieve VC config secret: %+v", err)
	}

	t.Logf("PE name, %v", md.VirtualStorageObject.Config.Name)
	md = FilterLabelsFromMetadataForCnsAPIs(md, "cns", logger)
	volumeId, err := createCnsVolumeWithClusterConfig(ctx, ivdPETM.vcenterConfig, config, virtualCenter.Client, ivdPETM.cnsManager, md, logger)
	if err != nil {
		t.Fatal("Fail to provision a new volume")
	}

	t.Logf("CNS volume, %v, created", volumeId)
	var volumeIDListToDelete []cnstypes.CnsVolumeId
	volumeIDList = append(volumeIDListToDelete, cnstypes.CnsVolumeId{Id: volumeId})

	defer func() {
		// Always delete the newly created volume at the end of test
		t.Logf("Deleting volume: %+v", volumeIDList)
		deleteTask, err := ivdPETM.cnsManager.DeleteVolume(ctx, volumeIDList, true)
		if err != nil {
			t.Errorf("Failed to delete volume. Error: %+v \n", err)
			t.Fatal(err)
		}
		deleteTaskInfo, err := cns.GetTaskInfo(ctx, deleteTask)
		if err != nil {
			t.Errorf("Failed to delete volume. Error: %+v \n", err)
			t.Fatal(err)
		}
		deleteTaskResult, err := cns.GetTaskResult(ctx, deleteTaskInfo)
		if err != nil {
			t.Errorf("Failed to detach volume. Error: %+v \n", err)
			t.Fatal(err)
		}
		if deleteTaskResult == nil {
			t.Fatalf("Empty delete task results")
		}
		deleteVolumeOperationRes := deleteTaskResult.GetCnsVolumeOperationResult()
		if deleteVolumeOperationRes.Fault != nil {
			t.Fatalf("Failed to delete volume: fault=%+v", deleteVolumeOperationRes.Fault)
		}
		t.Logf("Volume deleted sucessfully")
	}()

	// Step 4: Query the volume result for the newly created protected entity/volume
	queryFilter.VolumeIds = volumeIDList
	queryResult, err = ivdPETM.cnsManager.QueryVolume(ctx, queryFilter)
	if err != nil {
		t.Errorf("Failed to query volume. Error: %+v \n", err)
		t.Fatal(err)
	}
	logger.Debugf("Sucessfully Queried Volumes. queryResult: %+v", queryResult)

	newPE, err := newIVDProtectedEntity(ivdPETM, newProtectedEntityID(NewIDFromString(volumeId)))
	if err != nil {
		t.Fatalf("Failed to get a new PE from the peID, %v: %v", peID.String(), err)
	}

	newMD, err := newPE.getMetadata(ctx)
	if err != nil {
		t.Fatalf("Failed to get the metadata of the PE, %v: %v", pe.id.String(), err)
	}

	logger.Debugf("IVD md: %v", newMD.ExtendedMetadata)

	// Verify the test result between the actual and expected
	if md.VirtualStorageObject.Config.Name != queryResult.Volumes[0].Name {
		t.Errorf("Volume names mismatch, src: %v, dst: %v", md.VirtualStorageObject.Config.Name, queryResult.Volumes[0].Name)
	} else {
		t.Logf("Volume names match, name: %v", md.VirtualStorageObject.Config.Name)
	}

	if verifyMdIsRestoredAsExpected(newMD, version, logger) {
		t.Logf("Volume metadata is restored as expected")
	} else {
		t.Errorf("Volume metadata is NOT restored as expected")
	}
}

func TestOverwriteCnsVolumeFromSnapshot(t *testing.T) {
	path := os.Getenv("KUBECONFIG")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// path/to/whatever does not exist
		t.Skipf("The KubeConfig file, %v, is not exist", path)
	}

	config, err := clientcmd.BuildConfigFromFlags("", path)
	if err != nil {
		t.Fatalf("Failed to build k8s config from kubeconfig file: %+v ", err)
	}

	ctx := context.Background()

	// Step 1: To create the IVD PETM, get all PEs and select one as the reference.
	params, err := GetParamsFromConfig(config)
	if err != nil {
		t.Fatalf("Failed to get VC config from params: %+v", err)
	}

	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	ivdPETM := getIVDProtectedEntityTypeManager(t, err, params, astrolabe.S3Config{URLBase: "/ivd"}, logger)

	virtualCenter := ivdPETM.vcenter

	peIDs, err := ivdPETM.GetProtectedEntities(ctx)
	if err != nil {
		t.Fatalf("Failed to get all PEs: %+v", err)
	}
	t.Logf("# of PEs returned = %d\n", len(peIDs))

	var md metadata
	var queryFilter cnstypes.CnsQueryFilter
	var volumeIDList []cnstypes.CnsVolumeId

	peID := peIDs[0]
	t.Logf("Selected PE ID: %v", peID.String())

	// Get general govmomi client and cns client
	// Step 2: Query the volume result for the selected protected entity/volume
	volumeIDList = append(volumeIDList, cnstypes.CnsVolumeId{Id: peID.GetID()})

	queryFilter.VolumeIds = volumeIDList
	queryResult, err := ivdPETM.cnsManager.QueryVolume(ctx, queryFilter)
	if err != nil {
		t.Errorf("Failed to query volume. Error: %+v \n", err)
		t.Fatal(err)
	}
	logger.Debugf("Sucessfully Queried Volumes. queryResult: %+v", queryResult)

	// Step 3: Create a new volume with the same metadata as the selected one
	pe, err := newIVDProtectedEntity(ivdPETM, peID)
	if err != nil {
		t.Fatalf("Failed to get a new PE from the peID, %v: %v", peID.String(), err)
	}

	md, err = pe.getMetadata(ctx)
	if err != nil {
		t.Fatalf("Failed to get the metadata of the PE, %v: %v", pe.id.String(), err)
	}
	logger.Debugf("IVD md: %v", md.ExtendedMetadata)

	snapshotID, err := pe.Snapshot(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to snapshot PE, %v: %v", pe.id.String(), err)
	}
	snapshotPEID := pe.GetID().IDWithSnapshot(snapshotID)

	ivdParams := make(map[string]interface{})
	err = util.RetrievePlatformInfoFromConfig(config, ivdParams)
	if err != nil {
		t.Fatalf("Failed to retrieve VC config secret: %+v", err)
	}

	t.Logf("PE name, %v", md.VirtualStorageObject.Config.Name)
	md = FilterLabelsFromMetadataForCnsAPIs(md, "cns", logger)
	volumeId, err := createCnsVolumeWithClusterConfig(ctx, ivdPETM.vcenterConfig, config, virtualCenter.Client, ivdPETM.cnsManager, md, logger)
	if err != nil {
		t.Fatal("Fail to provision a new volume")
	}

	t.Logf("CNS volume, %v, created", volumeId)
	var volumeIDListToDelete []cnstypes.CnsVolumeId
	volumeIDList = append(volumeIDListToDelete, cnstypes.CnsVolumeId{Id: volumeId})

	defer func() {
		// Always delete the newly created volume at the end of test
		t.Logf("Deleting volume: %+v", volumeIDList)
		deleteTask, err := ivdPETM.cnsManager.DeleteVolume(ctx, volumeIDList, true)
		if err != nil {
			t.Errorf("Failed to delete volume. Error: %+v \n", err)
			t.Fatal(err)
		}
		deleteTaskInfo, err := cns.GetTaskInfo(ctx, deleteTask)
		if err != nil {
			t.Errorf("Failed to delete volume. Error: %+v \n", err)
			t.Fatal(err)
		}
		deleteTaskResult, err := cns.GetTaskResult(ctx, deleteTaskInfo)
		if err != nil {
			t.Errorf("Failed to detach volume. Error: %+v \n", err)
			t.Fatal(err)
		}
		if deleteTaskResult == nil {
			t.Fatalf("Empty delete task results")
		}
		deleteVolumeOperationRes := deleteTaskResult.GetCnsVolumeOperationResult()
		if deleteVolumeOperationRes.Fault != nil {
			t.Fatalf("Failed to delete volume: fault=%+v", deleteVolumeOperationRes.Fault)
		}
		t.Logf("Volume deleted sucessfully")
	}()

	// Step 4: Query the volume result for the newly created protected entity/volume
	queryFilter.VolumeIds = volumeIDList
	queryResult, err = ivdPETM.cnsManager.QueryVolume(ctx, queryFilter)
	if err != nil {
		t.Errorf("Failed to query volume. Error: %+v \n", err)
		t.Fatal(err)
	}
	logger.Debugf("Sucessfully Queried Volumes. queryResult: %+v", queryResult)

	snapshotPE, err := ivdPETM.GetProtectedEntity(ctx, snapshotPEID)
	if err != nil {
		t.Fatalf("Failed to get a PE for the snapshot peID, %v: %v", snapshotID.String(), err)
	}

	newPE, err := newIVDProtectedEntity(ivdPETM, newProtectedEntityID(NewIDFromString(volumeId)))
	if err != nil {
		t.Fatalf("Failed to get a new PE from the peID, %v: %v", peID.String(), err)
	}

	err = newPE.Overwrite(ctx, snapshotPE, nil, false)
	if err != nil {
		t.Fatalf("Failed to overwrite PE with snapshotPE, %v, %v: %v", pe.id.String(), snapshotPE.GetID().String(), err)
	}
}

/*
	Steps

	0. Set KUBECONFIG to the kubeconfig of vanilla or supervisor cluster.
	1. Invoke GetParamsFromConfig to retrieve the IVD params from secret.
	2. Invoke NewIVDProtectedEntityTypeManager to create a new IVDProtectedEntityTypeManager
	3. Use the vcenterManager instance to verify clients are initialized.
	4. Invoke GetProtectedEntities to verify vslm api invocations.

	Expected Output:
	1. No errors.

*/
func TestIVDProtectedEntityTypeManager_NewIVDProtectedEntityTypeManager(t *testing.T) {
	path := os.Getenv("KUBECONFIG")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("The KubeConfig file, %v, is not exist", path)
	}
	config, err := clientcmd.BuildConfigFromFlags("", path)
	if err != nil {
		t.Fatalf("Failed to build k8s config from kubeconfig file: %+v ", err)
	}
	log := GetLogger()
	ctx := context.Background()
	params := make(map[string]interface{})
	err = util.RetrievePlatformInfoFromConfig(config, params)
	if err != nil {
		t.Fatalf("Failed to get VC params from k8s: %+v", err)
	}
	s3Config := astrolabe.S3Config{
		URLBase: "VOID_URL",
	}
	ivdPETM := getIVDProtectedEntityTypeManager(t, err, params, s3Config, log)

	if ivdPETM.vcenterConfig == nil || ivdPETM.vslmManager == nil || ivdPETM.vcenter == nil ||
		ivdPETM.cnsManager == nil {
		t.Fatalf("Failed to correctly initialize IVD PE type manager.")
	}
	defer func() {
		err = ivdPETM.vcenter.Disconnect(ctx)
		if err != nil {
			log.Warnf("Was unable to disconnect vc as part of cleanup.")
		}
	}()

	// Validate by invoking ivd petm APIs.
	entities, err := ivdPETM.GetProtectedEntities(ctx)
	if err != nil {
		t.Fatalf("Received error when retrieving IVD PEs, err: %+v", err)
	}
	log.Infof("PASS: Successfully retrieved %d IVD PEs", len(entities))
}

/*
	Steps

	0. Set KUBECONFIG to the kubeconfig of vanilla or supervisor cluster.
	1. Invoke GetParamsFromConfig to retrieve the IVD params from secret.
	2. Invoke NewIVDProtectedEntityTypeManager to create a new IVDProtectedEntityTypeManager
	3. Invoke GetProtectedEntities to verify vslm api invocations.
	4. Create the params copy and update the credentials.
	5. Invoke ReloadConfig with the new params.
	6. Invoke GetProtectedEntities to verify that the new credentials are used for vsl api invocation.

	Expected Output:
	1. No errors.

*/
func TestIVDProtectedEntityTypeManager_ReloadConfig(t *testing.T) {
	path := os.Getenv("KUBECONFIG")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("The KubeConfig file, %v, is not exist", path)
	}
	config, err := clientcmd.BuildConfigFromFlags("", path)
	if err != nil {
		t.Fatalf("Failed to build k8s config from kubeconfig file: %+v ", err)
	}
	log := GetLogger()
	ctx := context.Background()
	params := make(map[string]interface{})
	err = util.RetrievePlatformInfoFromConfig(config, params)
	if err != nil {
		t.Fatalf("Failed to get VC params from k8s: %+v", err)
	}

	s3Config := astrolabe.S3Config{
		URLBase: "VOID_URL",
	}
	ivdPETM := getIVDProtectedEntityTypeManager(t, err, params, s3Config, log)

	defer func() {
		err = ivdPETM.vcenter.Disconnect(ctx)
		if err != nil {
			log.Warnf("Was unable to disconnect vc as part of cleanup.")
		}
	}()
	// Using admin as alternate vcenter config
	adminUser := "administrator@vsphere.local"
	adminPass := "Admin!23"
	params[vsphere.UserVcParamKey] = adminUser
	params[vsphere.PasswordVcParamKey] = adminPass
	err = ivdPETM.ReloadConfig(ctx, params)
	if err != nil {
		t.Fatalf("Failed to Reload Config for IVD PE Type Manager")
	}
	if ivdPETM.vcenterConfig.Username != adminUser || ivdPETM.vcenterConfig.Password != adminPass {
		t.Fatalf("The IVD PE Type Manager parameters weren't reloaded succcessfully.")
	}
	virtualCenter := ivdPETM.vcenter
	if virtualCenter.Config.Username != adminUser || virtualCenter.Config.Password != adminPass {
		t.Fatalf("The vcenter instance credentials were not updated.")
	}
	// Validate by invoking ivd petm APIs.
	entities, err := ivdPETM.GetProtectedEntities(ctx)
	if err != nil {
		t.Fatalf("Received error when retrieving IVD PEs, err: %+v", err)
	}
	log.Infof("PASS: Successfully retrieved %d IVD PEs after credential rotation", len(entities))
}

/*
	Steps

	0. Set KUBECONFIG to the kubeconfig of vanilla or supervisor cluster.
	1. Invoke GetParamsFromConfig to retrieve the IVD params from secret.
	2. Invoke NewIVDProtectedEntityTypeManager to create a new IVDProtectedEntityTypeManager
	3. Invoke GetProtectedEntities to verify vslm api invocations.
	4. Invoke ReloadConfig with the same params again.
	6. Verify vcenterManager instance to see no config change since there is no change in params.

	Expected Output:
	1. No errors.

*/
func TestIVDProtectedEntityTypeManager_ReloadConfigNoConfigChanges(t *testing.T) {
	path := os.Getenv("KUBECONFIG")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("The KubeConfig file, %v, is not exist", path)
	}
	config, err := clientcmd.BuildConfigFromFlags("", path)
	if err != nil {
		t.Fatalf("Failed to build k8s config from kubeconfig file: %+v ", err)
	}
	log := GetLogger()
	ctx := context.Background()
	params := make(map[string]interface{})
	err = util.RetrievePlatformInfoFromConfig(config, params)
	if err != nil {
		t.Fatalf("Failed to get VC params from k8s: %+v", err)
	}

	s3Config := astrolabe.S3Config{
		URLBase: "VOID_URL",
	}
	ivdPETM := getIVDProtectedEntityTypeManager(t, err, params, s3Config, log)
	defer func() {
		err = ivdPETM.vcenter.Disconnect(ctx)
		if err != nil {
			log.Warnf("Was unable to disconnect vc as part of cleanup.")
		}
	}()
	err = ivdPETM.ReloadConfig(ctx, params)
	if err != nil {
		t.Fatalf("Failure duing ReloadConfig with same params.")
	}
	virtualCenter := ivdPETM.vcenter
	if virtualCenter.Config.Username != params[vsphere.UserVcParamKey] ||
		virtualCenter.Config.Password != params[vsphere.PasswordVcParamKey] {
		t.Fatalf("The vcenter instance credentials were unexpectedly updated.")
	}
	// Validate by invoking ivd petm APIs.
	entities, err := ivdPETM.GetProtectedEntities(ctx)
	if err != nil {
		t.Fatalf("Received error when retrieving IVD PEs, err: %+v", err)
	}
	log.Infof("PASS: Reload with no param change did not update vc, IVD PE: %d", len(entities))
}

func getIVDProtectedEntityTypeManager(t *testing.T, err error, params map[string]interface{}, s3Config astrolabe.S3Config, log logrus.FieldLogger) *IVDProtectedEntityTypeManager {
	astrolabePETM, err := NewIVDProtectedEntityTypeManager(params, s3Config, log)
	if err != nil {
		t.Fatalf("Failed to initialize IVD PE Type manager, err: %+v", err)
	}
	ivdPETM, ok := astrolabePETM.(*IVDProtectedEntityTypeManager)
	if !ok {
		t.Fatalf("Did not get an IVDProtectedEntityTypeManager from NewIVDProtectedEntityTypeManager")
	}
	return ivdPETM
}

func GetLogger() logrus.FieldLogger {
	logger := logrus.New()
	formatter := new(logrus.TextFormatter)
	formatter.TimestampFormat = time.RFC3339Nano
	formatter.FullTimestamp = true
	logger.SetFormatter(formatter)
	logger.SetLevel(logrus.DebugLevel)
	return logger
}
