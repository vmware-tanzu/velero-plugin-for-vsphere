package util

import (
	"encoding/base64"
	"encoding/json"
	"github.com/pkg/errors"
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