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

	brName := snapshot.Spec.BackupRepository
	if ctrl.backupdriverClient == nil {
		errMsg := fmt.Sprintf("backupdriverClient is not initialized")
		ctrl.logger.Error(errMsg)
		return errors.New(errMsg)
	}

	if ctrl.svcKubeConfig != nil {
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

// deleteSnapshot deletes the specified volume snapshot.
func (ctrl *backupDriverController) deleteSnapshot(snapshot *backupdriverapi.Snapshot) error {
	ctrl.logger.Infof("deleteSnapshot called with snapshot %s/%s", snapshot.Namespace, snapshot.Name)
	if snapshot.Status.SnapshotID == "" {
		errMsg := fmt.Sprintf("snapshotID is required to delete snapshot")
		ctrl.logger.Error(errMsg)
		return errors.New(errMsg)
	}

	snapshotID := snapshot.Status.SnapshotID
	ctrl.logger.Infof("Calling Snapshot Manager to delete snapshot with snapshotID %s", snapshotID)
	peID, err := astrolabe.NewProtectedEntityIDFromString(snapshotID)
	if err != nil {
		ctrl.logger.WithError(err).Errorf("Fail to construct new Protected Entity ID from string %s", snapshotID)
		return err
	}

	if ctrl.snapManager == nil {
		errMsg := fmt.Sprintf("snapManager is not initialized.")
		ctrl.logger.Error(errMsg)
		return errors.New(errMsg)
	}

	err = ctrl.snapManager.DeleteSnapshot(peID)
	if err != nil {
		ctrl.logger.WithError(err).Errorf("Failed at calling SnapshotManager DeleteSnapshot for peID %v", peID)
		return err
	}

	ctrl.logger.Infof("deleteSnapshot %s/%s completed with snapshotID: %s", snapshot.Namespace, snapshot.Name, snapshotID)

	return nil
}
