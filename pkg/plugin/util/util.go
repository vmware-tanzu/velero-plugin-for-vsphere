package util

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	backupdriverv1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/backupdriver/v1alpha1"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/constants"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/utils"
	"github.com/vmware-tanzu/velero/pkg/restic"
	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"strings"
)

func GetSnapshotFromPVCAnnotation(snapshotAnnotation string, itemSnapshot interface{}) error {
	decodedItemSnapshot, err := base64.StdEncoding.DecodeString(snapshotAnnotation)
	if err != nil {
		return errors.WithStack(err)
	}

	err = json.Unmarshal(decodedItemSnapshot, itemSnapshot)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func GetAnnotationFromSnapshot(itemSnapshot interface{}) (string, error) {
	itemSnapshotByteArray, err := json.Marshal(itemSnapshot)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(itemSnapshotByteArray), nil
}

// AddAnnotations adds the supplied key-values to the annotations on the object
func AddAnnotations(o *metav1.ObjectMeta, vals map[string]string) {
	if o.Annotations == nil {
		o.Annotations = make(map[string]string)
	}
	for k, v := range vals {
		o.Annotations[k] = v
	}
}

func UpdateSnapshotWithNewNamespace(itemSnapshot *backupdriverv1.Snapshot, namespace string) (backupdriverv1.Snapshot, error) {
	var err error
	snapshotMetadata := itemSnapshot.Status.Metadata

	pvc := &corev1.PersistentVolumeClaim{}
	if err = pvc.Unmarshal(snapshotMetadata); err != nil {
		return backupdriverv1.Snapshot{}, err
	}

	// update the PVC namespace
	pvc.Namespace = namespace

	var updatedSnapshotMetadata []byte
	if updatedSnapshotMetadata, err = pvc.Marshal(); err != nil {
		return backupdriverv1.Snapshot{}, err
	}
	itemSnapshot.Status.Metadata = updatedSnapshotMetadata

	return *itemSnapshot, nil
}

const (
	minComponents     = 5
	k8sNamespaceIndex = 3
	crNamespaceIndex  = 4
	crGroupIndex      = 2
)

/*
 * Converts a K8S Self Link into the CRD name.  Supports namespaced and cluster level CRs
 * K8S Cluster Resource Self Link format: /api/<version>/<resource plural name>/<item name>,
 *     e.g. /api/v1/persistentvolumes/pvc-3240e5ed-9a97-446c-a6ab-b2442d852d04
 * K8S Namespace resource Self Link format: /api/<version>/namespaces/<namespace>/<resource plural name>/<item name>,
 *     e.g. /api/v1/namespaces/kibishii/persistentvolumeclaims/etcd0-pv-claim
 * K8S Resource Name = <resource plural name>, e.g. persistentvolumes
 * Custom Resource Cluster Self Link format: /apis/<CR group>/<version>/<CR plural name>/<item name>,
 *     e.g. /api/cnsdp.vmware.com/v1/backuprepositories/br-1
 * Custom Resource Namespace Self Link format: /apis/<CR group>/<version>/namespaces/<namespace>/<CR plural name>/<item name>,
 *     e.g. /apis/velero.io/v1/namespaces/velero/backups/kibishii-1
 * Custom Resource Name = <CR plural name>.<CR group>, e.g. backups.velero.io
 */
func SelfLinkToCRDName(selfLink string) string {
	components := strings.Split(selfLink, "/")
	if len(components) < minComponents || components[0] != "" || (components[1] != "apis" && components[1] != "api") {
		return ""
	}
	var pluralIndex, namespacesIndex int
	var hasGroup bool
	if components[1] == "api" {
		// If this is a K8S resource, there will be no group, so just return the plural name.
		namespacesIndex = k8sNamespaceIndex
		hasGroup = false
	} else {
		namespacesIndex = crNamespaceIndex
		hasGroup = true
	}
	if components[namespacesIndex] == "namespaces" {
		// Skip the "namespaces" and "<namespace>" component
		pluralIndex = namespacesIndex + 2
	} else {
		// no namespace components to skip
		pluralIndex = namespacesIndex
	}
	if hasGroup {
		return components[pluralIndex] + "." + components[crGroupIndex]
	} else {
		return components[pluralIndex]
	}
}

/*
 * Extracts the CRD name from a K8S Unstructured into the CRD name.  Supports namespaced and cluster level CRs
 * K8S Cluster Resource Self Link format: /api/<version>/<resource plural name>/<item name>,
 *     e.g. /api/v1/persistentvolumes/pvc-3240e5ed-9a97-446c-a6ab-b2442d852d04
 * K8S Resource Name = <resource plural name>, e.g. persistentvolumes
 * Custom Resource Cluster Self Link format: /apis/<CR group>/<version>/<CR plural name>/<item name>,
 *     e.g. /api/cnsdp.vmware.com/v1/backuprepositories/br-1
 * Custom Resource Name = <CR plural name>.<CR group>, e.g. backuprepositories.cnsdp.vmware.com
 * TODO - replace with a better system that does need singular to plural translation
 */
func UnstructuredToCRDName(item runtime.Unstructured) (string, error) {
	// Retrieve the common object metadata and check to see if this is a blocked type
	accessor := meta.NewAccessor()
	apiVersion, err := accessor.APIVersion(item)
	if err != nil {
		return "", err
	}
	var group string

	gv, err := schema.ParseGroupVersion(apiVersion)
	kind, err := accessor.Kind(item)
	if err != nil {
		return "", err
	}
	group = gv.Group

	var pluralName string
	// Singular to plural conversion is done by adding an 's' unless the kind ends in a 'y' in which case
	// y gets replaced with ies.  This is not a complete set of rules, it doesn't handle shelf->shelves for example
	// but the rules are weird.  Currently works for the resources defined, this is the place to add additional plural rules
	// if necessary
	if strings.HasSuffix(kind, "y") {
		pluralName = strings.ToLower(kind)[0:len(kind)-1] + "ies"
	} else {
		pluralName = strings.ToLower(kind) + "s"
	}
	if group != "" {
		return pluralName + "." + group, nil
	}
	return pluralName, nil
}

func GetKubeClient(config *rest.Config, logger logrus.FieldLogger) (*kubernetes.Clientset, error) {
	var err error
	if config == nil {
		config, err = utils.GetKubeClientConfig()
		if err != nil {
			logger.WithError(err).Errorf("Failed to get k8s inClusterConfig")
			return nil, errors.Wrap(err, "could not retrieve in-cluster config")
		}
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		logger.WithError(err).Error("Failed to get k8s clientset from the given config")
		return nil, err
	}
	return clientset, nil
}

func RetrieveStorageClassMapping(config *rest.Config, veleroNs string, logger logrus.FieldLogger) (map[string]string, error) {
	clientset, err := GetKubeClient(config, logger)
	if err != nil {
		logger.Error("Failed to get clientset from given config")
		return nil, err
	}
	opts := metav1.ListOptions{
		// velero.io/plugin-config: ""
		// velero.io/change-storage-class: RestoreItemAction
		LabelSelector: fmt.Sprintf("%s,%s=%s", constants.PluginConfigLabelKey, constants.ChangeStorageClassLabelKey, constants.PluginKindRestoreItemAction),
	}
	configMaps, err := clientset.CoreV1().ConfigMaps(veleroNs).List(context.TODO(), opts)
	if err != nil {
		logger.WithError(err).Errorf("Failed to retrieve config map lists for storage class mapping")
		return nil, err
	}
	if len(configMaps.Items) == 0 {
		logger.Info("No config map for storage class mapping exists.")
		return nil, nil
	}
	if len(configMaps.Items) > 1 {
		var items []string
		for _, item := range configMaps.Items {
			items = append(items, item.Name)
		}
		return nil, errors.Errorf("found more than one ConfigMap matching label selector %q: %v", opts.LabelSelector, items)
	}

	return configMaps.Items[0].Data, nil
}

func UpdateSnapshotWithNewStorageClass(config *rest.Config, itemSnapshot *backupdriverv1.Snapshot, storageClassMapping map[string]string, logger logrus.FieldLogger) (backupdriverv1.Snapshot, error) {
	if itemSnapshot == nil {
		return backupdriverv1.Snapshot{}, errors.New("itemSnapshot is nil, unable to update")
	}

	var err error
	snapshotMetadata := itemSnapshot.Status.Metadata

	pvc := &corev1.PersistentVolumeClaim{}
	if err = pvc.Unmarshal(snapshotMetadata); err != nil {
		logger.WithError(err).Error("Failed to unmarshal snapshotMetadata")
		return backupdriverv1.Snapshot{}, err
	}

	// update the PVC storage class
	var old string
	if pvc.Spec.StorageClassName == nil || *pvc.Spec.StorageClassName == "" {
		_, ok := storageClassMapping[constants.EmptyStorageClass]
		if !ok {
			errMsg := fmt.Sprintf("PVC %s/%s has no storage class, unable to restore", pvc.Namespace, pvc.Name)
			logger.Error(errMsg)
			return *itemSnapshot, errors.New(errMsg)
		}
		old = constants.EmptyStorageClass
	} else {
		old = *pvc.Spec.StorageClassName
	}
	logger.Infof("Updating storage class name for old storage class: %s", old)
	newName, ok := storageClassMapping[old]
	if !ok {
		logger.Infof("No mapping found for storage class %s", old)
		return *itemSnapshot, nil
	}

	// validate that new storage class exists
	clientset, err := GetKubeClient(config, logger)
	if err != nil {
		logger.Error("Failed to get core v1 client from given config")
		return *itemSnapshot, err
	}
	if _, err := clientset.StorageV1().StorageClasses().Get(context.TODO(), newName, metav1.GetOptions{}); err != nil {
		return *itemSnapshot, errors.Wrapf(err, "error getting storage class %s from API", newName)
	}

	logger.Infof("Updating item's storage class name to %s", newName)
	pvc.Spec.StorageClassName = &newName

	var updatedSnapshotMetadata []byte
	if updatedSnapshotMetadata, err = pvc.Marshal(); err != nil {
		return backupdriverv1.Snapshot{}, err
	}
	itemSnapshot.Status.Metadata = updatedSnapshotMetadata

	return *itemSnapshot, nil
}

func IsObjectBlocked(item runtime.Unstructured) (bool, string, error) {
	crdName, err := UnstructuredToCRDName(item)
	if err != nil {
		return false, "", errors.Errorf("Could not translate item kind %s to CRD name", item.GetObjectKind())
	}
	if IsResourceBlocked(crdName) {
		return true, crdName, nil
	}
	return false, crdName, nil
}

func GetResources() []string {
	desiredResources := make([]string, len(constants.ResourcesToHandle)+len(constants.ResourcesToBlock))
	for resourceToHandle, _ := range constants.ResourcesToHandle {
		desiredResources = append(desiredResources, resourceToHandle)
	}
	for resourceToBlock, _ := range constants.ResourcesToBlock {
		desiredResources = append(desiredResources, resourceToBlock)
	}
	return desiredResources
}

func IsResourceBlocked(resourceName string) bool {
	return constants.ResourcesToBlock[resourceName]
}

func IsResourceBlockedOnRestore(resourceName string) bool {
	return constants.ResourcesToBlockOnRestore[resourceName]
}

func GetPVForPVC(pvc *corev1.PersistentVolumeClaim, pvGetter corev1client.PersistentVolumesGetter) (*corev1.PersistentVolume, error) {
	if pvc.Spec.VolumeName == "" {
		return nil, errors.Errorf("PVC %s/%s has no volume backing this claim", pvc.Namespace, pvc.Name)
	}
	if pvc.Status.Phase != corev1.ClaimBound {
		// TODO: confirm if this PVC should be snapshotted if it has no PV bound
		return nil, errors.Errorf("PVC %s/%s is in phase %v and is not bound to a volume", pvc.Namespace, pvc.Name, pvc.Status.Phase)
	}
	pvName := pvc.Spec.VolumeName
	pv, err := pvGetter.PersistentVolumes().Get(context.TODO(), pvName, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get PV %s for PVC %s/%s", pvName, pvc.Namespace, pvc.Name)
	}
	return pv, nil
}

func Contains(slice []string, key string) bool {
	for _, i := range slice {
		if i == key {
			return true
		}
	}
	return false
}

func GetPodsUsingPVC(pvcNamespace, pvcName string, podsGetter corev1client.PodsGetter) ([]corev1.Pod, error) {
	podsUsingPVC := []corev1.Pod{}
	podList, err := podsGetter.Pods(pvcNamespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, p := range podList.Items {
		for _, v := range p.Spec.Volumes {
			if v.PersistentVolumeClaim != nil && v.PersistentVolumeClaim.ClaimName == pvcName {
				podsUsingPVC = append(podsUsingPVC, p)
			}
		}
	}

	return podsUsingPVC, nil
}

func GetPodVolumeNameForPVC(pod corev1.Pod, pvcName string) (string, error) {
	for _, v := range pod.Spec.Volumes {
		if v.PersistentVolumeClaim != nil && v.PersistentVolumeClaim.ClaimName == pvcName {
			return v.Name, nil
		}
	}
	return "", errors.Errorf("Pod %s/%s does not use PVC %s/%s", pod.Namespace, pod.Name, pod.Namespace, pvcName)
}

func IsPVCBackedUpByRestic(pvcNamespace, pvcName string, podClient corev1client.PodsGetter, defaultVolumesToRestic bool) (bool, error) {
	pods, err := GetPodsUsingPVC(pvcNamespace, pvcName, podClient)
	if err != nil {
		return false, errors.WithStack(err)
	}

	for _, p := range pods {
		resticVols := restic.GetPodVolumesUsingRestic(&p, defaultVolumesToRestic)
		if len(resticVols) > 0 {
			volName, err := GetPodVolumeNameForPVC(p, pvcName)
			if err != nil {
				return false, err
			}
			if Contains(resticVols, volName) {
				return true, nil
			}
		}
	}
	return false, nil
}

// SkipPVCCreation checks whether to skip creation of the PVC.
// Returns true to indicate skipping PVC creation and false otherwise.
// If an error occurrs, it returns true and the error.
// If PVC already exists, it returns true and nil.
// If PVC is not found, it returns false and nil to indicate
// PVC should be created.
func SkipPVCCreation(ctx context.Context, config *rest.Config, pvc *corev1.PersistentVolumeClaim, logger logrus.FieldLogger) (bool, error) {
	if pvc == nil {
		errMsg := "Input PVC cannot be nil"
		logger.Error(errMsg)
		return true, errors.New(errMsg)
	}
	logger.Infof("Check if PVC %s/%s creation should be skipped", pvc.Namespace, pvc.Name)
	kubeClient, err := GetKubeClient(config, logger)
	if err != nil {
		logger.Error("Failed to get clientset from given config")
		return true, err
	}

	getPvc, err := kubeClient.CoreV1().PersistentVolumeClaims(pvc.Namespace).Get(ctx, pvc.Name, metav1.GetOptions{})
	if err != nil && !apierrs.IsNotFound(err) {
		logger.Errorf("Error occurred when trying to get PVC %s/%s: %v", pvc.Namespace, pvc.Name, err)
		return true, err
	} else if getPvc != nil && getPvc.Namespace == pvc.Namespace && getPvc.Name == pvc.Name {
		// NOTE: If PVC does not exist, "Get" may return an empty PersistentVolumeClaim object, not nil; so we need to check Namespace/Name match here
		// NOTE: Need to skip it here in the plugin to avoid hanging as Velero skips restoring already existing resources
		logger.Warnf("Skipping PVC %s/%s creation since it already exists.", pvc.Namespace, pvc.Name)
		return true, nil
	} else if err != nil && apierrs.IsNotFound(err) {
		logger.Debugf("PVC %s/%s is not found. Create a new one.", pvc.Namespace, pvc.Name)
	}

	// Create a new PVC
	return false, nil
}

func IsMigratedCSIVolume(pv *corev1.PersistentVolume) bool {
	isProvisionedByVCP := pv.Spec.VsphereVolume != nil
	isAnnotatedByVsphereCSI := pv.Annotations[constants.VSphereCSIDriverMigrationAnnotation] == constants.VSphereCSIDriverName
	return isProvisionedByVCP && isAnnotatedByVsphereCSI
}
