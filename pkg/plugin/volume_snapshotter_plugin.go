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
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/utils"
	"github.com/vmware-tanzu/velero/pkg/plugin/velero"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/vmware-tanzu/astrolabe/pkg/astrolabe"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/snapshotmgr"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

// Volume keeps track of volumes created by this plugin
type Volume struct {
	volType, az string
	iops        int64
}

// Snapshot keeps track of snapshots created by this plugin
type Snapshot struct {
	volID, az string
	tags      map[string]string
}

// NewVolumeSnapshotter is a plugin for containing state for the blockstore
type NewVolumeSnapshotter struct {
	config map[string]string
	logrus.FieldLogger
	volumes   map[string]Volume
	snapshots map[string]Snapshot
	snapMgr *snapshotmgr.SnapshotManager
}

var _ velero.VolumeSnapshotter = (*NewVolumeSnapshotter)(nil)

// Init prepares the VolumeSnapshotter for usage using the provided map of
// configuration key-value pairs. It returns an error if the VolumeSnapshotter
// cannot be initialized from the provided config. Note that after v0.10.0, this will happen multiple times.
func (p *NewVolumeSnapshotter) Init(config map[string]string) error {
	p.Infof("Init called", config)
	p.config = config

	// Make sure we don't overwrite data, now that we can re-initialize the plugin
	if p.volumes == nil {
		p.volumes = make(map[string]Volume)
	}
	if p.snapshots == nil {
		p.snapshots = make(map[string]Snapshot)
	}

	// Initializing snapshot manager
	p.Infof("Initializing snapshot manager")
	if config == nil {
		config = make(map[string]string)
	}
	config[utils.VolumeSnapshotterManagerLocation] = utils.VolumeSnapshotterPlugin
	var err error
	p.snapMgr, err = snapshotmgr.NewSnapshotManagerFromCluster(config, p.FieldLogger)
	if err != nil {
		p.Errorf("Failed at calling snapshotmgr.NewSnapshotManagerFromConfigFile with error message: %v", err)
		return err
	}

	p.Infof("vSphere VolumeSnapshotter is initialized")

	return nil
}

// CreateVolumeFromSnapshot creates a new volume in the specified
// availability zone, initialized from the provided snapshot,
// and with the specified type and IOPS (if using provisioned IOPS).
func (p *NewVolumeSnapshotter) CreateVolumeFromSnapshot(snapshotID, volumeType, volumeAZ string, iops *int64) (string, error) {
	p.Infof("CreateVolumeFromSnapshot called", snapshotID, volumeType)
	var returnVolumeID, returnVolumeType string

	var peId,returnPeId astrolabe.ProtectedEntityID
	var err error
	peId, err = astrolabe.NewProtectedEntityIDFromString(snapshotID)
	if err != nil {
		p.Errorf("Fail to construct new PE ID from string")
		return returnVolumeID, err
	}
	returnPeId, err = p.snapMgr.CreateVolumeFromSnapshot(peId)
	if err != nil {
		p.Errorf("Failed at calling SnapshotManager CreateVolumeFromSnapshot")
		return returnVolumeID, err
	}

	returnVolumeID = returnPeId.GetID()
	returnVolumeType = returnPeId.GetPeType()

	p.Debugf("A new volume %s with type being %s was just created from the call of SnapshotManager CreateVolumeFromSnapshot", returnVolumeID, returnVolumeType)

	p.volumes[returnVolumeID] = Volume{
		volType: returnVolumeType,
		az:      volumeAZ,
		iops:    100,
	}
	return returnVolumeID, nil
}

const cnsBlockVolumeType = "ivd"

// GetVolumeInfo returns the type and IOPS (if using provisioned IOPS) for
// the specified volume in the given availability zone.
func (p *NewVolumeSnapshotter) GetVolumeInfo(volumeID, volumeAZ string) (string, *int64, error) {
	p.Infof("GetVolumeInfo called", volumeID, volumeAZ)
	if val, ok := p.volumes[volumeID]; ok {
		iops := val.iops
		return val.volType, &iops, nil
	}
	p.Debugf("Hardcoded GetVolumeInfo for now: volumeType: %s; iops: nil", cnsBlockVolumeType)
	return cnsBlockVolumeType, nil, nil
}

// IsVolumeReady Check if the volume is ready.
func (p *NewVolumeSnapshotter) IsVolumeReady(volumeID, volumeAZ string) (ready bool, err error) {
	p.Infof("IsVolumeReady called", volumeID, volumeAZ)
	return true, nil
}

// CreateSnapshot creates a snapshot of the specified volume, and applies any provided
// set of tags to the snapshot.
func (p *NewVolumeSnapshotter) CreateSnapshot(volumeID, volumeAZ string, tags map[string]string) (string, error) {
	p.Infof("CreateSnapshot called", volumeID, volumeAZ, tags)
	var snapshotID string

	// call SnapshotMgr CreateSnapshot API
	peID := astrolabe.NewProtectedEntityID("ivd", volumeID)
	peID, err := p.snapMgr.CreateSnapshot(peID, tags)
	if err != nil {
		p.Errorf("Fail at calling SnapshotManager CreateSnapshot")
		return "", err
	}

	// Remember the "original" volume, only required for the first
	// time.
	if _, exists := p.volumes[volumeID]; !exists {
		p.volumes[volumeID] = Volume{
			volType: cnsBlockVolumeType,
			az:      volumeAZ,
			iops:    100,
		}
	}

	// Construct the snapshotID for cns volume
	snapshotID = peID.String()
	p.Debugf("The snapshotID depends on the Astrolabe PE ID in the format, <peType>:<id>:<snapshotID>, %s", snapshotID)
	// Remember the snapshot
	p.snapshots[snapshotID] = Snapshot{volID: volumeID,
		az:   volumeAZ,
		tags: tags}

	p.Debugf("CreateSnapshot returning: ", snapshotID)
	return snapshotID, nil
}

// DeleteSnapshot deletes the specified volume snapshot.
func (p *NewVolumeSnapshotter) DeleteSnapshot(snapshotID string) error {
	p.Infof("DeleteSnapshot called", snapshotID)
	peID, err := astrolabe.NewProtectedEntityIDFromString(snapshotID)
	if err != nil {
		p.Errorf("Fail to construct new PE ID from string")
		return err
	}

	err = p.snapMgr.DeleteSnapshot(peID)
	if err != nil {
		p.Errorf("Failed at calling SnapshotManager DeleteSnapshot")
		return err
	}

	return nil
}

// GetVolumeID returns the specific identifier for the PersistentVolume.
func (p *NewVolumeSnapshotter) GetVolumeID(unstructuredPV runtime.Unstructured) (string, error) {
	p.Infof("GetVolumeID called", unstructuredPV)

	pv := new(v1.PersistentVolume)
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredPV.UnstructuredContent(), pv); err != nil {
		return "", errors.WithStack(err)
	}

	if pv.Spec.CSI == nil {
		return "", errors.New("Spec.CSI not found")
	}

	if pv.Spec.CSI.VolumeHandle == "" {
		return "", errors.New("Spec.CSI.VolumeHandle not found")
	}

	volumeId := pv.Spec.CSI.VolumeHandle
	p.Debugf("vSphere CSI VolumeID: ", volumeId)

	return volumeId, nil
}

// SetVolumeID sets the specific identifier for the PersistentVolume.
func (p *NewVolumeSnapshotter) SetVolumeID(unstructuredPV runtime.Unstructured, volumeID string) (runtime.Unstructured, error) {
	p.Infof("SetVolumeID called", unstructuredPV, volumeID)

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

	p.Debugf("Updated unstructured PV: ", unstructuredPV)

	return unstructuredPV, nil
}
