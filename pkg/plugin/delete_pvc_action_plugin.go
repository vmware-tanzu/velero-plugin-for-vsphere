package plugin

import (
	"github.com/sirupsen/logrus"
	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"github.com/vmware-tanzu/velero/pkg/plugin/velero"
	"k8s.io/apimachinery/pkg/runtime"
)

// PVCDeleteItemAction is a delete item action plugin for Velero.
type NewPVCDeleteItemAction struct {
	Log logrus.FieldLogger
}

func (p *NewPVCDeleteItemAction) AppliesTo() (velero.ResourceSelector, error) {
	p.Log.Info("VSphere PVCDeleteItemAction AppliesTo")

	return velero.ResourceSelector{
		IncludedResources: []string{"persistentvolumeclaims"},
	}, nil
}

func (p *NewPVCDeleteItemAction) Execute(item runtime.Unstructured, backup *velerov1api.Backup) error {
	// Skeleton, DeleteItemActionInput as input parameter is unavailable
	return nil
}
