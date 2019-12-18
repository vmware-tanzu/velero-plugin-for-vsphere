/*
Copyright 2019 the Velero contributors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package restore

import (
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	listers "github.com/vmware-tanzu/velero/pkg/generated/listers/velero/v1"
	"github.com/vmware-tanzu/velero/pkg/util/boolptr"
	"github.com/vmware-tanzu/velero/pkg/volume"
)

type PVRestorer interface {
	executePVAction(obj *unstructured.Unstructured) (*unstructured.Unstructured, error)
}

type pvRestorer struct {
	logger                  logrus.FieldLogger
	backup                  *api.Backup
	snapshotVolumes         *bool
	restorePVs              *bool
	volumeSnapshots         []*volume.Snapshot
	volumeSnapshotterGetter VolumeSnapshotterGetter
	snapshotLocationLister  listers.VolumeSnapshotLocationLister
}

func (r *pvRestorer) executePVAction(obj *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	pvName := obj.GetName()
	if pvName == "" {
		return nil, errors.New("PersistentVolume is missing its name")
	}

	// It's simpler to just access the spec through the unstructured object than to convert
	// to structured and back here, especially since the SetVolumeID(...) call below needs
	// the unstructured representation (and does a conversion internally).
	res, ok := obj.Object["spec"]
	if !ok {
		return nil, errors.New("spec not found")
	}
	spec, ok := res.(map[string]interface{})
	if !ok {
		return nil, errors.Errorf("spec was of type %T, expected map[string]interface{}", res)
	}

	delete(spec, "claimRef")

	if boolptr.IsSetToFalse(r.snapshotVolumes) {
		// The backup had snapshots disabled, so we can return early
		return obj, nil
	}

	if boolptr.IsSetToFalse(r.restorePVs) {
		// The restore has pv restores disabled, so we can return early
		return obj, nil
	}

	log := r.logger.WithFields(logrus.Fields{"persistentVolume": pvName})

	snapshotInfo, err := getSnapshotInfo(pvName, r.backup, r.volumeSnapshots, r.snapshotLocationLister)
	if err != nil {
		return nil, err
	}
	if snapshotInfo == nil {
		log.Infof("No snapshot found for persistent volume")
		return obj, nil
	}

	volumeSnapshotter, err := r.volumeSnapshotterGetter.GetVolumeSnapshotter(snapshotInfo.location.Spec.Provider)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	if err := volumeSnapshotter.Init(snapshotInfo.location.Spec.Config); err != nil {
		return nil, errors.WithStack(err)
	}

	volumeID, err := volumeSnapshotter.CreateVolumeFromSnapshot(snapshotInfo.providerSnapshotID, snapshotInfo.volumeType, snapshotInfo.volumeAZ, snapshotInfo.volumeIOPS)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	log.WithField("providerSnapshotID", snapshotInfo.providerSnapshotID).Info("successfully restored persistent volume from snapshot")

	updated1, err := volumeSnapshotter.SetVolumeID(obj, volumeID)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	updated2, ok := updated1.(*unstructured.Unstructured)
	if !ok {
		return nil, errors.Errorf("unexpected type %T", updated1)
	}
	return updated2, nil
}

type snapshotInfo struct {
	providerSnapshotID string
	volumeType         string
	volumeAZ           string
	volumeIOPS         *int64
	location           *api.VolumeSnapshotLocation
}

func getSnapshotInfo(pvName string, backup *api.Backup, volumeSnapshots []*volume.Snapshot, snapshotLocationLister listers.VolumeSnapshotLocationLister) (*snapshotInfo, error) {
	var pvSnapshot *volume.Snapshot
	for _, snapshot := range volumeSnapshots {
		if snapshot.Spec.PersistentVolumeName == pvName {
			pvSnapshot = snapshot
			break
		}
	}

	if pvSnapshot == nil {
		return nil, nil
	}

	loc, err := snapshotLocationLister.VolumeSnapshotLocations(backup.Namespace).Get(pvSnapshot.Spec.Location)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &snapshotInfo{
		providerSnapshotID: pvSnapshot.Status.ProviderSnapshotID,
		volumeType:         pvSnapshot.Spec.VolumeType,
		volumeAZ:           pvSnapshot.Spec.VolumeAZ,
		volumeIOPS:         pvSnapshot.Spec.VolumeIOPS,
		location:           loc,
	}, nil
}
