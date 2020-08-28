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

// PVCDeleteItemAction is a delete item action plugin for Velero.
type NewPVCDeleteItemAction struct {
	Log logrus.FieldLogger
}

func (p *NewPVCDeleteItemAction) AppliesTo() (velero.ResourceSelector, error) {
	p.Log.Info("VSphere PVCDeleteItemAction AppliesTo")

	return velero.ResourceSelector{
		IncludedResources: []string{"persistentvolumeclaims"},
	}, nil
}

func (p *NewPVCDeleteItemAction) Execute(input *velero.DeleteItemActionExecuteInput) error {
	item := input.Item
	var pvc corev1.PersistentVolumeClaim
	ctx := context.Background()
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.UnstructuredContent(), &pvc); err != nil {
		return errors.WithStack(err)
	}

	var err error
	// get snapshot blob from PVC annotation
	snapshotAnnotation, ok := pvc.Annotations[utils.ItemSnapshotLabel]
	if !ok {
		p.Log.Infof("Skipping PVCRestoreItemAction for PVC %s/%s, PVC does not have a vSphere BackupItemAction snapshot.", pvc.Namespace, pvc.Name)
		return nil
	}

	var itemSnapshot backupdriverv1.Snapshot
	if err = pluginItem.GetSnapshotFromPVCAnnotation(snapshotAnnotation, &itemSnapshot); err != nil {
		p.Log.Errorf("Failed to parse the Snapshot object from PVC annotation: %v", err)
		return errors.WithStack(err)
	}
	p.Log.Infof("VSphere PVCDeleteItemAction for PVC %s/%s started", pvc.Namespace, pvc.Name)
	defer func() {
		p.Log.Infof("VSphere PVCDeleteItemAction for PVC %s/%s completed with err: %v", pvc.Namespace, pvc.Name, err)
	}()

	restConfig, err := rest.InClusterConfig()
	if err != nil {
		p.Log.Error("Failed to get the rest config in k8s cluster: %v", err)
		return errors.WithStack(err)
	}
	backupdriverClient, err := backupdriverTypedV1.NewForConfig(restConfig)
	if err != nil {
		return errors.WithStack(err)
	}

	snapshotID := itemSnapshot.Status.SnapshotID
	backupRepository := snapshotUtils.NewBackupRepository(itemSnapshot.Spec.BackupRepository)
	p.Log.Info("Creating a DeleteSnapshot CR")

	updatedDeleteSnapshot, err := snapshotUtils.DeleteSnapshotRef(ctx, backupdriverClient, snapshotID, pvc.Namespace, *backupRepository,
		[]backupdriverv1.DeleteSnapshotPhase{backupdriverv1.DeleteSnapshotPhaseCompleted, backupdriverv1.DeleteSnapshotPhaseFailed}, p.Log)
	if err != nil {
		p.Log.Errorf("Failed to create a DeleteSnapshot CR: %v", err)
		return errors.WithStack(err)
	}
	if updatedDeleteSnapshot.Status.Phase == backupdriverv1.DeleteSnapshotPhaseFailed {
		errMsg := fmt.Sprintf("Failed to create a DeleteSnapshot CR: Phase=Failed, err=%v", updatedDeleteSnapshot.Status.Message)
		p.Log.Error(errMsg)
		return errors.New(errMsg)
	}
	p.Log.Info("Deleted Snapshot, %v, from PVC %s/%s in the backup", updatedDeleteSnapshot, pvc.Namespace, pvc.Name)

	return nil
}
