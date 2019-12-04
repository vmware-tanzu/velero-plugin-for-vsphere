package controller

import (
	"encoding/json"
	"fmt"
	jsonpatch "github.com/evanphx/json-patch"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/vmware-tanzu/astrolabe/pkg/astrolabe"
	pluginv1api "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/veleroplugin/v1"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/dataMover"
	pluginv1client "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned/typed/veleroplugin/v1"
	informers "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/informers/externalversions/veleroplugin/v1"
	listers "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/listers/veleroplugin/v1"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/snapshotmgr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	corev1informers "k8s.io/client-go/informers/core/v1"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/utils/clock"
)

type uploadController struct {
	*genericController

	uploadClient pluginv1client.UploadsGetter
	uploadLister listers.UploadLister
	pvcLister    corev1listers.PersistentVolumeClaimLister
	pvLister     corev1listers.PersistentVolumeLister
	nodeName     string
	dataMover    *dataMover.DataMover
	snapMgr      *snapshotmgr.SnapshotManager

	processBackupFunc func(*pluginv1api.Upload) error
	clock             clock.Clock
}

func NewUploadController(
	logger logrus.FieldLogger,
	uploadInformer informers.UploadInformer,
	uploadClient pluginv1client.UploadsGetter,
	pvcInformer corev1informers.PersistentVolumeClaimInformer,
	pvInformer corev1informers.PersistentVolumeInformer,
	dataMover *dataMover.DataMover,
	snapMgr *snapshotmgr.SnapshotManager,
	nodeName string,
) Interface {
	c := &uploadController{
		genericController: newGenericController("upload", logger),
		uploadClient:      uploadClient,
		uploadLister:      uploadInformer.Lister(),
		pvcLister:         pvcInformer.Lister(),
		pvLister:          pvInformer.Lister(),
		nodeName:          nodeName,
		dataMover:         dataMover,
		snapMgr:           snapMgr,
		clock:             &clock.RealClock{},
	}

	c.syncHandler = c.processQueueItem
	c.cacheSyncWaiters = append(
		c.cacheSyncWaiters,
		uploadInformer.Informer().HasSynced,
		pvcInformer.Informer().HasSynced,
	)
	c.processBackupFunc = c.processBackup

	uploadInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    c.pvbHandler,
			UpdateFunc: func(_, obj interface{}) { c.pvbHandler(obj) },
		},
	)

	return c

}

func (c *uploadController) processQueueItem(key string) error {
	log := c.logger.WithField("key", key)
	log.Debug("Running processQueueItem")

	ns, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		log.WithError(err).Error("error splitting queue key")
		return nil
	}

	req, err := c.uploadLister.Uploads(ns).Get(name)
	if apierrors.IsNotFound(err) {
		log.Debug("Unable to find Upload")
		return nil
	}
	if err != nil {
		return errors.Wrap(err, "error getting Upload")
	}

	// only process new items
	switch req.Status.Phase {
	case "", pluginv1api.UploadPhaseNew:
	default:
		return nil
	}

	// Don't mutate the shared cache
	reqCopy := req.DeepCopy()
	return c.processBackupFunc(reqCopy)
}

func (c *uploadController) pvbHandler(obj interface{}) {
	req := obj.(*pluginv1api.Upload)

	// only enqueue items for this node
	// Todo: Enabled in the production code
	// disable the check for local debug purpose
	//if req.Spec.Node != c.nodeName {
	//	return
	//}

	log := loggerForUpload(c.logger, req)

	if req.Status.Phase != "" && req.Status.Phase != pluginv1api.UploadPhaseNew {
		log.Debug("Upload is not new, not enqueuing")
		return
	}

	log.Debug("Enqueueing")
	c.enqueue(obj)
}

func (c *uploadController) patchUpload(req *pluginv1api.Upload, mutate func(*pluginv1api.Upload)) (*pluginv1api.Upload, error) {
	// Record original json
	oldData, err := json.Marshal(req)
	if err != nil {
		return nil, errors.Wrap(err, "error marshalling original PodVolumeBackup")
	}

	// Mutate
	mutate(req)

	// Record new json
	newData, err := json.Marshal(req)
	if err != nil {
		return nil, errors.Wrap(err, "error marshalling updated PodVolumeBackup")
	}

	patchBytes, err := jsonpatch.CreateMergePatch(oldData, newData)
	if err != nil {
		return nil, errors.Wrap(err, "error creating json merge patch for PodVolumeBackup")
	}

	req, err = c.uploadClient.Uploads(req.Namespace).Patch(req.Name, types.MergePatchType, patchBytes)
	if err != nil {
		return nil, errors.Wrap(err, "error patching PodVolumeBackup")
	}

	return req, nil
}

func (c *uploadController) processBackup(req *pluginv1api.Upload) error {
	log := loggerForUpload(c.logger, req)

	log.Info("Upload starting")

	var err error

	// update status to InProgress
	req, err = c.patchUpload(req, func(r *pluginv1api.Upload) {
		r.Status.Phase = pluginv1api.UploadPhaseInProgress
		r.Status.StartTimestamp = &metav1.Time{Time: c.clock.Now()}
	})
	if err != nil {
		log.WithError(err).Error("Error setting Upload StartTimestamp and phase to InProgress")
		return errors.WithStack(err)
	}

	// Call data mover API to do the remote copy
	peID, err := astrolabe.NewProtectedEntityIDFromString(req.Spec.SnapshotID)
	if err != nil {
		log.WithError(err).Error("Error getting PEID from SnapshotID string in upload CR")
		return errors.WithStack(err)
	}
	c.dataMover.CopyToRepo(peID)
	// Call snapshot manager API to cleanup the local snapshot
	c.snapMgr.DeleteProtectedEntitySnapshot(peID, false)

	// update status to Completed with path & snapshot id
	req, err = c.patchUpload(req, func(r *pluginv1api.Upload) {
		r.Status.Phase = pluginv1api.UploadPhaseCompleted
		r.Status.CompletionTimestamp = &metav1.Time{Time: c.clock.Now()}
	})
	if err != nil {
		log.WithError(err).Error("Error setting Upload phase to Completed")
		return err
	}

	log.Info("Upload completed")

	return nil
}

func loggerForUpload(baseLogger logrus.FieldLogger, req *pluginv1api.Upload) logrus.FieldLogger {
	log := baseLogger.WithFields(logrus.Fields{
		"namespace": req.Namespace,
		"name":      req.Name,
	})

	if len(req.OwnerReferences) == 1 {
		log = log.WithField("upload", fmt.Sprintf("%s/%s", req.Namespace, req.OwnerReferences[0].Name))
	}

	return log
}
