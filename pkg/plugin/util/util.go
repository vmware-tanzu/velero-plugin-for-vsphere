package util

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	backupdriverv1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/backupdriver/v1alpha1"
	"fmt"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/constants"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
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

func GetKubeClient(config *rest.Config, logger logrus.FieldLogger) (*kubernetes.Clientset, error) {
	var err error
	if config == nil {
		config, err = rest.InClusterConfig()
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
		LabelSelector: fmt.Sprintf("%s,%s=%s", constants.PluginConfigLabelKey,constants.ChangeStorageClassLabelKey,constants.PluginKindRestoreItemAction),
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