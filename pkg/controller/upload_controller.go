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
				log.Infof("Processed Upload")
			},
			OnNewLeader: func(identity string) {
				if identity == c.nodeName {
					// Same node is trying to acquire or renew the lease, ignore.
					return
				}
				log.Infof("Lock is acquired by another node %s. Current node - %s need not process the Upload.", identity, c.nodeName)
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
		log.Debug("Upload CR is not New or InProgress, skipping")
		return
	}

	log.Infof("Filtering out the upload request from nodes other than %v", c.nodeName)
	peID, err := astrolabe.NewProtectedEntityIDFromString(req.Spec.SnapshotID)
	if err != nil {
		log.WithError(err).Errorf("Failed to extract volume ID from snapshot ID, %v", req.Spec.SnapshotID)
		return
	}

	uploadNodeName, err := utils.RetrievePodNodesByVolumeId(peID.GetID())
	if err != nil {
		log.WithError(err).Errorf("Failed to retrieve pod nodes from volume ID, %v", peID.String())
		return
	}

	log.Infof("Current node: %v. Expected node for uploading the upload CR: %v", c.nodeName, uploadNodeName)
	if c.nodeName != uploadNodeName {
		return
	}

	log.Infof("Enqueueing upload")
	c.enqueue(obj)
}

func (c *uploadController) patchUpload(req *pluginv1api.Upload, mutate func(*pluginv1api.Upload)) (*pluginv1api.Upload, error) {
	log := loggerForUpload(c.logger, req)

	// Record original json
	oldData, err := json.Marshal(req)
	if err != nil {
		log.WithError(err).Error("Failed to marshall original Upload")
		return nil, err
	}

	// Mutate
	mutate(req)

	// Record new json
	newData, err := json.Marshal(req)
	if err != nil {
		log.WithError(err).Error("Failed to marshall updated Upload")
		return nil, err
	}

	patchBytes, err := jsonpatch.CreateMergePatch(oldData, newData)
	if err != nil {
		log.WithError(err).Error("Failed to creat json merge patch for Upload")
		return nil, err
	}

	req, err = c.uploadClient.Uploads(req.Namespace).Patch(req.Name, types.MergePatchType, patchBytes)
	if err != nil {
		log.WithError(err).Error("Failed to patch Upload")
		return nil, err
	}

	return req, nil
}

func (c *uploadController) processBackup(req *pluginv1api.Upload) error {
	log := loggerForUpload(c.logger, req)
	log.Infof("Upload starting")
	var err error

	// retrieve upload request for its updated status from k8s api server and filter out completed one
	req, err = c.uploadClient.Uploads(req.Namespace).Get(req.Name, metav1.GetOptions{})
	if err != nil {
		log.WithError(err).Error("Failed to retrieve upload CR from kubernetes API server")
		return errors.WithStack(err)
	}
	// update req with the one retrieved from k8s api server
	log.WithField("phase", req.Status.Phase).WithField("generation", req.Generation).Info("Upload request updated by retrieving from kubernetes API server")

	if req.Status.Phase == pluginv1api.UploadPhaseCompleted {
		log.WithField("phase", req.Status.Phase).WithField("generation", req.Generation).Info("The status of upload CR in kubernetes API server is completed. Skipping it")
		return nil
	}

	if req.Status.Phase != pluginv1api.UploadPhaseInProgress {
		// update status to InProgress
		req, err = patchUploadByStatus(c, req, pluginv1api.UploadPhaseInProgress, "")
		if err != nil {
			return errors.WithStack(err)
		}
	}

	// Call data mover API to do the remote copy
	peID, err := astrolabe.NewProtectedEntityIDFromString(req.Spec.SnapshotID)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to get PEID from SnapshotID, %v. %v", req.Spec.SnapshotID, errors.WithStack(err))
		_, err = patchUploadByStatus(c, req, pluginv1api.UploadPhaseFailed, errMsg)
		if err != nil {
			errMsg = fmt.Sprintf("%v. %v", errMsg, errors.WithStack(err))
		}
		log.Error(errMsg)
		return errors.New(errMsg)
	}

	_, err = c.dataMover.CopyToRepo(peID)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to upload snapshot, %v, to durable object storage. %v", peID.String(), errors.WithStack(err))
		_, err = patchUploadByStatus(c, req, pluginv1api.UploadPhaseFailed, errMsg)
		if err != nil {
			errMsg = fmt.Sprintf("%v. %v", errMsg, errors.WithStack(err))
		}
		log.Error(errMsg)
		return errors.New(errMsg)
	}

	// Call snapshot manager API to cleanup the local snapshot
	err = c.snapMgr.DeleteProtectedEntitySnapshot(peID, false)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to clean up local snapshot after uploading snapshot, %v. %v", peID.String(), errors.WithStack(err))
		// TODO: Change the upload CRD definition to add one more phase, such as, UploadPhaseFailedLocalCleanup
		_, err = patchUploadByStatus(c, req, pluginv1api.UploadPhaseFailed, errMsg)
		if err != nil {
			errMsg = fmt.Sprintf("%v. %v", errMsg, errors.WithStack(err))
		}
		log.Error(errMsg)
		return errors.New(errMsg)
	}

	// update status to Completed with path & snapshot id
	req, err = patchUploadByStatus(c, req, pluginv1api.UploadPhaseCompleted, "Upload completed")
	if err != nil {
		return errors.WithStack(err)
	}

	log.WithField("phase", req.Status.Phase).WithField("generation", req.Generation).Info("Upload completed")
	return nil
}

func loggerForUpload(baseLogger logrus.FieldLogger, req *pluginv1api.Upload) logrus.FieldLogger {
	log := baseLogger.WithFields(logrus.Fields{
		"namespace":  req.Namespace,
		"name":       req.Name,
		"phase":      req.Status.Phase,
		"generation": req.Generation,
	})

	if len(req.OwnerReferences) == 1 {
		log = log.WithField("upload", fmt.Sprintf("%s/%s", req.Namespace, req.OwnerReferences[0].Name))
	}

	return log
}

func patchUploadByStatus(c *uploadController, req *pluginv1api.Upload, newPhase pluginv1api.UploadPhase, msg string) (*pluginv1api.Upload, error) {
	// update status to Failed
	log := loggerForUpload(c.logger, req)
	oldPhase := req.Status.Phase

	var err error

	switch newPhase {
	case pluginv1api.UploadPhaseCompleted:
		req, err = c.patchUpload(req, func (r *pluginv1api.Upload){
		r.Status.Phase = newPhase
		r.Status.CompletionTimestamp = &metav1.Time{Time: c.clock.Now()}
		r.Status.Message = msg
	})
	case pluginv1api.UploadPhaseFailed:
		req, err = c.patchUpload(req, func (r *pluginv1api.Upload){
		r.Status.Phase = newPhase
		r.Status.CompletionTimestamp = &metav1.Time{Time: c.clock.Now()}
		r.Status.Message = msg
	})
	case pluginv1api.UploadPhaseInProgress:
		req, err = c.patchUpload(req, func (r *pluginv1api.Upload){
		if r.Status.Phase == pluginv1api.UploadPhaseNew {
			r.Status.StartTimestamp = &metav1.Time{Time: c.clock.Now()}
		}
		r.Status.Phase = newPhase
		r.Status.ProcessingNode = c.nodeName
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

