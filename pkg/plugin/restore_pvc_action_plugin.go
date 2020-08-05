package plugin

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	backupdriverv1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/backupdriver/v1"
	backupdriverTypedV1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned/typed/backupdriver/v1"
	pluginItem "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/plugin/util"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/snapshotUtils"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/utils"
	"github.com/vmware-tanzu/velero/pkg/plugin/velero"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
)

// PVCBackupItemAction is a backup item action plugin for Velero.
type NewPVCRestoreItemAction struct {
	Log logrus.FieldLogger
}

// AppliesTo returns information indicating that the PVCBackupItemAction should be invoked to backup PVCs.
func (p *NewPVCRestoreItemAction) AppliesTo() (velero.ResourceSelector, error) {
	p.Log.Info("VSphere PVCBackupItemAction AppliesTo")

	return velero.ResourceSelector{
		IncludedResources: []string{"persistentvolumeclaims"},
	}, nil
}

func (p *NewPVCRestoreItemAction) Execute(input *velero.RestoreItemActionExecuteInput) (*velero.RestoreItemActionExecuteOutput, error) {
	var pvc corev1.PersistentVolumeClaim
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(input.Item.UnstructuredContent(), &pvc); err != nil {
		return nil, errors.WithStack(err)
	}

	var err error
	// get snapshot blob from PVC annotation and reset PVC annotation
	snapshotAnnotation, ok := pvc.Annotations[utils.ItemSnapshotLabel]
	if !ok {
		p.Log.Infof("Skipping PVCRestoreItemAction for PVC %s/%s, PVC does not have a vSphere BackupItemAction snapshot.", pvc.Namespace, pvc.Name)
		return &velero.RestoreItemActionExecuteOutput{
			UpdatedItem: input.Item,
		}, nil
	}

	var itemSnapshot backupdriverv1.Snapshot
	if err = pluginItem.GetSnapshotFromPVCAnnotation(snapshotAnnotation, &itemSnapshot); err != nil {
		p.Log.Errorf("Failed to parse the Snapshot object from PVC annotation: %v", err)
		return nil, errors.WithStack(err)
	}

	p.Log.Infof("VSphere PVCRestoreItemAction for PVC %s/%s started", pvc.Namespace, pvc.Name)
	defer func() {
		p.Log.Infof("VSphere PVCRestoreItemAction for PVC %s/%s completed with err: %v", pvc.Namespace, pvc.Name, err)
	}()

	ctx := context.Background()
	restConfig, err := rest.InClusterConfig()
	if err != nil {
		p.Log.Error("Failed to get the rest config in k8s cluster: %v", err)
		return nil, errors.WithStack(err)
	}
	backupdriverClient, err := backupdriverTypedV1.NewForConfig(restConfig)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	snapshotID := itemSnapshot.Status.SnapshotID
	snapshotMetadata := itemSnapshot.Status.Metadata
	apiGroup := itemSnapshot.Spec.APIGroup
	kind := itemSnapshot.Spec.Kind
	backupRepository := snapshotUtils.NewBackupRepository(itemSnapshot.Spec.BackupRepository)

	p.Log.Info("Creating a CloneFromSnapshot CR")
	updatedCloneFromSnapshot, err := snapshotUtils.CloneFromSnapshopRef(ctx, backupdriverClient, snapshotID, snapshotMetadata, apiGroup, kind, pvc.Namespace, *backupRepository,
		[]backupdriverv1.ClonePhase{backupdriverv1.ClonePhaseCompleted, backupdriverv1.ClonePhaseFailed}, p.Log)
	if err != nil {
		p.Log.Errorf("Failed to create a CloneFromSnapshot CR: %v", err)
		return nil, errors.WithStack(err)
	}
	if updatedCloneFromSnapshot.Status.Phase == backupdriverv1.ClonePhaseFailed {
		errMsg := fmt.Sprintf("Failed to create a CloneFromSnapshot CR: Phase=Failed, err=%v", updatedCloneFromSnapshot.Status.Message)
		p.Log.Error(errMsg)
		return nil, errors.New(errMsg)
	}
	p.Log.Info("Restored, %v, from PVC %s/%s in the backup", updatedCloneFromSnapshot.Status.ResourceHandle, pvc.Namespace, pvc.Name)

	return &velero.RestoreItemActionExecuteOutput{
		SkipRestore: true,
	}, nil
}

