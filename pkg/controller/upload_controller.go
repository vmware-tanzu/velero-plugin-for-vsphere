package controller

import (
	"context"
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
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/utils"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	corev1informers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/utils/clock"
)

type uploadController struct {
	*genericController

	kubeClient			kubernetes.Interface
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
	kubeClient kubernetes.Interface,
	pvcInformer corev1informers.PersistentVolumeClaimInformer,
	pvInformer corev1informers.PersistentVolumeInformer,
	dataMover *dataMover.DataMover,
	snapMgr *snapshotmgr.SnapshotManager,
	nodeName string,
) Interface {
	c := &uploadController{
		genericController: newGenericController("upload", logger),
		kubeClient:		   kubeClient,
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
	log.Info("Running processQueueItem")

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
	case "", pluginv1api.UploadPhaseNew, pluginv1api.UploadPhaseInProgress:
		// Process new items
		// For UploadPhaseInProgress, the resource lease logic will process the Upload if the lease is not held by
		// another DataManager. If the DataManager holding the lease has died and/or lease has expired the current node
		// will pick such record in UploadPhaseInProgress status for processing.
	default:
		return nil
	}

	leaseLockName := "upload-lease." + name
	// Acquire lease for processing Upload.
	lock := &resourcelock.LeaseLock{
		LeaseMeta: metav1.ObjectMeta{
			Name:      leaseLockName,
			Namespace: ns,
		},
		Client: c.kubeClient.CoordinationV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: c.nodeName,
		},
	}

	// use a Go context so we can tell the leaderelection code when we
	// want to step down
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var processErr error

	// start the leader election code loop
	leaderelection.RunOrDie(ctx, leaderelection.LeaderElectionConfig{
		Lock: lock,
		ReleaseOnCancel: false,
		LeaseDuration:   utils.LeaseDuration,
		RenewDeadline:   utils.RenewDeadline,
		RetryPeriod:     utils.RetryPeriod,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				// Current node got the lease process request.
				// Don't mutate the shared cache
				reqCopy := req.DeepCopy()
				processErr = c.processBackupFunc(reqCopy)
				cancel()
			},
			OnStoppedLeading: func() {
				log.Debug("Processed Upload.")
			},
			OnNewLeader: func(identity string) {
				if identity == c.nodeName {
					// Same node is trying to acquire or renew the lease, ignore.
					return
				}
				log.Debugf("Lock is acquired by another node %s. Current node - %s need not process the Upload.", identity, c.nodeName)
				cancel()
			},
		},
	})

	return processErr
}

func (c *uploadController) pvbHandler(obj interface{}) {
	req := obj.(*pluginv1api.Upload)

	log := loggerForUpload(c.logger, req)

	switch req.Status.Phase {
	case "", pluginv1api.UploadPhaseNew, pluginv1api.UploadPhaseInProgress:
		// Process New and InProgress Uploads
	default:
		log.Debug("Upload is not New or InProgress, skipping")
		return
	}

	log.Info("Filtering out the upload requests come from different node")
	peID, err := astrolabe.NewProtectedEntityIDFromString(req.Spec.SnapshotID)
	if err != nil {
		log.WithError(err).Error("error extract volume ID from snapshot ID")
		return
	}

	uploadNodeName, err := utils.RetrievePodNodesByVolumeId(peID.GetID())
	if err != nil {
		log.WithError(err).Error("error retrieve pod nodes from volume ID")
		return
	}

	if c.nodeName != uploadNodeName {
		return
	}

	log.Debug("Enqueueing Upload")
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
		r.Status.ProcessingNode = c.nodeName
	})
	if err != nil {
		log.WithError(err).Error("Error setting Upload StartTimestamp and phase to InProgress")
		return errors.WithStack(err)
	}

	// Call data mover API to do the remote copy
	peID, err := astrolabe.NewProtectedEntityIDFromString(req.Spec.SnapshotID)
	if err != nil {
		msg := "Error getting PEID from SnapshotID string in upload CR"
		log.WithError(err).Error(msg)
		patchUploadFailure(c, req, msg)
		return errors.WithStack(err)
	}

	_, err = c.dataMover.CopyToRepo(peID)
	if err != nil {
		msg := "Error uploading snapshot to durable object storage"
		log.WithError(err).Error(msg)
		patchUploadFailure(c, req, msg)
		return errors.WithStack(err)
	}

	// Call snapshot manager API to cleanup the local snapshot
	err = c.snapMgr.DeleteProtectedEntitySnapshot(peID, false)
	if err != nil {
		msg := "Error cleaning up local snapshot after uploading snapshot"
		log.WithError(err).Error(msg)
		// TODO: Change the upload CRD definition to add one more phase, such as, UploadPhaseFailedLocalCleanup
		patchUploadFailure(c, req, msg)
		return errors.WithStack(err)
	}

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

func patchUploadFailure(c *uploadController, req *pluginv1api.Upload, msg string) (*pluginv1api.Upload, error) {
	// update status to Failed
	var err error
	req, err = c.patchUpload(req, func(r *pluginv1api.Upload) {
		r.Status.Phase = pluginv1api.UploadPhaseFailed
		r.Status.CompletionTimestamp = &metav1.Time{Time: c.clock.Now()}
		r.Status.Message = msg
	})
	if err != nil {
		c.logger.WithError(err).Error("Failed to patch Upload failure")
	}

	return req, err
}
