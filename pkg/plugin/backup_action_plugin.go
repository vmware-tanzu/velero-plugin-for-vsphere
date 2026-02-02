package plugin

import (
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"github.com/vmware-tanzu/velero/pkg/plugin/velero"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/restmapper"

	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/utils"
)

// BackupItemAction is a backup item action plugin for Velero.
type NewBackupItemAction struct {
	Log    logrus.FieldLogger
	Mapper *restmapper.DeferredDiscoveryRESTMapper
}

// AppliesTo returns information indicating that the BackupItemAction should be invoked to backup resources.
func (p *NewBackupItemAction) AppliesTo() (velero.ResourceSelector, error) {
	p.Log.Info("VSphere BackupItemAction AppliesTo")

	blockListConfigMap, err := utils.RetrieveBlockListConfigMap(p.Log)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			p.Log.Info("Failed to retrieve resources to block list.")
			return velero.ResourceSelector{}, errors.WithStack(err)
		}
		p.Log.Info("There is no blocklist configmap found. Assuming no resources to block.")
		blockListConfigMap = make(map[string]string)
	}

	return velero.ResourceSelector{
		IncludedResources: utils.GetResources(blockListConfigMap),
	}, nil
}

func (p *NewBackupItemAction) Execute(item runtime.Unstructured, backup *velerov1.Backup) (runtime.Unstructured, []velero.ResourceIdentifier, error) {
	blocked, crdName, err := utils.IsObjectBlocked(item, p.Mapper, p.Log)
	if err != nil {
		return nil, nil, errors.Wrap(err, "Failed during IsObjectBlocked check")
	}
	p.Log.Infof("Backing up resource %v: blocked = %v", crdName, blocked)

	if blocked {
		p.Log.Infof("Resource %v is blocked and will not be backed up", crdName)

		accessor := meta.NewAccessor()
		annotations, err := accessor.Annotations(item)
		if err != nil {
			return nil, nil, errors.Wrap(err, "Failed to get annotations from item")
		}

		if annotations == nil {
			annotations = make(map[string]string)
		}
		// Add annotation to indicate that this item should be skipped from backup.
		// Velero will check for this annotation and skip backing up this item if the annotation is present.
		annotations["velero.io/skip-from-backup"] = ""

		accessor.SetAnnotations(item, annotations)

		return item, nil, nil
	}

	var additionalItems []velero.ResourceIdentifier

	return item, additionalItems, nil
}
