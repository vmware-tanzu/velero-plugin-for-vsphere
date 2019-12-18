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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/utils/clock"
	"time"
)

type downloadController struct {
	*genericController

	kubeClient			kubernetes.Interface
	downloadClient		pluginv1client.DownloadsGetter
	downloadLister		listers.DownloadLister
	nodeName			string
	dataMover			*dataMover.DataMover
	clock				clock.Clock
}

func NewDownloadController(
	logger 				logrus.FieldLogger,
	downloadInformer	informers.DownloadInformer,
	downloadClient		pluginv1client.DownloadsGetter,
	kubeClient			kubernetes.Interface,
	dataMover				*dataMover.DataMover,
	nodeName			string,
) Interface {
	c := &downloadController{
		genericController:	newGenericController("download", logger),
		kubeClient:			kubeClient,
		downloadClient:		downloadClient,
		downloadLister:		downloadInformer.Lister(),
		nodeName:			nodeName,
		dataMover:			dataMover,
		clock:				&clock.RealClock{},
	}

	c.syncHandler = c.processDownload
	c.cacheSyncWaiters = append(
		c.cacheSyncWaiters,
		downloadInformer.Informer().HasSynced,
	)

	downloadInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    c.enqueueDownload,
			UpdateFunc: func(_, obj interface{}) { c.enqueueDownload(obj) },
		},
	)

	return c
}

func (c *downloadController) enqueueDownload(obj interface{}) {
	req := obj.(*pluginv1api.Download)

	log := loggerForDownload(c.logger, req)

	switch req.Status.Phase {
	case "", pluginv1api.DownloadPhaseNew:
		// only process new backups
	default:
		log.Debug("Download is not new, skipping")
		return
	}

	log.Debug("Enqueueing Download")
	c.enqueue(obj)
}

func (c *downloadController) processDownload(key string) error {
	log := c.logger.WithField("key", key)
	log.Info("Running processDownload")

	ns, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		log.WithError(err).Error("error splitting queue key")
		return nil
	}

	req, err := c.downloadLister.Downloads(ns).Get(name)
	if apierrors.IsNotFound(err) {
		log.Debug("Unable to find Download")
		return nil
	}
	if err != nil {
		return errors.Wrap(err, "error getting Download")
	}

	switch req.Status.Phase {
	case "", pluginv1api.DownloadPhaseNew:
		// only process new items
		// TODO process DownloadPhaseInProgress status as well to handle node crash scenarios.
		// For DownloadPhaseInProgress, the resource lease will take care of not processing the Download if the lease is owned by another running node.
	default:
		return nil
	}

	leaseLockName := "download-lease." + name
	// Acquire lease for processing Download.
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
		LeaseDuration:   60 * time.Second,
		RenewDeadline:   15 * time.Second,
		RetryPeriod:     5 * time.Second,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				// Current node got the lease process request.
				processErr = c.runDownload(req)
				cancel()
			},
			OnStoppedLeading: func() {
				log.Debug("Processed Download.")
			},
			OnNewLeader: func(identity string) {
				if identity == c.nodeName {
					// Same node is trying to acquire or renew the lease, ignore.
					return
				}
				log.Debugf("Lock is acquired by another node %s. Current node - %s need not process the Download.", identity, c.nodeName)
				cancel()
			},
		},
	})

	return processErr
}

func (c *downloadController) runDownload(req *pluginv1api.Download) error {
	log := loggerForDownload(c.logger, req)

	log.Info("Running Download")

	var err error

	// update status to InProgress
	req, err = c.patchDownload(req, func(r *pluginv1api.Download) {
		r.Status.Phase = pluginv1api.DownloadPhaseInProgress
		r.Status.StartTimestamp = &metav1.Time{Time: c.clock.Now()}
		r.Status.ProcessingNode = c.nodeName
	})
	if err != nil {
		log.WithError(err).Error("Error setting Download StartTimestamp and phase to InProgress")
		return errors.WithStack(err)
	}

	var peId, returnPeId astrolabe.ProtectedEntityID
	peId, err = astrolabe.NewProtectedEntityIDFromString(req.Spec.SnapshotID)
	if err != nil {
		msg := "Fail to construct new PE ID from string"
		log.WithError(err).Error(msg)
		patchDownloadFailure(c, req, msg)
		return err
	}
	returnPeId, err = c.dataMover.CopyFromRepo(peId)
	if err != nil {
		msg := "Error downloading snapshot from durable object storage"
		log.WithError(err).Error(msg)
		patchDownloadFailure(c, req, msg)
		return errors.WithStack(err)
	}

	log.Debugf("A new volume %s was just created from the call to CopyFromRepo", returnPeId.String())

	// update status to Completed with path & snapshot id
	req, err = c.patchDownload(req, func(r *pluginv1api.Download) {
		r.Status.Phase = pluginv1api.DownloadPhaseCompleted
		r.Status.CompletionTimestamp = &metav1.Time{Time: c.clock.Now()}
		r.Status.VolumeID = returnPeId.String()
	})
	if err != nil {
		log.WithError(err).Error("Error setting Download phase to Completed")
		return err
	}

	log.Info("Download completed")

	return nil
}

func (c *downloadController) patchDownload(req *pluginv1api.Download, mutate func(*pluginv1api.Download)) (*pluginv1api.Download, error) {
	// Record original json
	oldData, err := json.Marshal(req)
	if err != nil {
		return nil, errors.Wrap(err, "error marshalling original Download")
	}

	// Mutate
	mutate(req)

	// Record new json
	newData, err := json.Marshal(req)
	if err != nil {
		return nil, errors.Wrap(err, "error marshalling updated Download")
	}

	patchBytes, err := jsonpatch.CreateMergePatch(oldData, newData)
	if err != nil {
		return nil, errors.Wrap(err, "error creating json merge patch for Download")
	}

	req, err = c.downloadClient.Downloads(req.Namespace).Patch(req.Name, types.MergePatchType, patchBytes)
	if err != nil {
		return nil, errors.Wrap(err, "error patching Download")
	}

	return req, nil
}

func loggerForDownload(baseLogger logrus.FieldLogger, req *pluginv1api.Download) logrus.FieldLogger {
	log := baseLogger.WithFields(logrus.Fields{
		"namespace": req.Namespace,
		"name":      req.Name,
	})

	if len(req.OwnerReferences) == 1 {
		log = log.WithField("download", fmt.Sprintf("%s/%s", req.Namespace, req.OwnerReferences[0].Name))
	}

	return log
}

func patchDownloadFailure(c *downloadController, req *pluginv1api.Download, msg string) (*pluginv1api.Download, error) {
	// update status to Failed
	var err error
	req, err = c.patchDownload(req, func(r *pluginv1api.Download) {
		r.Status.Phase = pluginv1api.DownloadPhaseFailed
		r.Status.CompletionTimestamp = &metav1.Time{Time: c.clock.Now()}
		r.Status.Message = msg
	})
	if err != nil {
		c.logger.WithError(err).Error("Failed to patch Download failure")
	}

	return req, err
}
