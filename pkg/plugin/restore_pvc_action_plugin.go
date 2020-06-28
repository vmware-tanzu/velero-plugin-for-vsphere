package plugin

import (
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/vmware-tanzu/velero/pkg/plugin/velero"
	corev1api "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

// PVCBackupItemAction is a backup item action plugin for Velero.
type NewPVCRestoreItemAction struct {
	Log logrus.FieldLogger
}

// AppliesTo returns information indicating that the PVCBackupItemAction should be invoked to backup PVCs.
func (p *NewPVCRestoreItemAction) AppliesTo() (velero.ResourceSelector, error) {
	p.Log.Info("PVCBackupItemAction AppliesTo for vSphere")

	return velero.ResourceSelector{
		IncludedResources: []string{"persistentvolumeclaims"},
	}, nil
}

func (p *NewPVCRestoreItemAction) Execute(input *velero.RestoreItemActionExecuteInput) (*velero.RestoreItemActionExecuteOutput, error) {

	var pvc corev1api.PersistentVolumeClaim
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(input.Item.UnstructuredContent(), &pvc); err != nil {
		return nil, errors.WithStack(err)
	}

	p.Log.Infof("Starting PVCRestoreItemAction for PVC %s/%s", pvc.Namespace, pvc.Name)

	// TODO: add logic for PVC restoration

	pvcMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&pvc)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	p.Log.Infof("Returning from PVCRestoreItemAction for PVC %s/%s", pvc.Namespace, pvc.Name)

	return &velero.RestoreItemActionExecuteOutput{
		UpdatedItem: &unstructured.Unstructured{Object: pvcMap},
	}, nil
}