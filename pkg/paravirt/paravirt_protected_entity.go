package paravirt

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/vmware-tanzu/astrolabe/pkg/astrolabe"
	"github.com/vmware-tanzu/astrolabe/pkg/pvc"
	backupdriverv1api "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/backupdriver/v1alpha1"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/constants"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/snapshotUtils"
	"io"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

/*
ParaVirtProtectedEntity implements a Protected Entity interface to Paravirtualized objects,
including, Persistent Volumes, VMs, Persistence Services, etc., in the Paravirtualized model
in vSphere Project Pacific. Paravirtualized Persistent Volume would be the first use case,
but the ParaVirtProtectedEntity would be implemented to be extensible for other objects.

ParaVirtProtectedEntity has no component PEs as IVDProtectedEntity does. Specifically, the
whole Paravirtualized Backup Driver stack will serve as the backend of ParaVirtProtectedEntity.
*/

type ParaVirtProtectedEntity struct {
	pvpetm   *ParaVirtProtectedEntityTypeManager
	id       astrolabe.ProtectedEntityID
	data     []astrolabe.DataTransport
	metadata []astrolabe.DataTransport
	combined []astrolabe.DataTransport
	logger   logrus.FieldLogger
}

// Object methods

func (this ParaVirtProtectedEntity) GetInfo(ctx context.Context) (astrolabe.ProtectedEntityInfo, error) {
	var name string
	var err error
	if this.pvpetm.entityType == ParaVirtEntityTypePersistentVolume {
		name, err = this.getVolumeHandleFromPV()
		if err != nil {
			return nil, errors.WithStack(err)
		}
	}

	return astrolabe.NewProtectedEntityInfo(this.id, name, this.data, this.metadata, this.combined, []astrolabe.ProtectedEntityID{}), nil
}

func (this ParaVirtProtectedEntity) GetCombinedInfo(ctx context.Context) ([]astrolabe.ProtectedEntityInfo, error) {
	paravirtIPE, err := this.GetInfo(ctx)
	if err != nil {
		return nil, err
	}
	return []astrolabe.ProtectedEntityInfo{paravirtIPE}, nil
}

func (this ParaVirtProtectedEntity) Snapshot(ctx context.Context, params map[string]map[string]interface{}) (astrolabe.ProtectedEntitySnapshotID, error) {
	this.logger.Infof("CreateSnapshot called on Para-virtualized Protected Entity, %v", this.id.String())
	// create snapshot CR using snapshot utils
	peInfo, err := this.GetInfo(context.TODO())
	if err != nil {
		this.logger.Errorf("Failed to get info for ParaVirtProtectedEntity %v", this.id.String())
		return astrolabe.ProtectedEntitySnapshotID{}, errors.WithStack(err)
	}
	objectToSnapshot := corev1.TypedLocalObjectReference{
		APIGroup: &corev1.SchemeGroupVersion.Group,
		Kind:     "PersistentVolumeClaim", // PVC kind
		Name:     peInfo.GetName(),        // Supervisor PVC Name, i.e., Guest PV CSI VolumeHandle
	}

	backupRepositoryName, ok := params[astrolabe.PvcPEType][constants.SnapshotParamBackupRepository].(string)
	if !ok {
		backupRepositoryName = "INVALID_BR_NAME"
	}
	labels := map[string]string{
		constants.SnapshotBackupLabel: params[astrolabe.PvcPEType][constants.SnapshotParamBackupName].(string),
	}

	this.logger.Info("Creating a snapshot CR")
	backupRepository := snapshotUtils.NewBackupRepository(backupRepositoryName)
	snapshot, err := snapshotUtils.SnapshotRef(ctx, this.pvpetm.svcBackupDriverClient, objectToSnapshot, this.pvpetm.svcNamespace,
		*backupRepository, labels, []backupdriverv1api.SnapshotPhase{backupdriverv1api.SnapshotPhaseSnapshotted, backupdriverv1api.SnapshotPhaseSnapshotFailed}, this.logger)
	if err != nil {
		this.logger.Errorf("Failed to create a snapshot CR: %v", err)
		return astrolabe.ProtectedEntitySnapshotID{}, err
	}
	//Check Snapshot state to see if it failed.
	if snapshot.Status.Phase == backupdriverv1api.SnapshotPhaseSnapshotFailed {
		errMsg := fmt.Sprintf("ParaVirtProtectedEntity: Failed to create Snapshot for %s State: %s", objectToSnapshot.Name, backupdriverv1api.SnapshotPhaseSnapshotFailed)
		this.logger.Error(errMsg)
		return astrolabe.ProtectedEntitySnapshotID{}, errors.New(errMsg)
	}
	// Return the supervisor snapshot name as part of the param map.
	params[astrolabe.PvcPEType][constants.SnapshotParamSvcSnapshotName] = snapshot.Name

	this.logger.Infof("Supervisor snapshot status detected as done, extracted snapshotID : %s", snapshot.Status.SnapshotID)
	peIdFromSnap, err := astrolabe.NewProtectedEntityIDFromString(snapshot.Status.SnapshotID)
	if err != nil {
		this.logger.Errorf("Failed to retrieve pe-id from the snapshot CR: %v", err)
		return astrolabe.ProtectedEntitySnapshotID{}, err
	}
	return peIdFromSnap.GetSnapshotID(), nil
}

func (this ParaVirtProtectedEntity) ListSnapshots(ctx context.Context) ([]astrolabe.ProtectedEntitySnapshotID, error) {
	this.logger.Infof("ParaVirtProtectedEntity: ListSnapshots called on Para-virtualized Protected Entity, %v, Returning empty list", this.id.String())
	returnIDs := make([]astrolabe.ProtectedEntitySnapshotID, 0)
	return returnIDs, nil
}

func (this ParaVirtProtectedEntity) DeleteSnapshot(
	ctx context.Context,
	snapshotToDelete astrolabe.ProtectedEntitySnapshotID,
	params map[string]map[string]interface{}) (bool, error) {
	this.logger.Infof("ParaVirtProtectedEntity: DeleteSnapshot called on Paravirtualized Protected Entity, %v snapshotToDelete: %s", this.id.String(), snapshotToDelete.String())
	var err error
	backupRepositoryName, ok := params[astrolabe.PvcPEType]["BackupRepositoryName"].(string)
	if !ok {
		backupRepositoryName = "INVALID_BR_NAME"
	}
	deleteSnapshotName, ok := params[astrolabe.PvcPEType]["DeleteSnapshotName"].(string)
	if !ok {
		deleteSnapshotName = "INVALID_DELETE_SNAPSHOT_NAME"
	}

	peIDName := this.GetID().GetID()
	// Reconstruct the snapshot-id to delete.
	peID := astrolabe.NewProtectedEntityIDWithNamespaceAndSnapshot(
		astrolabe.PvcPEType,
		peIDName,
		this.pvpetm.svcNamespace,
		snapshotToDelete.String())
	this.logger.Infof("ParaVirtProtectedEntity: Reconstructed peID: %s", peID.String())

	backupRepository := snapshotUtils.NewBackupRepository(backupRepositoryName)
	svcDeleteSnap, err := snapshotUtils.DeleteSnapshotRef(ctx, this.pvpetm.svcBackupDriverClient, peID.String(), this.pvpetm.svcNamespace, *backupRepository,
		[]backupdriverv1api.DeleteSnapshotPhase{backupdriverv1api.DeleteSnapshotPhaseCompleted, backupdriverv1api.DeleteSnapshotPhaseFailed}, this.logger)
	if err != nil {
		this.logger.Errorf("Failed to create a DeleteSnapshot CR: %v", err)
		return false, err
	}
	this.logger.Infof("Created Supervisor DeleteSnapshot: %s for " +
		"Guest DeleteSnapshot: %s", svcDeleteSnap.Name, deleteSnapshotName)
	return true, nil
}

func (this ParaVirtProtectedEntity) GetInfoForSnapshot(ctx context.Context, snapshotID astrolabe.ProtectedEntitySnapshotID) (*astrolabe.ProtectedEntityInfo, error) {
	panic("implement me")
}

func (this ParaVirtProtectedEntity) GetComponents(ctx context.Context) ([]astrolabe.ProtectedEntity, error) {
	return make([]astrolabe.ProtectedEntity, 0), nil
}

func (this ParaVirtProtectedEntity) GetID() astrolabe.ProtectedEntityID {
	return this.id
}

func (this ParaVirtProtectedEntity) GetDataReader(ctx context.Context) (io.ReadCloser, error) {
	panic("implement me")
}

func (this ParaVirtProtectedEntity) GetMetadataReader(ctx context.Context) (io.ReadCloser, error) {
	panic("implement me")
}

func (this ParaVirtProtectedEntity) Overwrite(ctx context.Context, sourcePE astrolabe.ProtectedEntity,
	params map[string]map[string]interface{}, overwriteComponents bool) error {
	panic("implement me")
}

func (this ParaVirtProtectedEntity) getVolumeHandleFromPV() (string, error) {
	pv, err := this.pvpetm.gcKubeClientSet.CoreV1().PersistentVolumes().Get(context.TODO(), this.id.GetID(), metav1.GetOptions{})
	if err != nil {
		return "", errors.Wrapf(err, "Could not retrieve pv with name %s", this.id.GetID())
	}

	if pv.Spec.CSI == nil || pv.Spec.CSI.Driver != pvc.VSphereCSIProvisioner {
		return "", errors.Errorf("Unexpected pv %s retrieved", this.id.GetID())
	}

	return pv.Spec.CSI.VolumeHandle, nil
}

func newParaVirtProtectedEntity(pvpetm *ParaVirtProtectedEntityTypeManager, id astrolabe.ProtectedEntityID) (ParaVirtProtectedEntity, error) {
	data, metadata, combined, err := pvpetm.getDataTransports(id)
	if err != nil {
		return ParaVirtProtectedEntity{}, err
	}
	newIPE := ParaVirtProtectedEntity{
		pvpetm:   pvpetm,
		id:       id,
		data:     data,
		metadata: metadata,
		combined: combined,
		logger:   pvpetm.logger,
	}
	return newIPE, nil
}
