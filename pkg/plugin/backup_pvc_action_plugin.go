package plugin

import (
	"context"
	"fmt"
	"os"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	backupdriverv1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/backupdriver/v1alpha1"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/backuprepository"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/buildinfo"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/constants"
	backupdriverTypedV1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned/typed/backupdriver/v1alpha1"
	pluginUtil "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/plugin/util"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/snapshotUtils"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/utils"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"github.com/vmware-tanzu/velero/pkg/plugin/velero"
	"github.com/vmware-tanzu/velero/pkg/util/boolptr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
)

// PVCBackupItemAction is a backup item action plugin for Velero.
type NewPVCBackupItemAction struct {
	Log logrus.FieldLogger
}

// AppliesTo returns information indicating that the PVCBackupItemAction should be invoked to backup PVCs.
func (p *NewPVCBackupItemAction) AppliesTo() (velero.ResourceSelector, error) {
	p.Log.Info("VSphere PVCBackupItemAction AppliesTo")

	return velero.ResourceSelector{
		IncludedResources: pluginUtil.GetResources(),
	}, nil
}

// Execute recognizes PVCs backed by volumes provisioned by vSphere CNS block volumes
func (p *NewPVCBackupItemAction) Execute(item runtime.Unstructured, backup *velerov1.Backup) (runtime.Unstructured, []velero.ResourceIdentifier, error) {
	// Do nothing if volume snapshots have not been requested in this backup
	ctx := context.Background()
	if boolptr.IsSetToFalse(backup.Spec.SnapshotVolumes) {
		p.Log.Infof("Volume snapshotting not requested for backup %s/%s", backup.Namespace, backup.Name)
		return item, nil, nil
	}

	blocked, crdName, err := pluginUtil.IsObjectBlocked(item)
	if err != nil {
		return nil, nil, errors.Wrap(err, "Failed during IsObjectBlocked check")
	}
	p.Log.Infof("Backing up resource %v: blocked = %v", crdName, blocked)

	if blocked {
		return nil, nil, errors.Errorf("Resource CRD %s is blocked in backup, skipping", crdName)
	}

	var pvc corev1.PersistentVolumeClaim
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.UnstructuredContent(), &pvc); err != nil {
		return nil, nil, errors.WithStack(err)
	}
	storageClassName := ""
	if pvc.Spec.StorageClassName != nil {
		storageClassName = *pvc.Spec.StorageClassName
	} else {
		p.Log.Infof("VSphere PVCBackupItemAction: StorageClass is not set for PVC %s/%s", pvc.Namespace, pvc.Name)
	}
	p.Log.Infof("VSphere PVCBackupItemAction for PVC %s/%s started. Storage Class Name: %s", pvc.Namespace, pvc.Name, storageClassName)

	defer func() {
		p.Log.Infof("VSphere PVCBackupItemAction for PVC %s/%s completed with err: %v", pvc.Namespace, pvc.Name, err)
	}()

	// get the velero namespace and the rest config in k8s cluster
	veleroNs, exist := os.LookupEnv("VELERO_NAMESPACE")
	if !exist {
		errMsg := "Failed to lookup the ENV variable for velero namespace"
		p.Log.Error(errMsg)
		return nil, nil, errors.New(errMsg)
	}

	restConfig, err := utils.GetKubeClientConfig()
	if err != nil {
		p.Log.Errorf("Failed to get the rest config in k8s cluster: %v", err)
		return nil, nil, errors.WithStack(err)
	}

	kubeClient, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		p.Log.Errorf("failed to get the kubeclient: %v", err)
		return nil, nil, errors.WithStack(err)
	}

	p.Log.Infof("Fetching underlying PV for PVC %s", fmt.Sprintf("%s/%s", pvc.Namespace, pvc.Name))
	// Do nothing if this is not a vSphere CSI provisioned volume
	pv, err := pluginUtil.GetPVForPVC(&pvc, kubeClient.CoreV1())
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}

	// Do nothing if restic is used to backup this PV
	isResticUsed, err := pluginUtil.IsPVCBackedUpByRestic(pvc.Namespace, pvc.Name, kubeClient.CoreV1(), boolptr.IsSetToTrue(backup.Spec.DefaultVolumesToRestic))
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	if isResticUsed {
		p.Log.Infof("Skipping PVC %s/%s, PV %s will be backed up using restic", pvc.Namespace, pvc.Name, pv.Name)
		return item, nil, nil
	}

	if utils.IsFeatureEnabled(kubeClient, constants.CSIMigratedVolumeSupportFlag, false, p.Log) {
		if pv.Spec.PersistentVolumeSource.CSI == nil && !pluginUtil.IsMigratedCSIVolume(pv) {
			msg := fmt.Sprintf("skipping %s/%s, associated PV %s is not a CSI volume, nor a migrated volume", pvc.Namespace, pvc.Name, pv.Name)
			p.Log.Infof(msg)
			return nil, nil, errors.New(msg)
		}

		if pv.Spec.PersistentVolumeSource.CSI != nil && pv.Spec.PersistentVolumeSource.CSI.Driver != constants.VSphereCSIDriverName {
			msg := fmt.Sprintf("skipping %s/%s, associated PV %s is not a vSphere CSI volume", pvc.Namespace, pvc.Name, pv.Name)
			p.Log.Infof(msg)
			return nil, nil, errors.New(msg)
		}
	} else {
		if pv.Spec.PersistentVolumeSource.CSI == nil {
			p.Log.Infof("Skipping PVC %s/%s, associated PV %s is not a CSI volume", pvc.Namespace, pvc.Name, pv.Name)
			return item, nil, nil
		}

		if pv.Spec.PersistentVolumeSource.CSI.Driver != constants.VSphereCSIDriverName {
			p.Log.Infof("Skipping PVC %s/%s, associated PV %s is not a vSphere CSI volume", pvc.Namespace, pvc.Name, pv.Name)
			return item, nil, nil
		}
	}

	backupdriverClient, err := backupdriverTypedV1.NewForConfig(restConfig)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}

	// Do not claim a backup repository in local mode
	var backupRepositoryName string
	isLocalMode := utils.IsFeatureEnabled(kubeClient, constants.VSphereLocalModeFlag, false, p.Log)
	if !isLocalMode {
		p.Log.Info("Claiming backup repository during backup")
		bslName := backup.Spec.StorageLocation
		backupRepositoryName, err = backuprepository.RetrieveBackupRepositoryFromBSL(ctx, bslName, pvc.Namespace, veleroNs, backupdriverClient, restConfig, p.Log)
		if err != nil {
			p.Log.Errorf("Failed to retrieve backup repository name: %v", err)
			return nil, nil, errors.WithStack(err)
		}
	}

	backupRepository := snapshotUtils.NewBackupRepository(backupRepositoryName)

	objectToSnapshot := corev1.TypedLocalObjectReference{
		APIGroup: &corev1.SchemeGroupVersion.Group,
		Kind:     pvc.Kind,
		Name:     pvc.Name,
	}

	labels := map[string]string{
		constants.SnapshotBackupLabel: backup.Name,
	}

	p.Log.Info("Creating a Snapshot CR")
	updatedSnapshot, err := snapshotUtils.SnapshotRef(ctx, backupdriverClient, objectToSnapshot, pvc.Namespace, *backupRepository, labels,
		[]backupdriverv1.SnapshotPhase{backupdriverv1.SnapshotPhaseSnapshotted, backupdriverv1.SnapshotPhaseSnapshotFailed, backupdriverv1.SnapshotPhaseUploaded, backupdriverv1.SnapshotPhaseUploading, backupdriverv1.SnapshotPhaseUploadFailed, backupdriverv1.SnapshotPhaseCanceling, backupdriverv1.SnapshotPhaseCanceled, backupdriverv1.SnapshotPhaseCleanupFailed}, p.Log)
	if err != nil {
		p.Log.Errorf("Failed to create a Snapshot CR: %v", err)
		return nil, nil, errors.WithStack(err)
	}
	if updatedSnapshot.Status.Phase == backupdriverv1.SnapshotPhaseSnapshotFailed {
		errMsg := fmt.Sprintf("Failed to create a Snapshot CR: Phase=SnapshotFailed, err=%v", updatedSnapshot.Status.Message)
		p.Log.Error(errMsg)
		return nil, nil, errors.New(errMsg)
	} else if updatedSnapshot.Status.Phase == backupdriverv1.SnapshotPhaseUploadFailed {
		errMsg := fmt.Sprintf("Failed to upload a Snapshot: Phase=UploadFailed, err=%v", updatedSnapshot.Status.Message)
		p.Log.Error(errMsg)
		return nil, nil, errors.New(errMsg)
	} else if updatedSnapshot.Status.Phase == backupdriverv1.SnapshotPhaseCanceled {
		errMsg := fmt.Sprintf("Snapshot upload canceled: Phase=Canceled, err=%v", updatedSnapshot.Status.Message)
		p.Log.Error(errMsg)
		return nil, nil, errors.New(errMsg)
	} else if updatedSnapshot.Status.Phase == backupdriverv1.SnapshotPhaseCleanupFailed {
		errMsg := fmt.Sprintf("Failed to clean up a local snapshot after upload: Phase=CleanupAfterUploadFailed, err=%v", updatedSnapshot.Status.Message)
		p.Log.Error(errMsg)
		// Since upload is successful, log an error but don't return error so that backup will be considered successful
		// return nil, nil, errors.New(errMsg)
	}

	p.Log.Infof("Persisting snapshot with snapshotID :%s under label: %s Snapshot: %v", updatedSnapshot.Status.SnapshotID, constants.ItemSnapshotLabel, updatedSnapshot)
	// Persist the snapshot blob as an annotation of PVC
	snapshotAnnotation, err := pluginUtil.GetAnnotationFromSnapshot(updatedSnapshot)
	if err != nil {
		p.Log.Errorf("Failed to marshal Snapshot object: %v", err)
		return nil, nil, errors.WithStack(err)
	}
	vals := map[string]string{
		constants.ItemSnapshotLabel:  snapshotAnnotation,
		constants.PluginVersionLabel: buildinfo.Version,
	}
	pluginUtil.AddAnnotations(&pvc.ObjectMeta, vals)

	p.Log.Info("Snapshot completed in plugin")

	var additionalItems []velero.ResourceIdentifier

	pvcMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&pvc)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}

	return &unstructured.Unstructured{Object: pvcMap}, additionalItems, nil
}
