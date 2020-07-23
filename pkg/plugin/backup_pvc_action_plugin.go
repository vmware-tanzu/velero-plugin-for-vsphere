package plugin

import (
	"context"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	backupdriverv1api "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/backupdriver/v1"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/backupdriver"
	backupdriverTypedV1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned/typed/backupdriver/v1"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/snapshotUtils"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/utils"
	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"github.com/vmware-tanzu/velero/pkg/plugin/velero"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"os"
)

// PVCBackupItemAction is a backup item action plugin for Velero.
type NewPVCBackupItemAction struct {
	Log logrus.FieldLogger
}

// AppliesTo returns information indicating that the PVCBackupItemAction should be invoked to backup PVCs.
func (p *NewPVCBackupItemAction) AppliesTo() (velero.ResourceSelector, error) {
	p.Log.Info("VSphere PVCBackupItemAction AppliesTo")

	return velero.ResourceSelector{
		IncludedResources: []string{"persistentvolumeclaims"},
	}, nil
}

// Execute recognizes PVCs backed by volumes provisioned by vSphere CNS block volumes
func (p *NewPVCBackupItemAction) Execute(item runtime.Unstructured, backup *velerov1api.Backup) (runtime.Unstructured, []velero.ResourceIdentifier, error) {
	// Do nothing if volume snapshots have not been requested in this backup
	//if utils.IsSetToFalse(backup.Spec.SnapshotVolumes) {
	if backup.Spec.SnapshotVolumes != nil && *backup.Spec.SnapshotVolumes == false {
		p.Log.Infof("Volume snapshotting not requested for backup %s/%s", backup.Namespace, backup.Name)
		return item, nil, nil
	}

	var pvc corev1.PersistentVolumeClaim
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.UnstructuredContent(), &pvc); err != nil {
		return nil, nil, errors.WithStack(err)
	}

	p.Log.Infof("VSphere PVCBackupItemAction for PVC %s/%s started", pvc.Namespace, pvc.Name)
	var err error
	defer func() {
		p.Log.WithError(err).Infof("VSphere PVCBackupItemAction for PVC %s/%s completed", pvc.Namespace, pvc.Name)
	}()

	// get the velero namespace and the rest config in k8s cluster
	veleroNs, exist := os.LookupEnv("VELERO_NAMESPACE")
	if !exist {
		errMsg := "Failed to lookup the ENV variable for velero namespace"
		p.Log.Error(errMsg)
		return nil, nil, errors.New(errMsg)
	}

	restConfig, err := rest.InClusterConfig()
	if err != nil {
		p.Log.Error("Failed to get the rest config in k8s cluster: %v", err)
		return nil, nil, errors.WithStack(err)
	}

	backupdriverClient, err := backupdriverTypedV1.NewForConfig(restConfig)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}

	p.Log.Info("Claiming backup repository for snapshot")
	repositoryParameters := make(map[string]string) // TODO: retrieve the real object store config from velero storage location
	ctx := context.Background()

	backupRepositoryName, err := backupdriver.ClaimBackupRepository(ctx, utils.S3RepositoryDriver, repositoryParameters,
		[]string{pvc.Namespace}, veleroNs, backupdriverClient, p.Log)
	if err != nil {
		p.Log.Errorf("Failed to claim backup repository: %v", err)
		return nil, nil, errors.WithStack(err)
	}

	objectToSnapshot := corev1.TypedLocalObjectReference{
		APIGroup: &corev1.SchemeGroupVersion.Group,
		Kind: pvc.Kind,
		Name: pvc.Name,
	}

	p.Log.Info("Creating a snapshot CR")
	backupRepository := snapshotUtils.NewBackupRepository(backupRepositoryName)
	_, err = snapshotUtils.SnapshopRef(ctx, backupdriverClient, objectToSnapshot, pvc.Namespace, *backupRepository, []backupdriverv1api.SnapshotPhase{backupdriverv1api.SnapshotPhaseSnapshotted}, p.Log)
	if err != nil {
		p.Log.Errorf("Failed to create a snapshot CR: %v", err)
		return nil, nil, errors.WithStack(err)
	}

	p.Log.Info("Snapshot is completed in plugin")

	var additionalItems []velero.ResourceIdentifier

	pvcMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&pvc)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}

	return &unstructured.Unstructured{Object: pvcMap}, additionalItems, nil
}