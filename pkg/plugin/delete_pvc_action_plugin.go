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

func (p *NewPVCDeleteItemAction) Execute(item runtime.Unstructured, backup *velerov1api.Backup) (runtime.Unstructured, []velero.ResourceIdentifier, error) {
	// Skeleton
	return nil, nil, nil
}
