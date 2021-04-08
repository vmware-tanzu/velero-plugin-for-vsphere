package plugin

import (
	"context"
	"fmt"
	"os"

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
)

// PVCBackupItemAction is a backup item action plugin for Velero.
type NewPVCRestoreItemAction struct {
	Log logrus.FieldLogger
}

// AppliesTo returns information indicating that the PVCBackupItemAction should be invoked to backup PVCs.
func (p *NewPVCRestoreItemAction) AppliesTo() (velero.ResourceSelector, error) {
	p.Log.Info("VSphere PVCRestoreItemAction AppliesTo")

	resources := []string{"persistentvolumeclaims"}
	for resourceToBlock, _ := range constants.ResourcesToBlock {
		resources = append(resources, resourceToBlock)
	}
	for resourceToBlockOnRestore, _ := range constants.ResourcesToBlockOnRestore {
		resources = append(resources, resourceToBlockOnRestore)
	}
	return velero.ResourceSelector{
		IncludedResources: resources,
	}, nil
}

func (p *NewPVCRestoreItemAction) Execute(input *velero.RestoreItemActionExecuteInput) (*velero.RestoreItemActionExecuteOutput, error) {
	blocked, crdName, err := utils.IsObjectBlocked(input.ItemFromBackup) // Use ItemFromBackup here so that selflink is available
	if err != nil {
		return nil, errors.Wrap(err, "Failed during IsObjectBlocked check")
	}

	if blocked == false {
		// "pods", "images" and "nsxlbmonitors" are additional resources
		// blocked on restore only for now
		blocked = utils.IsResourceBlockedOnRestore(crdName)
	}
	item := input.Item // Use Item for everything else so that previous actions had a chance to modify the object
	// (e.g. Velero removes extraneous metadata earlier in the restore process)

	p.Log.Infof("Restoring resource %v: blocked = %v", crdName, blocked)

	if blocked {
		if crdName == "pods" {
			return p.createPod(item)
		} else if utils.IsResourceBlockedOnRestore(crdName) {
			// Skip the restore of image and nsxlbmonitor resources on Supervisor Cluster
			p.Log.Infof("Skipping resource %s on restore", crdName)
			return &velero.RestoreItemActionExecuteOutput{
				SkipRestore: true,
			}, nil
		}
		return nil, errors.Errorf("Resource CRD %s is blocked in restore, skipping", crdName)
	}

	var pvc corev1.PersistentVolumeClaim
	if err = runtime.DefaultUnstructuredConverter.FromUnstructured(item.UnstructuredContent(), &pvc); err != nil {
		return nil, errors.WithStack(err)
	}

	// exit early if the RestorePVs option is disabled
	p.Log.Info("Checking if the RestorePVs option is disabled in the Restore Spec")
	if input.Restore.Spec.RestorePVs != nil && *input.Restore.Spec.RestorePVs == false {
		p.Log.Infof("Skipping PVCRestoreItemAction for PVC %s/%s since the RestorePVs option is disabled in the Restore Spec.", pvc.Namespace, pvc.Name)
		return &velero.RestoreItemActionExecuteOutput{
			UpdatedItem: input.Item,
		}, nil
	}

	// get snapshot blob from PVC annotation
	p.Log.Info("Getting the snapshot blob from PVC annotation from backup")
	snapshotAnnotation, ok := pvc.Annotations[constants.ItemSnapshotLabel]

	if !ok {
		p.Log.Infof("Skipping PVCRestoreItemAction for PVC %s/%s, PVC does not have a vSphere BackupItemAction snapshot.", pvc.Namespace, pvc.Name)
		return &velero.RestoreItemActionExecuteOutput{
			UpdatedItem: item,
		}, nil
	}
	var itemSnapshot backupdriverv1.Snapshot
	if err = pluginItem.GetSnapshotFromPVCAnnotation(snapshotAnnotation, &itemSnapshot); err != nil {
		p.Log.Errorf("Failed to parse the Snapshot object from PVC annotation: %v", err)
		return nil, errors.WithStack(err)
	}

	// update the target pvc namespace based on the namespace mapping option in the restore spec
	p.Log.Info("Updating target PVC namespace based on the namespace mapping option in the Restore Spec")
	targetNamespace := pvc.Namespace
	if input.Restore.Spec.NamespaceMapping != nil {
		_, pvcNsMappingExists := input.Restore.Spec.NamespaceMapping[pvc.Namespace]
		if pvcNsMappingExists {
			targetNamespace = input.Restore.Spec.NamespaceMapping[pvc.Namespace]
			itemSnapshot, err = pluginItem.UpdateSnapshotWithNewNamespace(&itemSnapshot, targetNamespace)
			if err != nil {
				p.Log.Errorf("Failed to update snapshot blob based on the namespace mapping specified in the Restore Spec")
				return nil, errors.WithStack(err)
			}
			p.Log.Infof("Updated the target PVC namespace from %s to %s based on the namespace mapping in the Restore Spec", pvc.Namespace, targetNamespace)
		}
	}

	p.Log.Infof("VSphere PVCRestoreItemAction for PVC %s/%s started", targetNamespace, pvc.Name)
	defer func() {
		p.Log.Infof("VSphere PVCRestoreItemAction for PVC %s/%s completed with err: %v", targetNamespace, pvc.Name, err)
	}()

	ctx := context.Background()
	veleroNs, exist := os.LookupEnv("VELERO_NAMESPACE")
	if !exist {
		errMsg := "Failed to lookup the ENV variable for velero namespace"
		p.Log.Error(errMsg)
		return nil, errors.New(errMsg)
	}
	restConfig, err := rest.InClusterConfig()
	if err != nil {
		p.Log.Errorf("Failed to get the rest config in k8s cluster: %v", err)
		return nil, errors.WithStack(err)
	}

	backupdriverClient, err := backupdriverTypedV1.NewForConfig(restConfig)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// Retrieve storage class mapping information and update pvc StorageClassName with new name
	p.Log.Info("Retrieving storage class mapping information from configMap")
	storageClassMapping, err := pluginItem.RetrieveStorageClassMapping(restConfig, veleroNs, p.Log)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to retrieve storage class mapping information : %v", err)
		p.Log.WithError(err).Error(errMsg)
		return nil, errors.New(errMsg)
	}
	if storageClassMapping != nil && len(storageClassMapping) != 0 {
		p.Log.Info("Updating target PVC storage class based on the storage class mapping")
		itemSnapshot, err = pluginItem.UpdateSnapshotWithNewStorageClass(restConfig, &itemSnapshot, storageClassMapping, p.Log)
		if err != nil {
			p.Log.Errorf("Failed to update storage class name")
			return nil, errors.WithStack(err)
		}
	}

	snapshotID := itemSnapshot.Status.SnapshotID
	snapshotMetadata := itemSnapshot.Status.Metadata
	apiGroup := itemSnapshot.Spec.APIGroup
	kind := itemSnapshot.Spec.Kind

	backupName := input.Restore.Spec.BackupName
	bslName, err := utils.RetrieveBSLFromBackup(ctx, backupName, restConfig, p.Log)
	if err != nil {
		p.Log.Errorf("Failed to retrieve the Backup Storage Location for the Backup during restore: %v", err)
		return nil, errors.WithStack(err)
	}
	var backupRepositoryName string
	isLocalMode := utils.IsFeatureEnabled(constants.VSphereLocalModeFlag, false, p.Log)
	if !isLocalMode {
		p.Log.Info("Claiming backup repository during restore")
		backupRepositoryName, err = backuprepository.RetrieveBackupRepositoryFromBSL(ctx, bslName, pvc.Namespace, veleroNs, backupdriverClient, restConfig, p.Log)
		if err != nil {
			p.Log.Errorf("Failed to retrieve backup repository name: %v", err)
			return nil, errors.WithStack(err)
		}
	}
	backupRepository := snapshotUtils.NewBackupRepository(backupRepositoryName)

	p.Log.Info("Creating a CloneFromSnapshot CR")
	updatedCloneFromSnapshot, err := snapshotUtils.CloneFromSnapshopRef(ctx, backupdriverClient, snapshotID, snapshotMetadata, apiGroup, kind, targetNamespace, *backupRepository,
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
	p.Log.Info("Restored, %v, from PVC %s/%s in the backup to PVC %s/%s", updatedCloneFromSnapshot.Status.ResourceHandle, pvc.Namespace, pvc.Name, targetNamespace, pvc.Name)

	return &velero.RestoreItemActionExecuteOutput{
		SkipRestore: true,
	}, nil
}

func (p *NewPVCRestoreItemAction) createPod(item runtime.Unstructured) (*velero.RestoreItemActionExecuteOutput, error) {
	pod := new(corev1.Pod)
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.UnstructuredContent(), &pod); err != nil {
		return nil, errors.WithStack(err)
	}

	if metav1.HasAnnotation(pod.ObjectMeta, constants.VMwareSystemVMUUID) {
		objectMeta := metav1.ObjectMeta{
			Name:      pod.Name,
			Namespace: pod.Namespace,
			Labels:    pod.Labels,
		}

		// Only restore Pod annotations not on the list to skip
		if pod.Annotations != nil {
			for annKey, annVal := range pod.Annotations {
				skip := false
				for keyToSkip, _ := range constants.PodAnnotationsToSkip {
					if annKey == keyToSkip {
						// Skip the matching key
						p.Log.Infof("createPod: Skipping annotation %s/%s on Pod %s/%s.", annKey, annVal, pod.Namespace, pod.Name)
						skip = true
						break
					}
				}
				if skip == false {
					metav1.SetMetaDataAnnotation(&objectMeta, annKey, annVal)
					p.Log.Infof("createPod: Set annotation %s:%s on Pod %s/%s.", annKey, annVal, pod.Namespace, pod.Name)
				}
			}

			pod.ObjectMeta = objectMeta
		}

		podMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&pod)
		if err != nil {
			p.Log.Errorf("Error converting the new pod %s/%s to unstructured. Error: %v", pod.Namespace, pod.Name, err)
			return nil, errors.WithStack(err)
		}

		return &velero.RestoreItemActionExecuteOutput{
			UpdatedItem: &unstructured.Unstructured{Object: podMap},
		}, nil
	}

	return &velero.RestoreItemActionExecuteOutput{}, nil
}
