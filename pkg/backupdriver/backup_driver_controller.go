/*
Copyright 2020 the Velero contributors.

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

package backupdriver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"

	"github.com/vmware-tanzu/astrolabe/pkg/astrolabe"
	backupdriverapi "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/backupdriver/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// createSnapshot creates a snapshot of the specified volume, and applies any provided
// set of tags to the snapshot.
func (ctrl *backupDriverController) createSnapshot(snapshot *backupdriverapi.Snapshot) error {
	ctrl.logger.Infof("Entering createSnapshot: %s/%s", snapshot.Namespace, snapshot.Name)

	objName := snapshot.Spec.TypedLocalObjectReference.Name
	objKind := snapshot.Spec.TypedLocalObjectReference.Kind
	if objKind != "PersistentVolumeClaim" {
		errMsg := fmt.Sprintf("resourceHandle Kind %s is not supported. Only PersistentVolumeClaim Kind is supported", objKind)
		ctrl.logger.Error(errMsg)
		return errors.New(errMsg)
	}

	// call SnapshotMgr CreateSnapshot API
	peID := astrolabe.NewProtectedEntityIDWithNamespace(objKind, objName, snapshot.Namespace)
	ctrl.logger.Infof("CreateSnapshot: The initial Astrolabe PE ID: %s", peID)
	if ctrl.snapManager == nil {
		errMsg := fmt.Sprintf("snapManager is not initialized.")
		ctrl.logger.Error(errMsg)
		return errors.New(errMsg)
	}

	// NOTE: tags is required to call snapManager.CreateSnapshot
	// but it is not really used
	var tags map[string]string

	if ctrl.backupdriverClient == nil {
		errMsg := fmt.Sprintf("backupdriverClient is not initialized")
		ctrl.logger.Error(errMsg)
		return errors.New(errMsg)
	}

	// Get the BackupRepository name. The snapshot spec can have an empty backup repository
	// name in case of local mode.
	brName := snapshot.Spec.BackupRepository
	if ctrl.svcKubeConfig != nil && brName != "" {
		// For guest cluster, get the supervisor backup repository name
		br, err := ctrl.backupdriverClient.BackupRepositories().Get(brName, metav1.GetOptions{})
		if err != nil {
			ctrl.logger.WithError(err).Errorf("Failed to get snapshot Backup Repository %s", brName)
		}
		// Update the backup repository name with the Supervisor BR name
		brName = br.SvcBackupRepositoryName
	}

	peID, err := ctrl.snapManager.CreateSnapshotWithBackupRepository(peID, tags, brName)
	if err != nil {
		errMsg := fmt.Sprintf("failed at calling SnapshotManager CreateSnapshot from peID %v", peID)
		ctrl.logger.Error(errMsg)
		return err
	}

	// Construct the snapshotID for cns volume
	snapshotID := peID.String()
	ctrl.logger.Infof("createSnapshot: The snapshotID depends on the Astrolabe PE ID in the format, <peType>:<id>:<snapshotID>, %s", snapshotID)

	// NOTE: Uncomment the code to retrieve snapshot from API server
	// when needed.
	// Retrieve snapshot from API server to make sure it is up to date
	//newSnapshot, err := ctrl.backupdriverClient.Snapshots(snapshot.Namespace).Get(snapshot.Name, metav1.GetOptions{})
	//if err != nil {
	//	return err
	//}
	//snapshotClone := newSnapshot.DeepCopy()

	snapshotClone := snapshot.DeepCopy()
	snapshotClone.Status.Phase = backupdriverapi.SnapshotPhaseSnapshotted
	snapshotClone.Status.Progress.TotalBytes = 0
	snapshotClone.Status.Progress.BytesDone = 0
	snapshotClone.Status.SnapshotID = snapshotID

	ctx := context.Background()
	pe, err := ctrl.snapManager.Pem.GetProtectedEntity(ctx, peID)
	if err != nil {
		ctrl.logger.WithError(err).Errorf("Failed to get the ProtectedEntity from peID %s", peID.String())
		return err
	}

	metadataReader, err := pe.GetMetadataReader(ctx)
	if err != nil {
		ctrl.logger.Errorf("createSnapshot: Error happened when calling PE GetMetadataReader: %v", err)
		return err
	}

	mdBuf, err := ioutil.ReadAll(metadataReader) // TODO - limit this so it can't run us out of memory here
	if err != nil && err != io.EOF {
		ctrl.logger.Errorf("createSnapshot: Error happened when reading metadata: %v", err)
		return err
	}

	snapshotClone.Status.Metadata = mdBuf

	snapshot, err = ctrl.backupdriverClient.Snapshots(snapshotClone.Namespace).UpdateStatus(snapshotClone)
	if err != nil {
		ctrl.logger.Infof("createSnapshot: update status fro snapshot %s/%s failed: %v", snapshotClone.Namespace, snapshotClone.Name, err)
		return err
	}

	ctrl.logger.Infof("createSnapshot %s/%s completed with snapshotID: %s, phase in status updated to %s", snapshot.Namespace, snapshot.Name, snapshotID, snapshot.Status.Phase)
	return nil
}

func (ctrl *backupDriverController) deleteSnapshot(deleteSnapshot *backupdriverapi.DeleteSnapshot) error {
	ctrl.logger.Infof("deleteSnapshot called with SnapshotID %s, Namespace: %s, Name: %s",
		deleteSnapshot.Spec.SnapshotID, deleteSnapshot.Namespace, deleteSnapshot.Name)
	if deleteSnapshot.Spec.SnapshotID == "" {
		errMsg := fmt.Sprintf("snapshotID is required to delete snapshot")
		ctrl.logger.Error(errMsg)
		return errors.New(errMsg)
	}
	snapshotID := deleteSnapshot.Spec.SnapshotID
	ctrl.logger.Infof("Calling Snapshot Manager to delete snapshot with snapshotID %s", snapshotID)
	peID, err := astrolabe.NewProtectedEntityIDFromString(snapshotID)
	if err != nil {
		ctrl.logger.WithError(err).Errorf("Fail to construct new Protected Entity ID from string %s", snapshotID)
		return err
	}

	if ctrl.backupdriverClient == nil {
		errMsg := fmt.Sprintf("backupdriverClient is not initialized")
		ctrl.logger.Error(errMsg)
		return errors.New(errMsg)
	}

	brName := deleteSnapshot.Spec.BackupRepository
	if ctrl.svcKubeConfig != nil && brName != "" {
		// For guest cluster, get the supervisor backup repository name
		br, err := ctrl.backupdriverClient.BackupRepositories().Get(brName, metav1.GetOptions{})
		if err != nil {
			ctrl.logger.WithError(err).Errorf("Failed to get snapshot Backup Repository %s", brName)
		}
		// Update the backup repository name with the Supervisor BR name
		brName = br.SvcBackupRepositoryName
	}

	if ctrl.snapManager == nil {
		errMsg := fmt.Sprintf("snapManager is not initialized.")
		ctrl.logger.Error(errMsg)
		return errors.New(errMsg)
	}

	err = ctrl.snapManager.DeleteSnapshotWithBackupRepository(peID, brName)
	if err != nil {
		ctrl.logger.WithError(err).Errorf("Failed at calling SnapshotManager DeleteSnapshot for peID %v", peID)
		return err
	}

	// Update the status
	deleteSnapshotClone := deleteSnapshot.DeepCopy()
	deleteSnapshotClone.Status.Phase = backupdriverapi.DeleteSnapshotPhaseCompleted
	deleteSnapshotClone.Status.Message = "DeleteSnapshot Successfully processed."

	deleteSnapshotUpdate, err := ctrl.backupdriverClient.DeleteSnapshots(deleteSnapshot.Namespace).UpdateStatus(deleteSnapshotClone)
	if err != nil {
		ctrl.logger.Errorf("deleteSnapshot: update status for SnapshotID: %s Namespace: %s Name: %s, failed: %v",
			deleteSnapshot.Spec.SnapshotID, deleteSnapshot.Namespace, deleteSnapshot.Name, err)
		return err
	}

	ctrl.logger.Errorf("deleteSnapshot: update status for SnapshotID: %s Namespace: %s Name: %s, Completed",
		deleteSnapshotUpdate.Spec.SnapshotID, deleteSnapshotUpdate.Namespace, deleteSnapshotUpdate.Name)
	return nil
}

// cloneFromSnapshot creates a new volume from the provided snapshot
func (ctrl *backupDriverController) cloneFromSnapshot(cloneFromSnapshot *backupdriverapi.CloneFromSnapshot) error {
	ctrl.logger.Infof("cloneFromSnapshot called with cloneFromSnapshot: %+v", cloneFromSnapshot)
	var returnVolumeID, returnVolumeType string

	var peId, returnPeId astrolabe.ProtectedEntityID
	var err error

	// Need to extract PVC info from metadata to clone from snapshot
	// Fail clone if metadata does not exist
	if len(cloneFromSnapshot.Spec.Metadata) == 0 {
		errMsg := fmt.Sprintf("no metadata in cloneFromSnapshot %s/%s", cloneFromSnapshot.Namespace, cloneFromSnapshot.Name)
		ctrl.logger.Error(errMsg)
		return errors.New(errMsg)
	}

	pvc := v1.PersistentVolumeClaim{}
	err = pvc.Unmarshal(cloneFromSnapshot.Spec.Metadata)
	if err != nil {
		ctrl.logger.Errorf("Error extracting metadata into PVC: %v", err)
		return err
	}
	ctrl.logger.Infof("cloneFromSnapshot: retrieved PVC %s/%s from metadata. %+v", pvc.Namespace, pvc.Name, pvc)

	// cloneFromSnapshot.Spec.Kind should be "PersistentVolumeClaim" for now
	peId = astrolabe.NewProtectedEntityIDWithNamespace(cloneFromSnapshot.Spec.Kind, pvc.Name, pvc.Namespace)
	if err != nil {
		ctrl.logger.WithError(err).Errorf("Fail to construct new PE ID for %s/%s", pvc.Namespace, pvc.Name)
		return err
	}
	ctrl.logger.Infof("cloneFromSnapshot: Generated PE ID: %s", peId.String())

	brName := cloneFromSnapshot.Spec.BackupRepository
	if brName != "" && ctrl.svcKubeConfig != nil {
		// For guest cluster, get the supervisor backup repository name
		br, err := ctrl.backupdriverClient.BackupRepositories().Get(brName, metav1.GetOptions{})
		if err != nil {
			ctrl.logger.WithError(err).Errorf("Failed to get snapshot Backup Repository %s", brName)
		}
		// Update the backup repository name with the Supervisor BR name
		brName = br.SvcBackupRepositoryName
	}

	// snapManager.CreateVolumeFromSnapshotWithMetadata waits until download is complete.
	returnPeId, download, err := ctrl.snapManager.CreateVolumeFromSnapshotWithMetadata(peId, cloneFromSnapshot.Spec.Metadata, cloneFromSnapshot.Spec.SnapshotID, brName)
	if err != nil {
		ctrl.logger.WithError(err).Errorf("Failed at calling SnapshotManager cloneFromSnapshot with peId %v", peId)
		return err
	}

	returnVolumeID = returnPeId.GetID()
	returnVolumeType = returnPeId.GetPeType()

	clone := cloneFromSnapshot.DeepCopy()
	// Since we wait until download is complete in
	// snapManager.CreateVolumeFromSnapshotWithMetadata,
	// download.Status.Phase should be up to date.
	clone.Status.Phase = backupdriverapi.ClonePhase(download.Status.Phase)
	clone.Status.Message = download.Status.Message
	apiGroup := ""
	clone.Status.ResourceHandle = &v1.TypedLocalObjectReference{
		APIGroup: &apiGroup,
		Kind:     "PersistentVolumeClaim",
		Name:     returnVolumeID,
	}

	clone, err = ctrl.backupdriverClient.CloneFromSnapshots(clone.Namespace).UpdateStatus(clone)
	if err != nil {
		return err
	}

	ctrl.logger.Infof("A new volume %s with type being %s was just created from the call of SnapshotManager cloneFromSnapshot", returnVolumeID, returnVolumeType)

	return nil
}