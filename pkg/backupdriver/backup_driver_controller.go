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

	backupdriverclientset "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned/typed/backupdriver/v1"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/vmware-tanzu/astrolabe/pkg/astrolabe"
	backupdriverapi "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/backupdriver/v1"
)

// CreateSnapshot creates a snapshot of the specified volume, and applies any provided
// set of tags to the snapshot.
func (ctrl *backupDriverController) CreateSnapshot(snapshot *backupdriverapi.Snapshot) error {
	ctrl.logger.Infof("Entering CreateSnapshot: %s/%s", snapshot.Namespace, snapshot.Name)

	var snapshotID string

	// TODO: Update code when pvPVC code is available
	clusterFlavor, _ := utils.GetClusterFlavor(nil)
	if clusterFlavor == utils.TkgGuest {
		err := ctrl.PVCreateSnapshot(snapshot)
		if err != nil {
			ctrl.logger.WithError(err).Error("Failed to create Para Virtual snapshot")
			return err
		}
		// TODO: snapshotID
	} else {
		objName := snapshot.Spec.TypedLocalObjectReference.Name
		objKind := snapshot.Spec.TypedLocalObjectReference.Kind
		if objKind != "PersistentVolumeClaim" {
			errMsg := fmt.Sprintf("resourceHandle Kind %s is not supported. Only PersistentVolumeClaim Kind is supported", objKind)
			ctrl.logger.Error(errMsg)
			return errors.New(errMsg)
		}

		pvc, err := ctrl.pvcLister.PersistentVolumeClaims(snapshot.Namespace).Get(objName)
		if err != nil {
			errMsg := fmt.Sprintf("pvc %s/%s not found in the informer cache: %v", snapshot.Namespace, objName, err)
			ctrl.logger.Error(errMsg)
			return err
		}

		pv, err := ctrl.pvLister.Get(pvc.Spec.VolumeName)
		if err != nil {
			errMsg := fmt.Sprintf("pv %s not found in the informer cache: %v", pvc.Spec.VolumeName, err)
			ctrl.logger.Error(errMsg)
			return err
		}

		if nil == pv.Spec.PersistentVolumeSource.CSI {
			errMsg := fmt.Sprintf("the CSI PersistentVolumeSource cannot be nil in PV %s", pv.Name)
			ctrl.logger.Error(errMsg)
			return errors.New(errMsg)
		}

		volumeID := pv.Spec.PersistentVolumeSource.CSI.VolumeHandle

		// call SnapshotMgr CreateSnapshot API
		peID := astrolabe.NewProtectedEntityID("ivd", volumeID)
		ctrl.logger.Infof("CreateSnapshot: The initial Astrolabe PE ID: %s", peID)
		if ctrl.snapManager == nil {
			errMsg := fmt.Sprintf("snapManager is not initialized.")
			ctrl.logger.Error(errMsg)
			return errors.New(errMsg)
		}

		// NOTE: tags is required to call snapManager.CreateSnapshot
		// but it is not really used
		var tags map[string]string

		peID, err = ctrl.snapManager.CreateSnapshot(peID, tags)
		if err != nil {
			errMsg := fmt.Sprintf("failed at calling SnapshotManager CreateSnapshot from peID %v", peID)
			ctrl.logger.Error(errMsg)
			return err
		}

		// Construct the snapshotID for cns volume
		snapshotID = peID.String()
		ctrl.logger.Infof("CreateSnapshot: The snapshotID depends on the Astrolabe PE ID in the format, <peType>:<id>:<snapshotID>, %s", snapshotID)
	}

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
	// NOTE: snapshotClone.Status.Metadata is not populated yet

	if ctrl.backupdriverClient == nil {
		errMsg := fmt.Sprintf("backupdriverClient is not initialized")
		ctrl.logger.Error(errMsg)
		return errors.New(errMsg)
	}
	snapshot, err := ctrl.backupdriverClient.Snapshots(snapshotClone.Namespace).UpdateStatus(snapshotClone)
	if err != nil {
		return err
	}

	ctrl.logger.Infof("CreateSnapshot %s/%s completed with snapshotID: %s, phase in status updated to %s", snapshot.Namespace, snapshot.Name, snapshotID, snapshot.Status.Phase)
	return nil
}

/*
 * TEMP Function. Move it to pvPVC once it is implemented
 */
func (ctrl *backupDriverController) PVCreateSnapshot(snapshot *backupdriverapi.Snapshot) error {
	// For guest cluster, create snapshot in supervisor namespace.
	// Temp code. Move to pvPVC
	svcConfig, err := utils.SupervisorConfig()
	if err != nil {
		ctrl.logger.WithError(err).Error("Failed to get the rest config in k8s cluster")
		return err
	}
	svcClient, err := backupdriverclientset.NewForConfig(svcConfig)
	if err != nil {
		ctrl.logger.WithError(err).Error("Failed to get the Supervisor config in k8s cluster")
		return err
	}

	ctx := context.Background()
	pvParams := make(map[string]string)
	err = utils.RetrievePvCredentials(pvParams, ctrl.logger)
	if err != nil {
		ctrl.logger.WithError(err).Error("Failed to get guest pv parameters")
		return err
	}
	ns := pvParams[utils.PvNamespaceParamKey]

	// Create BackupRepositoryClaim if not present
	ctrl.logger.Infof("Claiming backup repository for snapshot in Supervisor namespace %s", ns)

	guestBR, err := ctrl.backupdriverClient.BackupRepositories().Get(snapshot.Spec.BackupRepository, metav1.GetOptions{})
	if err != nil {
		ctrl.logger.WithError(err).Errorf("Failed to get the BackupRepository: %s", snapshot.Spec.BackupRepository)
		return err
	}

	// TODO: uncomment when updated ClaimBackupRepository is available
	/*
		svcBbackupRepository, err := ClaimBackupRepository(ctx, guestBR.RepositoryDriver, guestBR.RepositoryParameters,
			[]string{ns}, ns, svcClient, ctrl.logger)
		if err != nil {
			ctrl.logger.Errorf("Failed to claim backup repository: %v", err)
			return err
		}
	*/

	// TODO: Replace with svcBR.Name
	backupRepository := NewBackupRepository(guestBR.Name)
	_, err = SnapshopRef(ctx, svcClient, snapshot.Spec.TypedLocalObjectReference, ns, *backupRepository, []backupdriverapi.SnapshotPhase{backupdriverapi.SnapshotPhaseSnapshotted}, ctrl.logger)
	if err != nil {
		ctrl.logger.WithError(err).Errorf("Failed to create a snapshot CR in Supervisor Namespace: %s", ns)
		return err
	}

	ctrl.logger.Info("Snapshot is created in Supervisor Namespace")

	return nil
}
