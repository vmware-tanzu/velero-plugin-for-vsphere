package util

import (
	"encoding/base64"
	"encoding/json"
	"github.com/pkg/errors"
	backupdriverv1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/backupdriver/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
