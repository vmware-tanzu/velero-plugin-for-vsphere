package controller

import (
	"encoding/json"
	"fmt"
	jsonpatch "github.com/evanphx/json-patch"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	pluginv1api "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/veleroplugin/v1"
	pluginv1client "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned/typed/veleroplugin/v1"
	informers "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/informers/externalversions/veleroplugin/v1"
	listers "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/listers/veleroplugin/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"k8s.io/utils/clock"
	"time"
)

type downloadController struct {
	*genericController

	downloadClient		pluginv1client.DownloadsGetter
	downloadLister		listers.DownloadLister
	nodeName			string
	clock				clock.Clock
}

func NewDownloadController(
	logger 				logrus.FieldLogger,
	downloadInformer	informers.DownloadInformer,
	downloadClient		pluginv1client.DownloadsGetter,
	nodeName			string,
) Interface {
	c := &downloadController{
		genericController:	newGenericController("download", logger),
		downloadClient:		downloadClient,
		downloadLister:		downloadInformer.Lister(),
		nodeName:			nodeName,
		clock:				&clock.RealClock{},
	}

	c.syncHandler = c.processDownload
	c.cacheSyncWaiters = append(
		c.cacheSyncWaiters,
		downloadInformer.Informer().HasSynced,
	)

	downloadInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    c.enqueueRestore,
			UpdateFunc: func(_, obj interface{}) { c.enqueueRestore(obj) },
		},
	)

	return c
}

func (c *downloadController) enqueueRestore(obj interface{}) {
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
	log.Debug("Running processDownload")

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
	default:
		return nil
	}

	// Don't mutate the shared cache
	reqCopy := req.DeepCopy()
	return c.runRestore(reqCopy)
}

func (c *downloadController) runRestore(req *pluginv1api.Download) error {
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

	// TODO call astrolabe API to do the remote download
	time.Sleep(5 * time.Second)

	// update status to Completed with path & snapshot id
	req, err = c.patchDownload(req, func(r *pluginv1api.Download) {
		r.Status.Phase = pluginv1api.DownloadPhaseCompleted
		r.Status.CompletionTimestamp = &metav1.Time{Time: c.clock.Now()}
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
