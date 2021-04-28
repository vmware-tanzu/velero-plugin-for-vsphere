package util

import (
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"testing"
)

func Test_ItemToCRDName(t *testing.T) {
	accessor := meta.NewAccessor()

	backupUnstructuredMock := unstructured.Unstructured{}
	accessor.SetKind(&backupUnstructuredMock, "Backup")
	accessor.SetAPIVersion(&backupUnstructuredMock, "velero.io/v1")

	backupCRDName, err := UnstructuredToCRDName(&backupUnstructuredMock)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, backupCRDName, "backups.velero.io")

	repositoryUnstructuredMock := unstructured.Unstructured{}
	accessor.SetKind(&repositoryUnstructuredMock, "ResticRepository")
	accessor.SetAPIVersion(&repositoryUnstructuredMock, "velero.io/v1")

	repositoryCRDName, err := UnstructuredToCRDName(&repositoryUnstructuredMock)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, repositoryCRDName, "resticrepositories.velero.io")

	pvcUnstructuredMock := unstructured.Unstructured{}
	accessor.SetKind(&pvcUnstructuredMock, "PersistentVolumeClaim")
	accessor.SetAPIVersion(&pvcUnstructuredMock, "v1")

	pvcCRDName, err := UnstructuredToCRDName(&pvcUnstructuredMock)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, pvcCRDName, "persistentvolumeclaims")
}
