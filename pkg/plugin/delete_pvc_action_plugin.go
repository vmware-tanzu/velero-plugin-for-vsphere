package plugin

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	backupdriverv1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/backupdriver/v1alpha1"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/backuprepository"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/constants"
	backupdriverTypedV1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned/typed/backupdriver/v1alpha1"
	pluginItem "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/plugin/util"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/snapshotUtils"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/utils"
	"github.com/vmware-tanzu/velero/pkg/plugin/velero"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"os"
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

	blocked, crdName, err := pluginItem.IsObjectBlocked(item)

	if err != nil {
		return errors.Wrap(err, "Failed during IsObjectBlocked check")
	}

	if blocked {
		return errors.Errorf("Resource CRD %s is blocked, skipping", crdName)
	}

	var pvc corev1.PersistentVolumeClaim
	ctx := context.Background()
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.UnstructuredContent(), &pvc); err != nil {
		return errors.WithStack(err)
	}

	// get snapshot blob from PVC annotation
	snapshotAnnotation, ok := pvc.Annotations[constants.ItemSnapshotLabel]
	if !ok {
		p.Log.Infof("Skipping PVCDeleteItemAction for PVC %s/%s, PVC does not have a vSphere BackupItemAction snapshot.", pvc.Namespace, pvc.Name)
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

	veleroNs, exist := os.LookupEnv("VELERO_NAMESPACE")
	if !exist {
		errMsg := "Failed to lookup the ENV variable for velero namespace"
		p.Log.Error(errMsg)
		return errors.New(errMsg)
	}
	restConfig, err := utils.GetKubeClientConfig()
	if err != nil {
		p.Log.Error("Failed to get the rest config in k8s cluster: %v", err)
		return errors.WithStack(err)
	}
	backupdriverClient, err := backupdriverTypedV1.NewForConfig(restConfig)
	if err != nil {
		return errors.WithStack(err)
	}

	snapshotID := itemSnapshot.Status.SnapshotID
	bslName := input.Backup.Spec.StorageLocation

	var backupRepositoryName string
	isLocalMode := utils.IsFeatureEnabled(constants.VSphereLocalModeFlag, false, p.Log)
	if !isLocalMode {
		p.Log.Info("Claiming backup repository during delete")
		backupRepositoryName, err = backuprepository.RetrieveBackupRepositoryFromBSL(ctx, bslName, pvc.Namespace, veleroNs, backupdriverClient, restConfig, p.Log)
		if err != nil {
			p.Log.Errorf("Failed to retrieve backup repository name: %v", err)
			return errors.WithStack(err)
		}
	}
	backupRepository := snapshotUtils.NewBackupRepository(backupRepositoryName)

	p.Log.Info("Creating a DeleteSnapshot CR")

	updatedDeleteSnapshot, err := snapshotUtils.DeleteSnapshotRef(ctx, backupdriverClient, snapshotID, veleroNs, *backupRepository,
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
	p.Log.Infof("Deleted Snapshot, %v, from PVC %s/%s in the backup", updatedDeleteSnapshot, pvc.Namespace, pvc.Name)

	return nil
}
