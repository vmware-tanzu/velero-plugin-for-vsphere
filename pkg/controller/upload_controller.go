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
	"fmt"
	"k8s.io/apimachinery/pkg/util/wait"
	backupdriverapi "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/backupdriver/v1"
	"math"
	"time"

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
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/utils/clock"
)

type uploadController struct {
	*genericController

	kubeClient        kubernetes.Interface
	uploadClient      pluginv1client.UploadsGetter
	uploadLister      listers.UploadLister
	nodeName          string
	dataMover         *dataMover.DataMover
	snapMgr           *snapshotmgr.SnapshotManager
	clock             clock.Clock
	processUploadFunc func(*pluginv1api.Upload) error
	externalDataMgr   bool
}

func NewUploadController(
	logger logrus.FieldLogger,
	uploadInformer informers.UploadInformer,
	uploadClient pluginv1client.UploadsGetter,
	kubeClient kubernetes.Interface,
	dataMover *dataMover.DataMover,
	snapMgr *snapshotmgr.SnapshotManager,
	nodeName string,
	externalDataMgr bool,
) Interface {
	c := &uploadController{
		genericController: newGenericController("upload", logger),
		kubeClient:        kubeClient,
		uploadClient:      uploadClient,
		uploadLister:      uploadInformer.Lister(),
		nodeName:          nodeName,
		dataMover:         dataMover,
		snapMgr:           snapMgr,
		clock:             &clock.RealClock{},
		externalDataMgr:   externalDataMgr,
	}

	c.syncHandler = c.processUploadItem
	c.retryHandler = c.exponentialBackoffHandler
	c.cacheSyncWaiters = append(
		c.cacheSyncWaiters,
		uploadInformer.Informer().HasSynced,
	)
	c.processUploadFunc = c.processUpload

	uploadInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    c.enqueueUploadItem,
			UpdateFunc: func(_, obj interface{}) { c.enqueueUploadItem(obj) },
		},
	)

	return c

}

func (c *uploadController) enqueueUploadItem(obj interface{}) {
	req := obj.(*pluginv1api.Upload)

	log := loggerForUpload(c.logger, req)

	switch req.Status.Phase {
	case "", pluginv1api.UploadPhaseNew, pluginv1api.UploadPhaseInProgress, pluginv1api.UploadPhaseUploadError, pluginv1api.UploadPhaseCanceling:
		// Process New and InProgress and UploadError Uploads
	case pluginv1api.UploadPhaseCanceled:
		// The upload was canceled, nothing to do.
		log.Debug("The upload request was canceled")
		return
	default:
		log.Debug("Upload CR is not New or InProgress or UploadError, skipping")
		return
	}

	// Check if the upload was canceled and trigger cancellation.
	if req.Spec.UploadCancel {
		err := c.triggerUploadCancellation(req)
		if err != nil {
			log.Error("Received error during upload cancellation.")
		}
		return
	}

	log.Infof("Filtering out the retry upload request which comes in before next retry time")
	now := c.clock.Now()
	if now.Unix() < req.Status.NextRetryTimestamp.Unix() {
		log.WithFields(logrus.Fields{
			"nextRetryTime": req.Status.NextRetryTimestamp,
			"currentTime":   now,
		}).Infof("Ignore retry upload request which comes in before next retry time, upload CR: %s", req.Name)
		return
	}

	// Check if current node is the expected upload node only if data manager is not remote.
	if !c.externalDataMgr {
		log.Infof("Filtering out the upload request from nodes other than %v", c.nodeName)
		peID, err := astrolabe.NewProtectedEntityIDFromString(req.Spec.SnapshotID)
		if err != nil {
			log.WithError(err).Errorf("Failed to extract volume ID from snapshot ID, %v", req.Spec.SnapshotID)
			return
		}

		uploadNodeName, err := utils.RetrievePodNodesByVolumeId(peID.GetID())
		if err != nil {
			_, ok := err.(utils.NotFoundError)
			if ok {
				log.Infof("Trying to back independent PV from volume ID, %v", peID.String())
				uploadNodeName = c.nodeName
			} else {
				log.WithError(err).Errorf("Failed to retrieve pod nodes from volume ID, %v", peID.String())
				return
			}
		}

		log.Infof("Current node: %v. Expected node for uploading the upload CR: %v", c.nodeName, uploadNodeName)
		if c.nodeName != uploadNodeName {
			return
		}
	}

	log.Infof("Enqueueing upload")
	c.enqueue(obj)
}

func (c *uploadController) processUploadItem(key string) error {
	log := c.logger.WithField("key", key)
	log.Info("Running processUploadItem")

	ns, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		log.WithError(err).Error("Failed to split the key of queue item")
		return nil
	}

	req, err := c.uploadLister.Uploads(ns).Get(name)
	if apierrors.IsNotFound(err) {
		log.Error("Upload is not found")
		return nil
	}
	if err != nil {
		return errors.Wrap(err, "Failed to get Upload")
	}

	// only process new items
	switch req.Status.Phase {
	case "", pluginv1api.UploadPhaseNew, pluginv1api.UploadPhaseInProgress, pluginv1api.UploadPhaseUploadError:
		// Process new items
		// For UploadPhaseInProgress, the resource lease logic will process the Upload if the lease is not held by
		// another DataManager. If the DataManager holding the lease has died and/or lease has expired the current node
		// will pick such record in UploadPhaseInProgress status for processing.
	case pluginv1api.UploadPhaseCanceling:
		log.Infof("The upload request is being canceled")
	case pluginv1api.UploadPhaseCanceled:
		log.Infof("The upload request has been canceled, skipping")
		return nil
	default:
		return nil
	}

	// Check if the upload was canceled and trigger cancellation if needed.
	if req.Spec.UploadCancel {
		err := c.triggerUploadCancellation(req)
		if err != nil {
			log.Error("Received error during upload cancellation, skipping.")
		}
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
		Lock:            lock,
		ReleaseOnCancel: false,
		LeaseDuration:   utils.LeaseDuration,
		RenewDeadline:   utils.RenewDeadline,
		RetryPeriod:     utils.RetryPeriod,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				// Current node got the lease process request.
				// Don't mutate the shared cache
				log.Infof("Lock is acquired by current node - %s to process Upload %s.", c.nodeName, req.Name)
				reqCopy := req.DeepCopy()
				processErr = c.processUploadFunc(reqCopy)
				cancel()
			},
			OnStoppedLeading: func() {
				log.Infof("Processed Upload")
			},
			OnNewLeader: func(identity string) {
				if identity == c.nodeName {
					// Same node is trying to acquire or renew the lease, ignore.
					return
				}
				log.Infof("Lock is acquired by another node %s. Current node - %s need not process the Upload %s.", identity, c.nodeName, req.Name)
				cancel()
			},
		},
	})

	return processErr
}

func (c *uploadController) processUpload(req *pluginv1api.Upload) error {
	log := loggerForUpload(c.logger, req)
	log.Infof("Upload starting")
	var err error

	// retrieve upload request for its updated status from k8s api server and filter out completed one
	req, err = c.uploadClient.Uploads(req.Namespace).Get(context.TODO(), req.Name, metav1.GetOptions{})
	if err != nil {
		log.WithError(err).Error("Failed to retrieve upload CR from kubernetes API server")
		return errors.WithStack(err)
	}
	// update req with the one retrieved from k8s api server
	log.WithFields(logrus.Fields{
		"phase":      req.Status.Phase,
		"generation": req.Generation,
	}).Info("Upload request updated by retrieving from kubernetes API server")

	if req.Status.Phase == pluginv1api.UploadPhaseCanceled {
		log.WithField("phase", req.Status.Phase).WithField("generation", req.Generation).Info("The status of upload CR in kubernetes API server is canceled. Skipping it")
		return nil
	}

	if req.Status.Phase == pluginv1api.UploadPhaseCanceling {
		log.WithField("phase", req.Status.Phase).WithField("generation", req.Generation).Info("The status of upload CR in kubernetes API server is canceling. Skipping it")
		return nil
	}

	if req.Status.Phase == pluginv1api.UploadPhaseCompleted {
		log.WithField("phase", req.Status.Phase).WithField("generation", req.Generation).Info("The status of upload CR in kubernetes API server is completed. Skipping it")
		return nil
	}

	// update status to InProgress
	if req.Status.Phase != pluginv1api.UploadPhaseInProgress {
		// update status to InProgress
		req, err = c.patchUploadByStatusWithRetry(req, pluginv1api.UploadPhaseInProgress, "")
		if err != nil {
			return errors.WithStack(err)
		}
	}

	// Call data mover API to do the remote copy
	peID, err := astrolabe.NewProtectedEntityIDFromString(req.Spec.SnapshotID)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to get PEID from SnapshotID, %v. %v", req.Spec.SnapshotID, errors.WithStack(err))
		_, err = c.patchUploadByStatusWithRetry(req, pluginv1api.UploadPhaseUploadError, errMsg)
		if err != nil {
			errMsg = fmt.Sprintf("%v. %v", errMsg, errors.WithStack(err))
		}
		log.Error(errMsg)
		return errors.New(errMsg)
	}

	if req.Spec.BackupRepositoryName != "" {
		var backupRepositoryCR *backupdriverapi.BackupRepository
		backupRepositoryCR, err = utils.GetBackupRepositoryFromBackupRepositoryName(req.Spec.BackupRepositoryName)
		if err != nil {
			log.WithError(err).Errorf("Failed to get BackupRepository from BackupRepositoryName %s", req.Spec.BackupRepositoryName)
			return err
		}
		_, err = c.dataMover.CopyToRepoWithBackupRepository(peID, backupRepositoryCR)
	} else {
		_, err = c.dataMover.CopyToRepo(peID)
	}
	if err != nil {
		log.Infof("CopyToRepo Error Received: %v", err.Error())
		// Check if the request was canceled.
		if errors.Is(err, context.Canceled) {
			log.Infof("The upload of PE %v upload was canceled.", peID.String())
			_, err = c.patchUploadByStatusWithRetry(req, pluginv1api.UploadPhaseCanceled, "The upload was canceled.")
			if err != nil {
				return err
			}
			log.Infof("Upload Cancellation complete.")
			return nil
		} else {
			errMsg := fmt.Sprintf("Failed to upload snapshot, %v, to durable object storage. %v", peID.String(), errors.WithStack(err))
			_, err = c.patchUploadByStatusWithRetry(req, pluginv1api.UploadPhaseUploadError, errMsg)
			if err != nil {
				errMsg = fmt.Sprintf("%v. %v", errMsg, errors.WithStack(err))
			}
			log.Error(errMsg)
			return errors.New(errMsg)
		}
	}

	// Unregister on-going upload
	c.dataMover.UnregisterOngoingUpload(peID)

	// Call snapshot manager API to cleanup the local snapshot
	err = c.snapMgr.DeleteLocalSnapshot(peID)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to clean up local snapshot after uploading snapshot, %v. %v", peID.String(), errors.WithStack(err))
		// TODO: Change the upload CRD definition to add one more phase, such as, UploadPhaseFailedLocalCleanup
		_, err = c.patchUploadByStatusWithRetry(req, pluginv1api.UploadPhaseCleanupFailed, errMsg)
		if err != nil {
			errMsg = fmt.Sprintf("%v. %v", errMsg, errors.WithStack(err))
		}
		log.Error(errMsg)
		return errors.New(errMsg)
	}

	// update status to Completed with path & snapshot id
	req, err = c.patchUploadByStatusWithRetry(req, pluginv1api.UploadPhaseCompleted, "Upload completed")
	if err != nil {
		return errors.WithStack(err)
	}

	log.WithFields(logrus.Fields{
		"phase":      req.Status.Phase,
		"generation": req.Generation,
	}).Info("Upload Completed")

	return nil
}

func (c *uploadController) patchUpload(req *pluginv1api.Upload, mutate func(*pluginv1api.Upload)) (*pluginv1api.Upload, error) {
	log := loggerForUpload(c.logger, req)
	return utils.PatchUpload(req, mutate, c.uploadClient.Uploads(req.Namespace), log)
}

func (c *uploadController) patchUploadByStatusWithRetry(req *pluginv1api.Upload, newPhase pluginv1api.UploadPhase, msg string) (*pluginv1api.Upload, error) {
	var updatedUpload *pluginv1api.Upload
	var err error
	log := loggerForUpload(c.logger, req)
	log.Infof("Ready to call patchUploadByStatus API. Will retry on patch failure of Upload status every %d seconds up to %d seconds.", utils.RetryInterval, utils.RetryMaximum)
	err = wait.PollImmediate(utils.RetryInterval * time.Second, utils.RetryInterval * utils.RetryMaximum * time.Second, func() (bool, error) {
		updatedUpload, err = c.patchUploadByStatus(req, newPhase, msg)
		if err != nil {
			return false, nil
		}
		return true, nil
	})
	log.Debugf("Return from patchUploadByStatus with retry %v times.", utils.RetryMaximum)
	if err != nil {
		log.WithError(err).Errorf("Failed to patch Upload, retry time exceeds maximum %d.", utils.RetryMaximum)
	}
	return updatedUpload, err
}

func (c *uploadController) patchUploadByStatus(req *pluginv1api.Upload, newPhase pluginv1api.UploadPhase, msg string) (*pluginv1api.Upload, error) {
	// update status to Failed
	log := loggerForUpload(c.logger, req)
	oldPhase := req.Status.Phase

	var err error

	switch newPhase {
	case pluginv1api.UploadPhaseCompleted:
		req, err = c.patchUpload(req, func(r *pluginv1api.Upload) {
			r.Status.Phase = newPhase
			r.Status.CompletionTimestamp = &metav1.Time{Time: c.clock.Now()}
			r.Status.Message = msg
		})
	case pluginv1api.UploadPhaseUploadError:
		var retry int32
		req, err = c.patchUpload(req, func(r *pluginv1api.Upload) {
			r.Status.Phase = newPhase
			r.Status.Message = msg
			r.Status.RetryCount = r.Status.RetryCount + 1
			log.Infof("Retry for %d times", r.Status.RetryCount)
			retry = r.Status.RetryCount
			currentBackOff := math.Exp2(float64(retry - 1))
			if currentBackOff >= utils.UPLOAD_MAX_BACKOFF {
				currentBackOff = utils.UPLOAD_MAX_BACKOFF
			}
			r.Status.CurrentBackOff = int32(currentBackOff)
			r.Status.NextRetryTimestamp = &metav1.Time{Time: c.clock.Now().Add(time.Duration(currentBackOff) * time.Minute)}
		})
		if retry > utils.RETRY_WARNING_COUNT {
			errMsg := fmt.Sprintf("Please fix the network issue on the work node, %s", c.nodeName)
			log.Warningf(errMsg)
		}
	case pluginv1api.UploadPhaseCleanupFailed:
		req, err = c.patchUpload(req, func(r *pluginv1api.Upload) {
			r.Status.Phase = newPhase
			r.Status.CompletionTimestamp = &metav1.Time{Time: c.clock.Now()}
			r.Status.Message = msg
		})
	case pluginv1api.UploadPhaseInProgress:
		req, err = c.patchUpload(req, func(r *pluginv1api.Upload) {
			if r.Status.Phase == pluginv1api.UploadPhaseNew {
				r.Status.StartTimestamp = &metav1.Time{Time: c.clock.Now()}
				r.Status.RetryCount = utils.MIN_RETRY
			}
			r.Status.Phase = newPhase
			r.Status.ProcessingNode = c.nodeName
		})
	case pluginv1api.UploadPhaseCanceled:
		req, err = c.patchUpload(req, func(r *pluginv1api.Upload) {
			r.Status.Phase = newPhase
			r.Status.CompletionTimestamp = &metav1.Time{Time: c.clock.Now()}
			r.Status.Message = msg
		})
	case pluginv1api.UploadPhaseCanceling:
		req, err = c.patchUpload(req, func(r *pluginv1api.Upload) {
			r.Status.Phase = newPhase
			r.Status.CompletionTimestamp = &metav1.Time{Time: c.clock.Now()}
			r.Status.Message = msg
		})
	default:
		err = errors.New("Unexpected upload phase")
	}

	if err != nil {
		log.WithError(err).Errorf("Failed to patch Upload from %v to %v", oldPhase, newPhase)
	} else {
		log.Infof("Upload status updated from %v to %v", oldPhase, newPhase)
	}

	return req, err
}

func loggerForUpload(baseLogger logrus.FieldLogger, req *pluginv1api.Upload) logrus.FieldLogger {
	log := baseLogger.WithFields(logrus.Fields{
		"namespace":  req.Namespace,
		"name":       req.Name,
		"snapshotID": req.Spec.SnapshotID,
		"phase":      req.Status.Phase,
		"generation": req.Generation,
	})

	if len(req.OwnerReferences) == 1 {
		log = log.WithField("upload", fmt.Sprintf("%s/%s", req.Namespace, req.OwnerReferences[0].Name))
	}

	return log
}

func (c *uploadController) exponentialBackoffHandler(key string) error {
	log := c.logger.WithField("key", key)
	log.Info("Running exponentialBackoffHandler")

	ns, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		log.WithError(err).Error("Failed to split the key of queue item")
		c.queue.Forget(key)
		return nil
	}

	req, err := c.uploadLister.Uploads(ns).Get(name)
	if apierrors.IsNotFound(err) {
		log.Error("Upload is not found")
		c.queue.Forget(key)
		return nil
	}
	if err != nil {
		return errors.Wrap(err, "Failed to get Upload")
	}
	log.Infof("Re-adding failed upload to the queue")
	c.queue.AddAfter(key, time.Duration(req.Status.CurrentBackOff)*time.Minute)
	return nil
}

func (c *uploadController) triggerUploadCancellation(req *pluginv1api.Upload) error {
	log := loggerForUpload(c.logger, req)
	cancelPeId, err := astrolabe.NewProtectedEntityIDFromString(req.Spec.SnapshotID)
	if err != nil {
		log.Errorf("Error received when processing cancel")
		return err
	}
	uploadStatus := c.dataMover.IsUploading(cancelPeId)
	if !uploadStatus {
		log.Infof("Current node: %v is not processing the upload, skipping", c.nodeName)
		return nil
	}
	_, err = c.patchUploadByStatusWithRetry(req, pluginv1api.UploadPhaseCanceling, "Canceling on-going upload to repository.")
	if err != nil {
		log.WithError(err).Error("Failed to patch ongoing Upload to Canceling state")
		return err
	}
	log.Infof("Current node: %v is processing the upload for PE %v, triggering cancel", c.nodeName, cancelPeId.String())
	err = c.dataMover.CancelUpload(cancelPeId)
	if err != nil {
		return err
	}
	log.Infof("Upload cancellation trigger on current node: %v for PE %v is complete.", c.nodeName, cancelPeId.String())
	return nil
}
