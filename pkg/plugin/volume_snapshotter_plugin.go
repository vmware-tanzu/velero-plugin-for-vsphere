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

package plugin

import (
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/vmware-tanzu/astrolabe/pkg/astrolabe"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/constants"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/snapshotmgr"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/utils"
	vsv1 "github.com/vmware-tanzu/velero/pkg/plugin/velero/volumesnapshotter/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

// NewVolumeSnapshotter is a plugin for containing state for the blockstore
type NewVolumeSnapshotter struct {
	config map[string]string
	logrus.FieldLogger
	snapMgr       *snapshotmgr.SnapshotManager
	clusterFlavor constants.ClusterFlavor
}

var _ vsv1.VolumeSnapshotter = (*NewVolumeSnapshotter)(nil)

// Init prepares the VolumeSnapshotter for usage using the provided map of
// configuration key-value pairs. It returns an error if the VolumeSnapshotter
// cannot be initialized from the provided config. Note that after v0.10.0, this will happen multiple times.
func (p *NewVolumeSnapshotter) Init(config map[string]string) error {
	p.Infof("Init called with config: %v", config)
	p.config = config
	var err error
	clusterFlavor, err := utils.GetClusterFlavor(nil)
	if err != nil {
		p.WithError(err).Errorf("Failed to determine cluster flavour")
		return err
	}
	p.clusterFlavor = clusterFlavor
	p.Infof("VolumeSnapshotter: Detected cluster flavor: %s", clusterFlavor)
	// Currently backward compatibility is supported only on vanilla.
	if clusterFlavor != constants.VSphere {
		p.Warnf("VolumeSnapshotter: Skipping SnapshotManager initialization for cluster type %s", clusterFlavor)
		return nil
	}
	// Initializing snapshot manager
	// Pass empty param list. VC credentials will be retrieved from the cluster configuration
	p.Infof("Initializing snapshot manager")
	if config == nil {
		config = make(map[string]string)
	}
	config[constants.VolumeSnapshotterManagerLocation] = constants.VolumeSnapshotterPlugin

	params := make(map[string]interface{})
	p.snapMgr, err = snapshotmgr.NewSnapshotManagerFromCluster(params, config, p.FieldLogger)
	if err != nil {
		p.WithError(err).Errorf("Failed at calling snapshotmgr.NewSnapshotManagerFromConfigFile with config: %v", config)
		return err
	}

	p.Infof("vSphere VolumeSnapshotter is initialized")
	return nil
}

// CreateVolumeFromSnapshot creates a new volume in the specified
// availability zone, initialized from the provided snapshot,
// and with the specified type and IOPS (if using provisioned IOPS).
func (p *NewVolumeSnapshotter) CreateVolumeFromSnapshot(snapshotID, volumeType, volumeAZ string, iops *int64) (string, error) {
	p.Infof("VolumeSnapshotter: CreateVolumeFromSnapshot called with snapshotID %s, volumeType %s", snapshotID, volumeType)
	if p.clusterFlavor != constants.VSphere {
		errMsg := "Restore of vsphere volume snapshots with version less than or prior to 1.0.2 are NOT supported in Supervisor and Guest Clusters."
		p.Error(errMsg)
		return "", errors.New("NotSupported: " + errMsg)
	}
	var returnVolumeID, returnVolumeType string
	var peId, returnPeId astrolabe.ProtectedEntityID
	var err error

	peId, err = astrolabe.NewProtectedEntityIDFromString(snapshotID)
	if err != nil {
		p.WithError(err).Errorf("Fail to construct new PE ID from string %s", snapshotID)
		return returnVolumeID, err
	}
	params := make(map[string]map[string]interface{})
	returnPeId, err = p.snapMgr.CreateVolumeFromSnapshot(peId, astrolabe.ProtectedEntityID{}, params)
	if err != nil {
		p.WithError(err).Errorf("Failed at calling SnapshotManager CreateVolumeFromSnapshot with peId %v", peId)
		return returnVolumeID, err
	}

	returnVolumeID = returnPeId.GetID()
	returnVolumeType = returnPeId.GetPeType()

	p.Debugf("A new volume %s with type being %s was just created from the call of SnapshotManager CreateVolumeFromSnapshot", returnVolumeID, returnVolumeType)

	return returnVolumeID, nil
}

// GetVolumeInfo returns the type and IOPS (if using provisioned IOPS) for
// the specified volume in the given availability zone.
func (p *NewVolumeSnapshotter) GetVolumeInfo(volumeID, volumeAZ string) (string, *int64, error) {
	p.Infof("GetVolumeInfo called with volumeID %s, volumeAZ %s", volumeID, volumeAZ)
	var iops int64
	iops = 100 // dummy iops is applied
	return constants.CnsBlockVolumeType, &iops, nil
}

// IsVolumeReady Check if the volume is ready.
func (p *NewVolumeSnapshotter) IsVolumeReady(volumeID, volumeAZ string) (ready bool, err error) {
	p.Infof("IsVolumeReady called with volumeID %s and volumeAZ %s", volumeID, volumeAZ)
	return true, nil
}

// CreateSnapshot creates a snapshot of the specified volume, and applies any provided
// set of tags to the snapshot.
func (p *NewVolumeSnapshotter) CreateSnapshot(volumeID, volumeAZ string, tags map[string]string) (string, error) {
	p.Infof("CreateSnapshot called with volumeID %s, volumeAZ %s, tags %v", volumeID, volumeAZ, tags)
	p.Warnf("Snapshot operation not supported on volume snapshotter plugin")
	return "", nil
}

// DeleteSnapshot deletes the specified volume snapshot.
func (p *NewVolumeSnapshotter) DeleteSnapshot(snapshotID string) error {
	p.Infof("VolumeSnapshotter: DeleteSnapshot called with snapshotID %s", snapshotID)
	if p.clusterFlavor != constants.VSphere {
		errMsg := "Delete of vsphere volume snapshots with version less than or prior to 1.0.2 are NOT supported in Supervisor and Guest Clusters."
		p.Error(errMsg)
		return errors.New("NotSupported: " + errMsg)
	}
	peID, err := astrolabe.NewProtectedEntityIDFromString(snapshotID)
	if err != nil {
		p.WithError(err).Errorf("Fail to construct new Protected Entity ID from string %s", snapshotID)
		return err
	}

	err = p.snapMgr.DeleteSnapshot(peID)
	if err != nil {
		p.WithError(err).Errorf("Failed at calling SnapshotManager DeleteSnapshot for peID %v", peID)
		return err
	}
	return nil
}

// GetVolumeID returns the specific identifier for the PersistentVolume.
func (p *NewVolumeSnapshotter) GetVolumeID(unstructuredPV runtime.Unstructured) (string, error) {
	p.Infof("GetVolumeID called with unstructuredPV %v", unstructuredPV)
	p.Warnf("Explicitly setting empty volume-id to prevent snapshot operations.")
	return "", nil
}

// SetVolumeID sets the specific identifier for the PersistentVolume.
func (p *NewVolumeSnapshotter) SetVolumeID(unstructuredPV runtime.Unstructured, volumeID string) (runtime.Unstructured, error) {
	p.Infof("SetVolumeID called with unstructuredPV %v, volumeID %s", unstructuredPV, volumeID)

	pv := new(v1.PersistentVolume)
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredPV.UnstructuredContent(), pv); err != nil {
		return nil, errors.WithStack(err)
	}

	// the following check is applied to velero v1.1.0 and above
	if pv.Spec.CSI == nil {
		return nil, errors.New("Spec.CSI not found")
	}

	p.Debugf("Set VolumeID, %s, to vSphere CSI VolumeHandle", volumeID)
	pv.Spec.CSI.VolumeHandle = volumeID

	res, err := runtime.DefaultUnstructuredConverter.ToUnstructured(pv)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	unstructuredPV = &unstructured.Unstructured{Object: res}
	p.Debugf("Updated unstructured PV: %v", unstructuredPV)

	return unstructuredPV, nil
}
