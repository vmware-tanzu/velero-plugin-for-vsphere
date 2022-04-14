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
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/constants"
	"io"
	"io/ioutil"
	"time"

	"github.com/vmware-tanzu/astrolabe/pkg/astrolabe"
	backupdriverapi "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/backupdriver/v1alpha1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// createSnapshot creates a snapshot of the specified volume, and applies any provided
// set of tags to the snapshot.
func (ctrl *backupDriverController) createSnapshot(snapshot *backupdriverapi.Snapshot) error {
	ctrl.logger.Infof("Entering createSnapshot: %s/%s", snapshot.Namespace, snapshot.Name)
	ctx := context.Background()
	snapshotStatusFields := make(map[string]interface{})
	// NOTE: tags is required to call snapManager.CreateSnapshot
	// but it is not really used
	var tags map[string]string
	objName := snapshot.Spec.TypedLocalObjectReference.Name
	objKind := snapshot.Spec.TypedLocalObjectReference.Kind
	if objKind != "PersistentVolumeClaim" {
		errMsg := fmt.Sprintf("createSnapshot PreSnapshot: resourceHandle Kind %s is not supported. Only PersistentVolumeClaim Kind is supported", objKind)
		ctrl.logger.Error(errMsg)
		snapshotStatusFields["Message"] = errMsg
		_, statusUpdateErr := ctrl.updateSnapshotStatusPhase(ctx, snapshot.Namespace, snapshot.Name, backupdriverapi.SnapshotPhaseSnapshotFailed, snapshotStatusFields)
		if statusUpdateErr != nil {
			ctrl.logger.Error("Failed to update the Snapshot Status to Failed state.")
		}
		return errors.New(errMsg)
	}

	peID := astrolabe.NewProtectedEntityIDWithNamespace(objKind, objName, snapshot.Namespace)
	ctrl.logger.Infof("createSnapshot: The initial Astrolabe PE ID: %s", peID)

	if ctrl.backupdriverClient == nil || ctrl.snapManager == nil {
		var errMsg string
		if ctrl.backupdriverClient == nil {
			errMsg = fmt.Sprintf("createSnapshot PreSnapshot: backupdriverClient is not initialized\n")
		}
		if ctrl.snapManager == nil {
			errMsg = errMsg + fmt.Sprintf("createSnapshot PreSnapshot: snapManager is not initialized.")
		}
		ctrl.logger.Error(errMsg)
		snapshotStatusFields["Message"] = errMsg
		_, statusUpdateErr := ctrl.updateSnapshotStatusPhase(ctx, snapshot.Namespace, snapshot.Name, backupdriverapi.SnapshotPhaseSnapshotFailed, snapshotStatusFields)
		if statusUpdateErr != nil {
			ctrl.logger.Error("Failed to update the Snapshot Status to Failed state.")
		}
		return errors.New(errMsg)
	}

	// Get the BackupRepository name. The snapshot spec can have an empty backup repository
	// name in case of local mode.
	brName := snapshot.Spec.BackupRepository
	if ctrl.svcKubeConfig != nil && brName != "" {
		// For guest cluster, get the supervisor backup repository name
		br, err := ctrl.backupdriverClient.BackupRepositories().Get(ctx, brName, metav1.GetOptions{})
		if err != nil {
			errMsg := fmt.Sprintf("createSnapshot PreSnapshot: Failed to get snapshot Backup Repository %s", brName)
			ctrl.logger.Error(errMsg)
			snapshotStatusFields["Message"] = errMsg
			_, statusUpdateErr := ctrl.updateSnapshotStatusPhase(ctx, snapshot.Namespace, snapshot.Name, backupdriverapi.SnapshotPhaseSnapshotFailed, snapshotStatusFields)
			if statusUpdateErr != nil {
				ctrl.logger.Error("Failed to update the Snapshot Status to Failed state.")
			}
			return err
		}
		// Update the backup repository name with the Supervisor BR name
		brName = br.SvcBackupRepositoryName
	}

	// call SnapshotMgr CreateSnapshot API
	peID, svcSnapshotName, err := ctrl.snapManager.CreateSnapshotWithBackupRepository(peID, tags, brName, snapshot.Namespace+"/"+snapshot.Name, snapshot.Labels[constants.SnapshotBackupLabel])
	if err != nil {
		errMsg := fmt.Sprintf("createSnapshot: Failed at calling SnapshotManager CreateSnapshot from peID %v , Error: %v", peID, err)
		ctrl.logger.Error(errMsg)
		snapshotStatusFields["Message"] = errMsg
		_, statusUpdateErr := ctrl.updateSnapshotStatusPhase(ctx, snapshot.Namespace, snapshot.Name, backupdriverapi.SnapshotPhaseSnapshotFailed, snapshotStatusFields)
		if statusUpdateErr != nil {
			ctrl.logger.Error("Failed to update the Snapshot Status to Failed state.")
		}
		return err
	}

	// Construct the snapshotID for cns volume
	snapshotID := peID.String()
	ctrl.logger.Infof("createSnapshot: The snapshotID depends on the Astrolabe PE ID in the format, <peType>:<id>:<snapshotID>, %s", snapshotID)

	snapshotStatusFields["Progress.TotalBytes"] = int64(0)
	snapshotStatusFields["Progress.BytesDone"] = int64(0)
	snapshotStatusFields["SnapshotID"] = snapshotID
	if svcSnapshotName != "" {
		snapshotStatusFields["SvcSnapshotName"] = svcSnapshotName
	}

	pe, err := ctrl.snapManager.Pem.GetProtectedEntity(ctx, peID)
	if err != nil {
		errMsg := fmt.Sprintf("createSnapshot PostSnapshot: Failed to get the ProtectedEntity from peID %s, err: %+v", peID.String(), err)
		ctrl.logger.Error(errMsg)
		snapshotStatusFields["Message"] = errMsg
		_, statusUpdateErr := ctrl.updateSnapshotStatusPhase(ctx, snapshot.Namespace, snapshot.Name, backupdriverapi.SnapshotPhaseSnapshotFailed, snapshotStatusFields)
		if statusUpdateErr != nil {
			ctrl.logger.Error("Failed to update the Snapshot Status to Failed state.")
		}
		return err
	}

	metadataReader, err := pe.GetMetadataReader(ctx)
	if err != nil {
		errMsg := fmt.Sprintf("createSnapshot PostSnapshot: Error happened when calling PE GetMetadataReader: %v", err)
		ctrl.logger.Error(errMsg)
		snapshotStatusFields["Message"] = errMsg
		_, statusUpdateErr := ctrl.updateSnapshotStatusPhase(ctx, snapshot.Namespace, snapshot.Name, backupdriverapi.SnapshotPhaseSnapshotFailed, snapshotStatusFields)
		if statusUpdateErr != nil {
			ctrl.logger.Error("Failed to update the Snapshot Status to Failed state.")
		}
		return err
	}

	mdBuf, err := ioutil.ReadAll(metadataReader) // TODO - limit this so it can't run us out of memory here
	if err != nil && err != io.EOF {
		errMsg := fmt.Sprintf("createSnapshot PostSnapshot: Error happened when reading metadata: %v", err)
		ctrl.logger.Error(errMsg)
		snapshotStatusFields["Message"] = errMsg
		_, statusUpdateErr := ctrl.updateSnapshotStatusPhase(ctx, snapshot.Namespace, snapshot.Name, backupdriverapi.SnapshotPhaseSnapshotFailed, snapshotStatusFields)
		if statusUpdateErr != nil {
			ctrl.logger.Error("Failed to update the Snapshot Status to Failed state.")
		}
		return err
	}
	snapshotStatusFields["Metadata"] = mdBuf

	updatedSnapshot, err := ctrl.updateSnapshotStatusPhase(ctx, snapshot.Namespace, snapshot.Name, backupdriverapi.SnapshotPhaseSnapshotted, snapshotStatusFields)
	if err != nil {
		ctrl.logger.WithError(err).Errorf("createSnapshot PostSnapshot: update status for snapshot %s/%s failed", snapshot.Namespace, snapshot.Name)
		return err
	}

	ctrl.logger.Infof("createSnapshot %s/%s completed with snapshotID: %s, phase in status updated from %s to %s", updatedSnapshot.Namespace, updatedSnapshot.Name, updatedSnapshot.Status.SnapshotID, snapshot.Status.Phase, updatedSnapshot.Status.Phase)
	return nil
}

func (ctrl *backupDriverController) deleteSnapshot(deleteSnapshot *backupdriverapi.DeleteSnapshot) error {
	ctrl.logger.Infof("deleteSnapshot called with SnapshotID %s, Namespace: %s, Name: %s",
		deleteSnapshot.Spec.SnapshotID, deleteSnapshot.Namespace, deleteSnapshot.Name)
	snapshotID := deleteSnapshot.Spec.SnapshotID
	ctx := context.Background()
	deleteSnapshotStatusFields := make(map[string]interface{})
	ctrl.logger.Infof("Calling Snapshot Manager to delete snapshot with snapshotID %s", snapshotID)
	peID, err := astrolabe.NewProtectedEntityIDFromString(snapshotID)
	if err != nil {
		errMsg := fmt.Sprintf("deleteSnapshot PreDeleteSnapshot: Fail to construct new Protected Entity ID from string %s", snapshotID)
		ctrl.logger.Error(errMsg)
		deleteSnapshotStatusFields["Message"] = errMsg
		_, statusUpdateErr := ctrl.updateDeleteSnapshotStatusPhase(ctx, deleteSnapshot.Namespace, deleteSnapshot.Name,
			backupdriverapi.DeleteSnapshotPhaseFailed, deleteSnapshotStatusFields)
		if statusUpdateErr != nil {
			ctrl.logger.Error("Failed to update the DeleteSnapshot Status to Failed state.")
		}
		return err
	}

	if ctrl.backupdriverClient == nil || ctrl.snapManager == nil {
		var errMsg string
		if ctrl.backupdriverClient == nil {
			errMsg = fmt.Sprintf("deleteSnapshot PreDeleteSnapshot: backupdriverClient is not initialized\n")
		}
		if ctrl.snapManager == nil {
			errMsg = errMsg + fmt.Sprintf("deleteSnapshot PreDeleteSnapshot: snapManager is not initialized.")
		}
		ctrl.logger.Error(errMsg)
		deleteSnapshotStatusFields["Message"] = errMsg
		_, statusUpdateErr := ctrl.updateDeleteSnapshotStatusPhase(ctx, deleteSnapshot.Namespace, deleteSnapshot.Name,
			backupdriverapi.DeleteSnapshotPhaseFailed, deleteSnapshotStatusFields)
		if statusUpdateErr != nil {
			ctrl.logger.Error("deleteSnapshot PreDeleteSnapshot: Failed to update the DeleteSnapshot Status to Failed state.")
		}
		return errors.New(errMsg)
	}

	brName := deleteSnapshot.Spec.BackupRepository
	// Backups created in Guest Cluster when local-mode is set do not exercise the Backup Repository claims workflow, as
	// a result, the Backup Repository is unset. If the Backup Repository is observed unset when deleting the backup
	// then there is no need to retrieve the corresponding supervisor Backup Repository as it is implied local-mode.
	if ctrl.svcKubeConfig != nil && brName!= "" {
		// For guest cluster, get the supervisor backup repository name
		br, err := ctrl.backupdriverClient.BackupRepositories().Get(ctx, brName, metav1.GetOptions{})
		if err != nil {
			errMsg := fmt.Sprintf("deleteSnapshot PreDeleteSnapshot: Failed to get delete snapshot Backup Repository %s", brName)
			ctrl.logger.Error(errMsg)
			deleteSnapshotStatusFields["Message"] = errMsg
			_, statusUpdateErr := ctrl.updateDeleteSnapshotStatusPhase(ctx, deleteSnapshot.Namespace, deleteSnapshot.Name,
				backupdriverapi.DeleteSnapshotPhaseFailed, deleteSnapshotStatusFields)
			if statusUpdateErr != nil {
				ctrl.logger.Error("Failed to update the DeleteSnapshot Status to Failed state.")
			}
			return errors.New(errMsg)
		}
		// Update the backup repository name with the Supervisor BR name
		brName = br.SvcBackupRepositoryName
	}

	err = ctrl.snapManager.DeleteSnapshotWithBackupRepository(peID, brName, deleteSnapshot.Name)
	if err != nil {
		errMsg := fmt.Sprintf("deleteSnapshot : Failed at calling SnapshotManager DeleteSnapshot for peID %v, error: %v", peID, err)
		ctrl.logger.Error(errMsg)
		deleteSnapshotStatusFields["Message"] = errMsg
		_, statusUpdateErr := ctrl.updateDeleteSnapshotStatusPhase(ctx, deleteSnapshot.Namespace, deleteSnapshot.Name,
			backupdriverapi.DeleteSnapshotPhaseFailed, deleteSnapshotStatusFields)
		if statusUpdateErr != nil {
			ctrl.logger.Error("Failed to update the DeleteSnapshot Status to Failed state.")
		}
		return err
	}

	deleteSnapshotStatusFields["Message"] = "DeleteSnapshot Successfully processed."

	deleteSnapshotUpdate, err := ctrl.updateDeleteSnapshotStatusPhase(ctx, deleteSnapshot.Namespace, deleteSnapshot.Name,
		backupdriverapi.DeleteSnapshotPhaseCompleted, deleteSnapshotStatusFields)
	if err != nil {
		errMsg := fmt.Sprintf("deleteSnapshot PostDeleteSnapshot: update status for SnapshotID: %s Namespace: %s Name: %s, failed: %v",
			deleteSnapshot.Spec.SnapshotID, deleteSnapshot.Namespace, deleteSnapshot.Name, err)
		ctrl.logger.Error(errMsg)
		return err
	}

	ctrl.logger.Infof("deleteSnapshot: update status for SnapshotID: %s Namespace: %s Name: %s, Completed",
		deleteSnapshotUpdate.Spec.SnapshotID, deleteSnapshotUpdate.Namespace, deleteSnapshotUpdate.Name)
	return nil
}

// cloneFromSnapshot creates a new volume from the provided snapshot
func (ctrl *backupDriverController) cloneFromSnapshot(cloneFromSnapshot *backupdriverapi.CloneFromSnapshot) error {
	ctrl.logger.Infof("cloneFromSnapshot called with cloneFromSnapshot: %+v", cloneFromSnapshot)
	ctx := context.Background()
	var returnVolumeID, returnVolumeType string
	cloneFromSnapshotStatusFields := make(map[string]interface{})
	var peId, returnPeId astrolabe.ProtectedEntityID
	var err error

	// Need to extract PVC info from metadata to clone from snapshot
	// Fail clone if metadata does not exist
	if len(cloneFromSnapshot.Spec.Metadata) == 0 {
		errMsg := fmt.Sprintf("cloneFromSnapshot PreCloneFromSnapshot: no metadata in CloneFromSnapshot CR %s/%s", cloneFromSnapshot.Namespace, cloneFromSnapshot.Name)
		ctrl.logger.Error(errMsg)
		cloneFromSnapshotStatusFields["Message"] = errMsg
		_, statusUpdateErr := ctrl.updateCloneFromSnapshotStatusPhase(ctx, cloneFromSnapshot.Namespace, cloneFromSnapshot.Name,
			backupdriverapi.ClonePhaseFailed, cloneFromSnapshotStatusFields)
		if statusUpdateErr != nil {
			ctrl.logger.Error("Failed to update the CloneFromSnapshot Status to Failed state.")
		}
		return errors.New(errMsg)
	}

	pvc := v1.PersistentVolumeClaim{}
	err = pvc.Unmarshal(cloneFromSnapshot.Spec.Metadata)
	if err != nil {
		errMsg := fmt.Sprintf("cloneFromSnapshot PreCloneFromSnapshot: Error extracting metadata into PVC: %v", err)
		ctrl.logger.Error(errMsg)
		cloneFromSnapshotStatusFields["Message"] = errMsg
		_, statusUpdateErr := ctrl.updateCloneFromSnapshotStatusPhase(ctx, cloneFromSnapshot.Namespace, cloneFromSnapshot.Name,
			backupdriverapi.ClonePhaseFailed, cloneFromSnapshotStatusFields)
		if statusUpdateErr != nil {
			ctrl.logger.Error("Failed to update the CloneFromSnapshot Status to Failed state.")
		}
		return err
	}
	if pvc.Spec.StorageClassName != nil && (*pvc.Spec.StorageClassName) != ""{
		ctrl.logger.Infof("StorageClassName is %s for PVC %s/%s", *pvc.Spec.StorageClassName, pvc.Namespace, pvc.Name)
	} else {
		errMsg := fmt.Sprintf("cloneFromSnapshot PreCloneFromSnapshot: Failed for PVC %s/%s because StorageClassName is not set", pvc.Namespace, pvc.Name)
		ctrl.logger.Error(errMsg)
		cloneFromSnapshotStatusFields["Message"] = errMsg
		_, statusUpdateErr := ctrl.updateCloneFromSnapshotStatusPhase(ctx, cloneFromSnapshot.Namespace, cloneFromSnapshot.Name,
			backupdriverapi.ClonePhaseFailed, cloneFromSnapshotStatusFields)
		if statusUpdateErr != nil {
			ctrl.logger.Error("Failed to update the CloneFromSnapshot Status to Failed state.")
		}
		return errors.New(errMsg)
	}
	ctrl.logger.Infof("cloneFromSnapshot: retrieved PVC %s/%s from metadata. %+v", pvc.Namespace, pvc.Name, pvc)

	// cloneFromSnapshot.Spec.Kind should be "PersistentVolumeClaim" for now
	peId = astrolabe.NewProtectedEntityIDWithNamespace(cloneFromSnapshot.Spec.Kind, pvc.Name, pvc.Namespace)
	ctrl.logger.Infof("cloneFromSnapshot: Generated PE ID: %s", peId.String())

	returnPeId, err = ctrl.snapManager.CreateVolumeFromSnapshotWithMetadata(peId, cloneFromSnapshot.Spec.Metadata,
		cloneFromSnapshot.Spec.SnapshotID, cloneFromSnapshot.Spec.BackupRepository, cloneFromSnapshot.Namespace, cloneFromSnapshot.Name)
	if err != nil {
		errMsg := fmt.Sprintf("cloneFromSnapshot: Failed at calling SnapshotManager CreateVolumeFromSnapshotWithMetadata with peId %s, err: %+v", peId, err)
		cloneFromSnapshotStatusFields["Message"] = errMsg
		_, statusUpdateErr := ctrl.updateCloneFromSnapshotStatusPhase(ctx, cloneFromSnapshot.Namespace, cloneFromSnapshot.Name,
			backupdriverapi.ClonePhaseFailed, cloneFromSnapshotStatusFields)
		if statusUpdateErr != nil {
			ctrl.logger.Error("Failed to update the CloneFromSnapshot Status to Failed state.")
		}
		return err
	}

	returnVolumeID = returnPeId.GetID()
	returnVolumeType = returnPeId.GetPeType()

	ctrl.logger.Infof("A new volume %s with type being %s was just created from the call of SnapshotManager CreateVolumeFromSnapshotWithMetadata", returnVolumeID, returnVolumeType)
	return nil
}

// Update the snapshot status phase
func (ctrl *backupDriverController) updateSnapshotStatusPhase(ctx context.Context, snapshotNs string, snapshotName string,
	newPhase backupdriverapi.SnapshotPhase, snapshotStatusFields map[string]interface{}) (*backupdriverapi.Snapshot, error) {
	ctrl.logger.Debugf("Entering updateSnapshotStatusPhase: %s/%s, Phase %s", snapshotNs, snapshotName, newPhase)

	// Retrieve the latest version of Snapshot and update the status.
	snapshot, err := ctrl.backupdriverClient.Snapshots(snapshotNs).Get(ctx, snapshotName, metav1.GetOptions{})
	if err != nil {
		ctrl.logger.Errorf("updateSnapshotStatusPhase: Failed to retrieve the latest snapshot state, error: %v", err)
		return nil, err
	}

	if snapshot.Status.Phase == newPhase {
		ctrl.logger.Debugf("updateSnapshotStatusPhase: Snapshot %s/%s already updated with %s", snapshot.Namespace, snapshot.Name, newPhase)
		return snapshot, nil
	}

	snapshotClone := snapshot.DeepCopy()
	snapshotClone.Status.Phase = newPhase
	if msg, ok := snapshotStatusFields["Message"]; ok {
		snapshotClone.Status.Message = msg.(string)
	}
	if totalBytes, ok := snapshotStatusFields["Progress.TotalBytes"]; ok {
		snapshotClone.Status.Progress.TotalBytes = totalBytes.(int64)
	}
	if bytesDone, ok := snapshotStatusFields["Progress.BytesDone"]; ok {
		snapshotClone.Status.Progress.BytesDone = bytesDone.(int64)
	}
	if snapshotID, ok := snapshotStatusFields["SnapshotID"]; ok {
		snapshotClone.Status.SnapshotID = snapshotID.(string)
	}
	if svcSnapshotName, ok := snapshotStatusFields["SvcSnapshotName"]; ok {
		snapshotClone.Status.SvcSnapshotName = svcSnapshotName.(string)
	}
	if mdBuf, ok := snapshotStatusFields["Metadata"]; ok {
		snapshotClone.Status.Metadata = mdBuf.([]byte)
	}

	if newPhase == backupdriverapi.SnapshotPhaseUploaded {
		snapshotClone.Status.CompletionTimestamp = &metav1.Time{Time: time.Now()}
	}

	updatedSnapshot, err := ctrl.backupdriverClient.Snapshots(snapshotClone.Namespace).UpdateStatus(ctx, snapshotClone, metav1.UpdateOptions{})
	if err != nil {
		ctrl.logger.Errorf("updateSnapshotStatusPhase: update status for snapshot %s/%s failed: %v", snapshotClone.Namespace, snapshotClone.Name, err)
		return nil, err
	}
	ctrl.logger.Infof("updateSnapshotStatusPhase: Snapshot %s/%s updated phase from %s to %s",
		updatedSnapshot.Namespace, updatedSnapshot.Name, snapshot.Status.Phase, updatedSnapshot.Status.Phase)
	return updatedSnapshot, nil
}

func (ctrl *backupDriverController) updateDeleteSnapshotStatusPhase(ctx context.Context, deleteSnapshotNs string, deleteSnapshotName string,
	newPhase backupdriverapi.DeleteSnapshotPhase, deleteSnapshotStatusFields map[string]interface{}) (*backupdriverapi.DeleteSnapshot, error) {
	ctrl.logger.Debugf("Entering updateDeleteSnapshotStatusPhase: %s/%s, Phase %s", deleteSnapshotNs, deleteSnapshotName, newPhase)

	// Retrieve the latest version of DeleteSnapshot CRD and update the status
	deleteSnapshot, err := ctrl.backupdriverClient.DeleteSnapshots(deleteSnapshotNs).Get(ctx, deleteSnapshotName, metav1.GetOptions{})
	if err != nil {
		ctrl.logger.Errorf("updateDeleteSnapshotStatusPhase: Failed to retrieve the latest DeleteSnapshot state, error: %v", err)
		return nil, err
	}

	if deleteSnapshot.Status.Phase == newPhase {
		ctrl.logger.Debugf("updateDeleteSnapshotStatusPhase: Snapshot %s/%s already updated with %s", deleteSnapshot.Namespace, deleteSnapshot.Name, newPhase)
		return deleteSnapshot, nil
	}

	deleteSnapshotClone := deleteSnapshot.DeepCopy()
	deleteSnapshotClone.Status.Phase = newPhase
	if msg, ok := deleteSnapshotStatusFields["Message"]; ok {
		deleteSnapshotClone.Status.Message = msg.(string)
	}

	if newPhase == backupdriverapi.DeleteSnapshotPhaseCompleted {
		deleteSnapshotClone.Status.CompletionTimestamp = &metav1.Time{Time: time.Now()}
	}

	updatedDeleteSnapshot, err := ctrl.backupdriverClient.DeleteSnapshots(deleteSnapshotClone.Namespace).UpdateStatus(context.TODO(), deleteSnapshotClone, metav1.UpdateOptions{})
	if err != nil {
		ctrl.logger.Errorf("updateDeleteSnapshotStatusPhase: update status for deleteSnapshot %s/%s failed: %v", deleteSnapshotClone.Namespace, deleteSnapshotClone.Name, err)
		return nil, err
	}
	ctrl.logger.Infof("updateDeleteSnapshotStatusPhase: DeleteSnapshot %s/%s updated phase from %s to %s",
		updatedDeleteSnapshot.Namespace, updatedDeleteSnapshot.Name, deleteSnapshot.Status.Phase, updatedDeleteSnapshot.Status.Phase)
	return updatedDeleteSnapshot, nil
}

func (ctrl *backupDriverController) updateCloneFromSnapshotStatusPhase(ctx context.Context, cloneFromSnapshotNs string, cloneFromSnapshotName string, newPhase backupdriverapi.ClonePhase, cloneFromSnapshotStatusFields map[string]interface{}) (*backupdriverapi.CloneFromSnapshot, error) {
	ctrl.logger.Debugf("Entering updateCloneFromSnapshotStatusPhase: %s/%s, Phase %s", cloneFromSnapshotNs, cloneFromSnapshotName, newPhase)

	// Retrieve the latest version of CloneFromSnapshots CRD and update the status
	cloneFromSnapshot, err := ctrl.backupdriverClient.CloneFromSnapshots(cloneFromSnapshotNs).Get(ctx, cloneFromSnapshotName, metav1.GetOptions{})
	if err != nil {
		ctrl.logger.Errorf("updateCloneFromSnapshotStatusPhase: Failed to retrieve the latest CloneFromSnapshot state, error: %v", err)
		return nil, err
	}

	if cloneFromSnapshot.Status.Phase == newPhase {
		ctrl.logger.Debugf("updateCloneFromSnapshotStatusPhase: Snapshot %s/%s already updated with %s", cloneFromSnapshot.Namespace, cloneFromSnapshot.Name, newPhase)
		return cloneFromSnapshot, nil
	}

	cloneFromSnapshotClone := cloneFromSnapshot.DeepCopy()
	cloneFromSnapshotClone.Status.Phase = newPhase
	if msg, ok := cloneFromSnapshotStatusFields["Message"]; ok {
		cloneFromSnapshotClone.Status.Message = msg.(string)
	}

	if newPhase == backupdriverapi.ClonePhaseCompleted {
		cloneFromSnapshotClone.Status.CompletionTimestamp = &metav1.Time{Time: time.Now()}
	}

	updatedCloneFromSnapshot, err := ctrl.backupdriverClient.CloneFromSnapshots(cloneFromSnapshotClone.Namespace).UpdateStatus(context.TODO(), cloneFromSnapshotClone, metav1.UpdateOptions{})
	if err != nil {
		ctrl.logger.Errorf("updateCloneFromSnapshotStatusPhase: update status for cloneFromSnapshot %s/%s failed: %v", cloneFromSnapshotClone.Namespace, cloneFromSnapshotClone.Name, err)
		return nil, err
	}
	ctrl.logger.Infof("updateCloneFromSnapshotStatusPhase: CloneFromSnapshot %s/%s updated phase from %s to %s",
		updatedCloneFromSnapshot.Namespace, updatedCloneFromSnapshot.Name, cloneFromSnapshot.Status.Phase, updatedCloneFromSnapshot.Status.Phase)
	return updatedCloneFromSnapshot, nil
}
