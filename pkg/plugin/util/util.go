package util

import (
	"encoding/base64"
	"encoding/json"
	"github.com/pkg/errors"
	backupdriverv1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/backupdriver/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
