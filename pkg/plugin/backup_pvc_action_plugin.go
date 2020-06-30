package plugin

import (
	"context"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	backupdriverv1api "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/backupdriver/v1"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/backupdriver"
	backupdriverTypedV1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned/typed/backupdriver/v1"
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
	p.Log.Info("PVCBackupItemAction AppliesTo for vSphere")

	return velero.ResourceSelector{
		IncludedResources: []string{"persistentvolumeclaims"},
	}, nil
}

//const (
//	PollTimeoutForSnapshot = 5 * time.Minute
//	PollLogInterval = 30 * time.Second
//)

// Execute recognizes PVCs backed by volumes provisioned by vSphere CNS block volumes
func (p *NewPVCBackupItemAction) Execute(item runtime.Unstructured, backup *velerov1api.Backup) (runtime.Unstructured, []velero.ResourceIdentifier, error) {
	p.Log.Info("Starting PVCBackupItemAction for vSphere")

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

	repositoryParameters := make(map[string]string) // TODO: retrieve the real object store config from velero storage location
	ctx := context.Background()

	backupRepositoryCR, err := backupdriver.ClaimBackupRepository(ctx, utils.S3RepositoryDriver, repositoryParameters,
		[]string{pvc.Namespace}, veleroNs, backupdriverClient)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}

	objectToSnapshot := corev1.TypedLocalObjectReference{
		APIGroup: &corev1.SchemeGroupVersion.Group,
		Kind: pvc.Kind,
		Name: pvc.Name,
	}

	backupRepository := backupdriver.NewBackupRepository(backupRepositoryCR.Name)
	_, err = backupdriver.SnapshopRef(ctx, backupdriverClient, objectToSnapshot, pvc.Namespace, *backupRepository, []backupdriverv1api.SnapshotPhase{backupdriverv1api.SnapshotPhaseSnapshotted})
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}

	var additionalItems []velero.ResourceIdentifier

	pvcMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&pvc)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}

	return &unstructured.Unstructured{Object: pvcMap}, additionalItems, nil
}