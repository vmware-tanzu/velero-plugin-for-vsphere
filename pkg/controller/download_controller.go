/*
Copyright 2020 the Velero contributors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/utils"
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
	processDownloadFunc func(*pluginv1api.Download) error
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

	c.syncHandler = c.processDownloadItem
	c.retryHandler = c.reEnqueueHandler
	c.cacheSyncWaiters = append(
		c.cacheSyncWaiters,
		downloadInformer.Informer().HasSynced,
	)
	c.processDownloadFunc = c.processDownload

	downloadInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    c.enqueueDownloadItem,
			UpdateFunc: func(_, obj interface{}) { c.enqueueDownloadItem(obj) },
		},
	)

	return c
}

func (c *downloadController) enqueueDownloadItem(obj interface{}) {
	req := obj.(*pluginv1api.Download)

	log := loggerForDownload(c.logger, req)

	switch req.Status.Phase {
	case "", pluginv1api.DownloadPhaseNew, pluginv1api.DownloadPhaseInProgress, pluginv1api.DownLoadPhaseRetry:
		// Process New InProgress and Retry Downloads
	default:
		log.Debug("Download CR is not New or InProgress or Retry, skipping")
		return
	}

	log.Infof("Filtering out the retry download request which comes in before next retry time")
	now := c.clock.Now()
	if now.Unix() < req.Status.NextRetryTimestamp.Unix() {
		log.WithFields(logrus.Fields{
			"nextRetryTime": req.Status.NextRetryTimestamp,
			"currentTime": now,
		}).Infof("Ingnore retry donwload request which comes in before next retry time, download CR: %s", req.Name)
		return
	}

	log.Infof("Enqueueing download")
	c.enqueue(obj)
}

func (c *downloadController) processDownloadItem(key string) error {
	log := c.logger.WithField("key", key)
	log.Info("Running processDownloadItem")

	ns, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		log.WithError(err).Error("Failed to split the key of queue item")
		return nil
	}

	req, err := c.downloadLister.Downloads(ns).Get(name)
	if apierrors.IsNotFound(err) {
		log.Error("Download is not found")
		return nil
	}
	if err != nil {
		return errors.Wrap(err, "Failed to get Download")
	}

	switch req.Status.Phase {
	case "", pluginv1api.DownloadPhaseNew, pluginv1api.DownloadPhaseInProgress, pluginv1api.DownLoadPhaseRetry:
		// Process new items
		// For DownloadPhaseInProgress, the resource lease logic will process the Download if the lease is not held by
		// another DataManager. If the DataManager holding the lease has died and/or lease has expired the current node
		// will pick such record in DownloadPhaseInProgress status for processing.
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
		LeaseDuration:   utils.LeaseDuration,
		RenewDeadline:   utils.RenewDeadline,
		RetryPeriod:     utils.RetryPeriod,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				// Current node got the lease process request.
				processErr = c.processDownload(req)
				cancel()
			},
			OnStoppedLeading: func() {
				log.Infof("Processed Download.")
			},
			OnNewLeader: func(identity string) {
				if identity == c.nodeName {
					// Same node is trying to acquire or renew the lease, ignore.
					return
				}
				log.Infof("Lock is acquired by another node %s. Current node - %s need not process the Download.", identity, c.nodeName)
				cancel()
			},
		},
	})

	return processErr
}

func (c *downloadController) processDownload(req *pluginv1api.Download) error {
	log := loggerForDownload(c.logger, req)
	log.Info("Download starting")
	var err error

	// retrieve download request for its updated status from k8s api server and filter out completed one
	req, err = c.downloadClient.Downloads(req.Namespace).Get(req.Name, metav1.GetOptions{})
	if err != nil {
		log.WithError(err).Error("Failed to retrieve download CR from kubernetes API server")
		return errors.WithStack(err)
	}
	// update req with the one retrieved from k8s api server
	log.WithFields(logrus.Fields{
		"phase": req.Status.Phase,
		"generation": req.Generation,
	}).Info("Download request updated by retrieving from kubernetes API server")

	if req.Status.Phase == pluginv1api.DownloadPhaseCompleted {
		log.Info("The status of download CR in kubernetes API server is completed. Skipping it")
		return nil
	}

	// update status to InProgress
	if req.Status.Phase != pluginv1api.DownloadPhaseInProgress {
		// update status to InProgress
		req, err = c.patchDownloadByStatus(req, pluginv1api.DownloadPhaseInProgress, "")
		if err != nil {
			return errors.WithStack(err)
		}
	}

	peID, err := astrolabe.NewProtectedEntityIDFromString(req.Spec.SnapshotID)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to get PEID from SnapshotID, %v. %v", req.Spec.SnapshotID, errors.WithStack(err))
		_, err = c.patchDownloadByStatus(req, pluginv1api.DownLoadPhaseRetry, errMsg)
		if err != nil {
			errMsg = fmt.Sprintf("%v. %v", errMsg, errors.WithStack(err))
		}
		log.Error(errMsg)
		return errors.New(errMsg)
	}

	returnPeId, err := c.dataMover.CopyFromRepo(peID)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to download snapshot, %v, from durable object storage. %v", peID.String(), errors.WithStack(err))
		_, err = c.patchDownloadByStatus(req, pluginv1api.DownLoadPhaseRetry, errMsg)
		if err != nil {
			errMsg = fmt.Sprintf("%v. %v", errMsg, errors.WithStack(err))
		}
		log.Error(errMsg)
		return errors.New(errMsg)
	}

	log.Debugf("A new volume %s was just created from the call to CopyFromRepo", returnPeId.String())

	// update status to Completed with path & snapshot id
	req, err = c.patchDownloadByStatus(req, pluginv1api.DownloadPhaseCompleted, returnPeId.String())
	if err != nil {
		return errors.WithStack(err)
	}

	log.WithFields(logrus.Fields{
		"phase": req.Status.Phase,
		"generation": req.Generation,
	}).Infof("Download completed")

	return nil
}

func (c *downloadController) patchDownload(req *pluginv1api.Download, mutate func(*pluginv1api.Download)) (*pluginv1api.Download, error) {
	log := loggerForDownload(c.logger, req)
	// Record original json
	oldData, err := json.Marshal(req)
	if err != nil {
		log.WithError(err).Error("Failed to marshall original Download")
		return nil, err
	}

	// Mutate
	mutate(req)

	// Record new json
	newData, err := json.Marshal(req)
	if err != nil {
		log.WithError(err).Error("Failed to marshall updated Download")
		return nil, err
	}

	patchBytes, err := jsonpatch.CreateMergePatch(oldData, newData)
	if err != nil {
		log.WithError(err).Error("Failed to create json merge patch for Download")
		return nil, err
	}

	req, err = c.downloadClient.Downloads(req.Namespace).Patch(req.Name, types.MergePatchType, patchBytes)
	if err != nil {
		log.WithError(err).Error("Failed to patch Download")
		return nil, err
	}

	return req, nil
}

func (c *downloadController) patchDownloadByStatus(req *pluginv1api.Download, newPhase pluginv1api.DownloadPhase, msg string) (*pluginv1api.Download, error) {
	// update status to Failed
	log := loggerForDownload(c.logger, req)
	oldPhase := req.Status.Phase

	var err error

	switch newPhase {
	case pluginv1api.DownloadPhaseCompleted:
		// in the status of DownloadPhaseCompleted, use the msg param to pass the new volume id
		req, err = c.patchDownload(req, func (r *pluginv1api.Download){
			r.Status.Phase = newPhase
			r.Status.CompletionTimestamp = &metav1.Time{Time: c.clock.Now()}
			r.Status.Message = "Download completed"
			r.Status.VolumeID = msg
		})
	case pluginv1api.DownLoadPhaseRetry:
		if req.Status.RetryCount > utils.DOWNLOAD_MAX_RETRY {
			log.Debugf("Number of retry for download %s exceeds maximum limit, mark this download as DownloadPhaseFailed", req.Name)
			req, err = c.patchDownload(req, func (r *pluginv1api.Download){
				r.Status.Phase = pluginv1api.DownloadPhaseFailed
				r.Status.CompletionTimestamp = &metav1.Time{Time: c.clock.Now()}
				r.Status.Message = msg
			})
		} else {
			req, err = c.patchDownload(req, func (r *pluginv1api.Download){
				r.Status.Phase = newPhase
				r.Status.NextRetryTimestamp = &metav1.Time{Time: c.clock.Now().Add(utils.DOWNLOAD_BACKOFF * time.Minute)}
				r.Status.RetryCount = r.Status.RetryCount + 1
				r.Status.Message = msg
			})
		}
	case pluginv1api.DownloadPhaseInProgress:
		req, err = c.patchDownload(req, func (r *pluginv1api.Download){
			if r.Status.Phase == pluginv1api.DownloadPhaseNew {
				r.Status.StartTimestamp = &metav1.Time{Time: c.clock.Now()}
				r.Status.RetryCount = utils.MIN_RETRY
			}
			r.Status.Phase = newPhase
			r.Status.ProcessingNode = c.nodeName
		})
	default:
		err = errors.New("Unexpected download phase")
	}

	if err != nil {
		log.WithError(err).Errorf("Failed to patch Download from %v to %v", oldPhase, newPhase)
	} else {
		log.Infof("Download status updated from %v to %v", oldPhase, newPhase)
	}

	return req, err
}

func loggerForDownload(baseLogger logrus.FieldLogger, req *pluginv1api.Download) logrus.FieldLogger {
	log := baseLogger.WithFields(logrus.Fields{
		"namespace": req.Namespace,
		"name":      req.Name,
		"phase":      req.Status.Phase,
		"generation": req.Generation,
	})

	if len(req.OwnerReferences) == 1 {
		log = log.WithField("download", fmt.Sprintf("%s/%s", req.Namespace, req.OwnerReferences[0].Name))
	}

	return log
}

func (c *downloadController) reEnqueueHandler(key string) error {
	log := c.logger.WithField("key", key)
	log.Info("Running reEnqueueHandler for re-adding failed download CR")
	log.Infof("Re-adding failed download %s to the queue", key)
	c.queue.AddAfter(key, utils.DOWNLOAD_BACKOFF * time.Minute)
	return nil
}
