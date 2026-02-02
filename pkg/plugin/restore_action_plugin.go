package plugin

import (
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/vmware-tanzu/velero/pkg/plugin/velero"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/restmapper"

	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/constants"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/utils"
)

// BackupItemAction is a backup item action plugin for Velero.
type NewRestoreItemAction struct {
	Log    logrus.FieldLogger
	Mapper *restmapper.DeferredDiscoveryRESTMapper
}

// AppliesTo returns information indicating that the BackupItemAction should be invoked to backup resources.
func (p *NewRestoreItemAction) AppliesTo() (velero.ResourceSelector, error) {
	p.Log.Info("VSphere RestoreItemAction AppliesTo")

	blockListConfigMap, err := utils.RetrieveBlockListConfigMap(p.Log)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			p.Log.Info("Failed to retrieve resources to block list.")
			return velero.ResourceSelector{}, errors.WithStack(err)
		}
		p.Log.Info("There is no blocklist configmap found. Assuming no resources to block.")
		blockListConfigMap = make(map[string]string)
	}

	// This RIA applies to PVC and pod by default.
	resources := []string{"persistentvolumeclaims", "pods"}
	for resourceToBlock := range blockListConfigMap {
		resources = append(resources, resourceToBlock)
	}
	for resourceToBlockOnRestore := range constants.ResourcesToBlockOnRestore {
		resources = append(resources, resourceToBlockOnRestore)
	}
	return velero.ResourceSelector{
		IncludedResources: resources,
	}, nil
}

func (p *NewRestoreItemAction) Execute(input *velero.RestoreItemActionExecuteInput) (*velero.RestoreItemActionExecuteOutput, error) {
	blocked, crdName, err := utils.IsObjectBlocked(input.ItemFromBackup, p.Mapper, p.Log) // Use ItemFromBackup here so that selflink is available
	if err != nil {
		return nil, errors.Wrap(err, "Failed during IsObjectBlocked check")
	}

	if blocked == false {
		// "images" and "nsxlbmonitors" are additional resources blocked on restore only for now
		blocked = utils.IsResourceBlockedOnRestore(crdName)
	}
	item := input.Item // Use Item for everything else so that previous actions had a chance to modify the object
	// (e.g. Velero removes extraneous metadata earlier in the restore process)

	p.Log.Infof("Restoring resource %v: blocked = %v", crdName, blocked)

	if blocked {
		// Skip the restore of blocked resources.
		p.Log.Infof("Skipping resource %s on restore", crdName)
		return &velero.RestoreItemActionExecuteOutput{
			SkipRestore: true,
		}, nil
	}

	if crdName == "pods" {
		p.Log.Info("Start to modify pod.")

		updatePod, err := p.removeAnnotationsFromPod(item)
		if err != nil {
			return nil, errors.Wrap(err, "Failed to remove annotations from Pod")
		}
		item = updatePod
	}

	if crdName == "persistentvolumeclaims" {
		p.Log.Info("Start to modify PVC.")

		var pvc corev1.PersistentVolumeClaim
		if err = runtime.DefaultUnstructuredConverter.FromUnstructured(item.UnstructuredContent(), &pvc); err != nil {
			return nil, errors.WithStack(err)
		}

		// Remove the volume health annotations before the restore
		// because vSphere CSI driver has a webhook that prevents this annotation
		// from being created/updated by a non-CSI driver system service account
		healthAnnotations := []string{constants.AnnVolumeHealth, constants.AnnVolumeHealthTS}
		utils.RemoveAnnotations(&pvc.ObjectMeta, healthAnnotations)

		// Update item with the PVC without the volume health annotations
		// so that Velero can create a PVC without being rejected by the
		// vSphere CSI driver webhook in the case of Skipping PVCRestoreItemAction
		pvcMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&pvc)
		if err != nil {
			p.Log.Errorf("Error converting the pvc %s/%s to unstructured. Error: %v", pvc.Namespace, pvc.Name, err)
			return nil, errors.WithStack(err)
		}
		item = &unstructured.Unstructured{Object: pvcMap}
	}

	return &velero.RestoreItemActionExecuteOutput{UpdatedItem: item}, nil
}

func (p *NewRestoreItemAction) removeAnnotationsFromPod(item runtime.Unstructured) (runtime.Unstructured, error) {
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

		return &unstructured.Unstructured{Object: podMap}, nil
	}

	return item, nil
}
