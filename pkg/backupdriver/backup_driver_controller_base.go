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

package backupdriver

import (
	"context"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/backuprepository"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/constants"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/client-go/informers/core/v1"
	"strings"
	"time"

	"k8s.io/client-go/rest"

	"github.com/sirupsen/logrus"
	backupdriverapi "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/backupdriver/v1alpha1"
	datamoverapi "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/datamover/v1alpha1"
	backupdriverclientset "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned/typed/backupdriver/v1alpha1"
	datamoverclientset "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned/typed/datamover/v1alpha1"
	backupdriverinformers "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/informers/externalversions"
	backupdriverlisters "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/listers/backupdriver/v1alpha1"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/snapshotmgr"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

// BackupDriverController is the interface for backup driver controller
type BackupDriverController interface {
	// Run starts the controller.
	Run(ctx context.Context, workers int)
}

type backupDriverController struct {
	name   string
	logger logrus.FieldLogger

	// Supervisor Cluster KubeClient for Guest Cluster
	svcKubeConfig *rest.Config

	backupdriverClient    *backupdriverclientset.BackupdriverV1alpha1Client
	datamoverClient       *datamoverclientset.DatamoverV1alpha1Client
	svcBackupdriverClient *backupdriverclientset.BackupdriverV1alpha1Client

	// Supervisor Cluster namespace
	svcNamespace string

	rateLimiter workqueue.RateLimiter

	// All the initialized cache sync
	cacheSyncs []cache.InformerSynced

	// PV Lister
	pvLister corelisters.PersistentVolumeLister

	// PVC Lister
	pvcLister corelisters.PersistentVolumeClaimLister
	// claim queue
	claimQueue workqueue.RateLimitingInterface

	// Supervisor Cluster PVC Lister
	svcPVCLister corelisters.PersistentVolumeClaimLister

	// BackupRepositoryClaim Lister
	backupRepositoryClaimLister backupdriverlisters.BackupRepositoryClaimLister
	// backupRepositoryClaim queue
	backupRepositoryClaimQueue workqueue.RateLimitingInterface

	// Supervisor Cluster BackupRepositoryClaim Lister
	svcBackupRepositoryClaimLister backupdriverlisters.BackupRepositoryClaimLister

	// BackupRepository Lister
	backupRepositoryLister backupdriverlisters.BackupRepositoryLister

	// Snapshot queue
	snapshotQueue workqueue.RateLimitingInterface
	// Snapshot Lister
	snapshotLister backupdriverlisters.SnapshotLister

	// Supervisor snapshot queue in guest
	svcSnapshotQueue workqueue.RateLimitingInterface

	// CloneFromSnapshot queue
	cloneFromSnapshotQueue workqueue.RateLimitingInterface
	// CloneFromSnapshot Lister
	cloneFromSnapshotLister backupdriverlisters.CloneFromSnapshotLister

	// Supervisor Cluster CloneFromSnapshot Lister
	svcCloneFromSnapshotLister backupdriverlisters.CloneFromSnapshotLister

	// Upload queue
	uploadQueue workqueue.RateLimitingInterface

	// Secret queue
	secretQueue workqueue.RateLimitingInterface

	// DeleteSnapshot queue
	deleteSnapshotQueue workqueue.RateLimitingInterface
	// DeleteSnapshot Lister
	deleteSnapshotLister backupdriverlisters.DeleteSnapshotLister
	// DeleteSnapshot Synced
	deleteSnapshotSynced cache.InformerSynced

	// Map supervisor cluster snapshot CRs to guest cluster snapshot CRs
	svcSnapshotMap map[string]string

	// Snapshot manager
	snapManager *snapshotmgr.SnapshotManager
}

// NewBackupDriverController returns a BackupDriverController.
func NewBackupDriverController(
	name string,
	logger logrus.FieldLogger,
	backupdriverClient *backupdriverclientset.BackupdriverV1alpha1Client,
	datamoverclientset *datamoverclientset.DatamoverV1alpha1Client,
	svcBackupdriverClient *backupdriverclientset.BackupdriverV1alpha1Client,
	svcKubeConfig *rest.Config,
	svcNamespace string,
	resyncPeriod time.Duration,
	informerFactory informers.SharedInformerFactory,
	backupdriverInformerFactory backupdriverinformers.SharedInformerFactory,
	svcInformerFactory informers.SharedInformerFactory,
	svcBackupdriverInformerFactory backupdriverinformers.SharedInformerFactory,
	snapManager *snapshotmgr.SnapshotManager,
	rateLimiter workqueue.RateLimiter) BackupDriverController {

	var cacheSyncs []cache.InformerSynced

	pvcInformer := informerFactory.Core().V1().PersistentVolumeClaims()
	pvInformer := informerFactory.Core().V1().PersistentVolumes()

	backupRepositoryInformer := backupdriverInformerFactory.Backupdriver().V1alpha1().BackupRepositories()
	backupRepositoryClaimInformer := backupdriverInformerFactory.Backupdriver().V1alpha1().BackupRepositoryClaims()
	snapshotInformer := backupdriverInformerFactory.Backupdriver().V1alpha1().Snapshots()
	cloneFromSnapshotInformer := backupdriverInformerFactory.Backupdriver().V1alpha1().CloneFromSnapshots()
	deleteSnapshotInformer := backupdriverInformerFactory.Backupdriver().V1alpha1().DeleteSnapshots()
	uploadInformer := backupdriverInformerFactory.Datamover().V1alpha1().Uploads()

	cacheSyncs = append(cacheSyncs,
		pvcInformer.Informer().HasSynced,
		pvInformer.Informer().HasSynced,
		backupRepositoryInformer.Informer().HasSynced,
		backupRepositoryClaimInformer.Informer().HasSynced,
		snapshotInformer.Informer().HasSynced,
		cloneFromSnapshotInformer.Informer().HasSynced,
		deleteSnapshotInformer.Informer().HasSynced,
		uploadInformer.Informer().HasSynced)

	claimQueue := workqueue.NewNamedRateLimitingQueue(rateLimiter, "backup-driver-claim-queue")
	snapshotQueue := workqueue.NewNamedRateLimitingQueue(rateLimiter, "backup-driver-snapshot-queue")
	cloneFromSnapshotQueue := workqueue.NewNamedRateLimitingQueue(rateLimiter, "backup-driver-clone-queue")
	backupRepositoryClaimQueue := workqueue.NewNamedRateLimitingQueue(rateLimiter, "backup-driver-brc-queue")
	deleteSnapshotQueue := workqueue.NewNamedRateLimitingQueue(rateLimiter, "backup-driver-delete-snapshot-queue")
	uploadQueue := workqueue.NewNamedRateLimitingQueue(rateLimiter, "backup-driver-upload-queue")
	svcSnapshotQueue := workqueue.NewNamedRateLimitingQueue(rateLimiter, "backup-driver-svc-snapshot-queue")
	secretQueue := workqueue.NewNamedRateLimitingQueue(rateLimiter, "backup-driver-secret-queue")

	var svcSnapshotMap map[string]string
	var secretInformer v1.SecretInformer

	// Configure supervisor cluster queues and caches in the guest
	// We watch supervisor snapshot CRs for the upload status. If local mode is set, we do not have to watch for upload status
	if svcKubeConfig != nil {
		svcSnapshotMap = make(map[string]string)
		svcSnapshotInformer := svcBackupdriverInformerFactory.Backupdriver().V1alpha1().Snapshots()

		cacheSyncs = append(cacheSyncs,
			svcSnapshotInformer.Informer().HasSynced)
	} else {
		// Watch for Secret Changes only in Supervisor/Vanilla setup.
		secretInformer = informerFactory.Core().V1().Secrets()
		cacheSyncs = append(cacheSyncs, secretInformer.Informer().HasSynced)
	}

	ctrl := &backupDriverController{
		name:                        name,
		logger:                      logger.WithField("controller", name),
		svcKubeConfig:               svcKubeConfig,
		backupdriverClient:          backupdriverClient,
		datamoverClient:             datamoverclientset,
		svcBackupdriverClient:       svcBackupdriverClient,
		snapManager:                 snapManager,
		pvLister:                    pvInformer.Lister(),
		pvcLister:                   pvcInformer.Lister(),
		claimQueue:                  claimQueue,
		svcNamespace:                svcNamespace,
		snapshotLister:              snapshotInformer.Lister(),
		snapshotQueue:               snapshotQueue,
		cloneFromSnapshotLister:     cloneFromSnapshotInformer.Lister(),
		cloneFromSnapshotQueue:      cloneFromSnapshotQueue,
		backupRepositoryLister:      backupRepositoryInformer.Lister(),
		backupRepositoryClaimLister: backupRepositoryClaimInformer.Lister(),
		backupRepositoryClaimQueue:  backupRepositoryClaimQueue,
		deleteSnapshotQueue:         deleteSnapshotQueue,
		deleteSnapshotLister:        deleteSnapshotInformer.Lister(),
		uploadQueue:                 uploadQueue,
		svcSnapshotQueue:            svcSnapshotQueue,
		secretQueue:                 secretQueue,
		cacheSyncs:                  cacheSyncs,
		svcSnapshotMap:              svcSnapshotMap,
	}

	pvcInformer.Informer().AddEventHandlerWithResyncPeriod(
		cache.ResourceEventHandlerFuncs{
			//AddFunc:    func(obj interface{}) { ctrl.enqueueClaim(obj) },
			//UpdateFunc: func(oldObj, newObj interface{}) { ctrl.enqueueClaim(newObj) },
			//DeleteFunc: func(obj interface{}) { ctrl.delClaim(obj) },
		},
		resyncPeriod,
	)

	pvInformer.Informer().AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		//AddFunc:    rc.addPV,
		//UpdateFunc: rc.updatePV,
		//DeleteFunc: rc.deletePV,
	}, resyncPeriod)

	snapshotInformer.Informer().AddEventHandlerWithResyncPeriod(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    func(obj interface{}) { ctrl.enqueueSnapshot(obj) },
			UpdateFunc: func(_, obj interface{}) { ctrl.enqueueSnapshot(obj) },
			DeleteFunc: func(obj interface{}) { ctrl.delSnapshot(obj) },
		},
		resyncPeriod,
	)

	cloneFromSnapshotInformer.Informer().AddEventHandlerWithResyncPeriod(
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) { ctrl.enqueueCloneFromSnapshot(obj) },
			// UpdateFunc is not needed now. When adding it backup, make sure no multiple CloneFromSnapshot CR's created for the same request
			//UpdateFunc: func(oldObj, newObj interface{}) { ctrl.enqueueCloneFromSnapshot(newObj) },
			//DeleteFunc: func(obj interface{}) { ctrl.delCloneFromSnapshot(obj) },
		},
		resyncPeriod,
	)

	backupRepositoryInformer.Informer().AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		//AddFunc:    rc.addBackupRepository,
		//UpdateFunc: rc.updateBackupRepository,
		//DeleteFunc: rc.deleteBackupRepository,
	}, resyncPeriod)

	backupRepositoryClaimInformer.Informer().AddEventHandlerWithResyncPeriod(
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) { ctrl.enqueueBackupRepositoryClaim(obj) },
			//UpdateFunc: func(oldObj, newObj interface{}) { ctrl.enqueueBackupRepositoryClaim(newObj) },
			DeleteFunc: func(obj interface{}) { ctrl.dequeBackupRepositoryClaim(obj) },
		},
		resyncPeriod,
	)

	deleteSnapshotInformer.Informer().AddEventHandlerWithResyncPeriod(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    func(obj interface{}) { ctrl.enqueueDeleteSnapshot(obj) },
			UpdateFunc: func(oldObj, newObj interface{}) { ctrl.enqueueDeleteSnapshot(newObj) },
			DeleteFunc: func(obj interface{}) { ctrl.dequeDeleteSnapshot(obj) },
		},
		resyncPeriod,
	)

	uploadInformer.Informer().AddEventHandlerWithResyncPeriod(
		cache.ResourceEventHandlerFuncs{
			//AddFunc:    func(obj interface{}) { ctrl.enqueueUpload(obj) },
			UpdateFunc: func(oldObj, newObj interface{}) { ctrl.updateUpload(newObj) },
			//DeleteFunc: func(obj interface{}) { ctrl.delUpload(obj) },
		},
		resyncPeriod,
	)

	if svcKubeConfig != nil {
		// Configure supervisor cluster informers in the guest
		svcSnapshotInformer := svcBackupdriverInformerFactory.Backupdriver().V1alpha1().Snapshots()
		svcSnapshotInformer.Informer().AddEventHandlerWithResyncPeriod(
			cache.ResourceEventHandlerFuncs{
				//AddFunc:    func(obj interface{}) { ctrl.enqueueSvcSnapshot(obj) },
				UpdateFunc: func(oldObj, newObj interface{}) { ctrl.updateSvcSnapshot(newObj) },
				//DeleteFunc: func(obj interface{}) { ctrl.delSvcSnapshot(obj) },
			},
			resyncPeriod)
	}
	// Configure secret informer in Supervisor and Vanilla setup only.
	if secretInformer != nil {
		secretInformer.Informer().AddEventHandlerWithResyncPeriod(
			cache.FilteringResourceEventHandler{
				FilterFunc: utils.GetVcConfigSecretFilterFunc(logger),
				Handler: cache.ResourceEventHandlerFuncs{
					AddFunc:    func(obj interface{}) { ctrl.enqueueSecret(obj) },
					UpdateFunc: func(oldObj, newObj interface{}) { ctrl.enqueueSecret(newObj) },
				},
			},
			constants.DefaultSecretResyncPeriod)
	}
	return ctrl
}

// getKey helps to get the resource name from resource object
func (ctrl *backupDriverController) getKey(obj interface{}) (string, error) {
	if unknown, ok := obj.(cache.DeletedFinalStateUnknown); ok && unknown.Obj != nil {
		obj = unknown.Obj
	}
	objKey, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		ctrl.logger.Errorf("Failed to get key from object: %v", err)
		return "", err
	}
	ctrl.logger.Debugf("getKey: key %s", objKey)
	return objKey, nil
}

// Run starts the controller.
func (ctrl *backupDriverController) Run(
	ctx context.Context, workers int) {
	defer ctrl.claimQueue.ShutDown()
	defer ctrl.backupRepositoryClaimQueue.ShutDown()
	defer ctrl.snapshotQueue.ShutDown()
	defer ctrl.cloneFromSnapshotQueue.ShutDown()
	defer ctrl.deleteSnapshotQueue.ShutDown()
	defer ctrl.uploadQueue.ShutDown()
	defer ctrl.svcSnapshotQueue.ShutDown()
	defer ctrl.secretQueue.ShutDown()

	ctrl.logger.Infof("Starting backup driver controller")
	defer ctrl.logger.Infof("Shutting down backup driver controller")

	stopCh := ctx.Done()

	ctrl.logger.Infof("Waiting for caches to sync")

	if !cache.WaitForCacheSync(stopCh, ctrl.cacheSyncs...) {
		ctrl.logger.Errorf("Cannot sync caches")
		return
	}
	ctrl.logger.Infof("Caches are synced")

	for i := 0; i < workers; i++ {
		//go wait.Until(ctrl.pvcWorker, 0, stopCh)
		//go wait.Until(ctrl.pvWorker, 0, stopCh)
		go wait.Until(ctrl.snapshotWorker, 0, stopCh)
		go wait.Until(ctrl.cloneFromSnapshotWorker, 0, stopCh)
		go wait.Until(ctrl.backupRepositoryClaimWorker, 0, stopCh)
		go wait.Until(ctrl.deleteSnapshotWorker, 0, stopCh)
		go wait.Until(ctrl.uploadWorker, 0, stopCh)

		if ctrl.svcKubeConfig != nil {
			go wait.Until(ctrl.svcSnapshotWorker, 0, stopCh)
		} else {
			go wait.Until(ctrl.secretWorker, 0, stopCh)
		}
	}

	<-stopCh
}

func (ctrl *backupDriverController) secretWorker() {
	ctrl.logger.Debug("secretWorker: Enter secretWorker")

	key, quit := ctrl.secretQueue.Get()
	if quit {
		return
	}
	defer ctrl.secretQueue.Done(key)

	if err := ctrl.syncSecretByKey(key.(string)); err != nil {
		ctrl.secretQueue.AddRateLimited(key)
	} else {
		ctrl.secretQueue.Forget(key)
	}
}

func (ctrl *backupDriverController) syncSecretByKey(key string) error {
	ctrl.logger.Debugf("syncSecretByKey: Started Secret processing %s", key)
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		ctrl.logger.Errorf("Split meta namespace key of secret %s failed: %v", key, err)
		return err
	}
	// Retrieve the latest Secret.
	params := make(map[string]interface{})
	err = utils.RetrieveVcConfigSecret(params, nil, ctrl.logger)
	if err != nil {
		ctrl.logger.Errorf("Failed to retrieve the latest vc config secret")
		return err
	}
	ctrl.logger.Debugf("Successfully retrieved latest vSphere VC credentials.")
	err = ctrl.snapManager.ReloadSnapshotManagerIvdPetmConfig(params)
	if err != nil {
		ctrl.logger.Errorf("Secret %s/%s Reload failed, err: %v", namespace, name, err)
		return err
	}
	ctrl.logger.Debugf("Successfully processed updates in vc configuration.")
	return nil
}

func (ctrl *backupDriverController) enqueueSecret(obj interface{}) {
	// Beware of "xxx deleted" events
	if unknown, ok := obj.(cache.DeletedFinalStateUnknown); ok && unknown.Obj != nil {
		obj = unknown.Obj
	}
	if secretItem, ok := obj.(*corev1.Secret); ok {
		ctrl.logger.Debugf("enqueueSecret on update: %s", secretItem.Name)
		objName, err := cache.DeletionHandlingMetaNamespaceKeyFunc(secretItem)
		if err != nil {
			ctrl.logger.Errorf("failed to get key from object: %v, %v", err, secretItem)
			return
		}
		ctrl.logger.Debugf("enqueueSecret: enqueued %q for sync", objName)
		ctrl.secretQueue.Add(objName)
	}
}

// snapshotWorker is the main worker for snapshot request.
func (ctrl *backupDriverController) snapshotWorker() {
	ctrl.logger.Debugf("snapshotWorker: Enter snapshotWorker")

	key, quit := ctrl.snapshotQueue.Get()
	if quit {
		return
	}
	defer ctrl.snapshotQueue.Done(key)

	if err := ctrl.syncSnapshotByKey(key.(string)); err != nil {
		// Put snapshot back to the queue so that we can retry later.
		ctrl.snapshotQueue.AddRateLimited(key)
	} else {
		ctrl.snapshotQueue.Forget(key)
	}
}

// syncSnapshotByKey processes one Snapshot CRD
func (ctrl *backupDriverController) syncSnapshotByKey(key string) error {
	ctrl.logger.Debugf("syncSnapshotByKey: Started Snapshot processing %s", key)

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		ctrl.logger.Errorf("Split meta namespace key of snapshot %s failed: %v", key, err)
		return err
	}

	// Always retrieve up-to-date snapshot CR from API server
	snapshot, err := ctrl.backupdriverClient.Snapshots(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			ctrl.logger.Infof("Snapshot %s/%s is deleted, no need to process it", namespace, name)
			return nil
		}
		ctrl.logger.Errorf("Get Snapshot %s/%s failed: %v", namespace, name, err)
		return err
	}

	// For guest clusters, add supervisor Snapshot to guest snapshot mapping if not already added
	// The key is <supervisor snapshot CR name> and value is <guest snapshot namespace>:<guest snapshot CR name>
	if ctrl.svcKubeConfig != nil && snapshot.Status.SvcSnapshotName != "" {
		if _, ok := ctrl.svcSnapshotMap[snapshot.Status.SvcSnapshotName]; !ok {
			ctrl.svcSnapshotMap[snapshot.Status.SvcSnapshotName] = snapshot.Namespace + ":" + snapshot.Name
			ctrl.logger.Infof("Added supervisor snapshot %s to guest snapshot %s mapping",
				snapshot.Status.SvcSnapshotName, ctrl.svcSnapshotMap[snapshot.Status.SvcSnapshotName])
		}
	}

	if snapshot.Status.Phase != backupdriverapi.SnapshotPhaseNew {
		ctrl.logger.Debugf("Skipping snapshot, %v, which is not in New phase. Current phase: %v", key, snapshot.Status.Phase)
		if snapshot.Status.Phase == backupdriverapi.SnapshotPhaseUploaded {
			// If Snapshot CR status reaches terminal state, Snapshot CR should be deleted after clean up window
			ctrl.logger.Debug("Snapshot CR is in Uploaded phase.")
			now := time.Now()
			if now.After(snapshot.Status.CompletionTimestamp.Add(constants.DefaultCRCleanUpWindow * time.Hour)) {
				ctrl.logger.Infof("Snapshot CR %s reaches phase %v more than %v hours, deleting this CR.", snapshot.Name, snapshot.Status.Phase, constants.DefaultCRCleanUpWindow)
				err := ctrl.backupdriverClient.Snapshots(snapshot.Namespace).Delete(context.TODO(), snapshot.Name, metav1.DeleteOptions{})
				if err != nil {
					ctrl.logger.WithError(err).Errorf("Failed to delete Snapshot CR which is in %v phase.", snapshot.Status.Phase)
				}
			}
		}
		return nil
	}

	if snapshot.Spec.SnapshotCancel == false {
		ctrl.logger.Infof("syncSnapshotByKey: calling CreateSnapshot %s/%s", snapshot.Namespace, snapshot.Name)
		return ctrl.createSnapshot(snapshot)
	}

	return nil
}

// enqueueSnapshot adds Snapshot to given work queue.
func (ctrl *backupDriverController) enqueueSnapshot(obj interface{}) {
	// Beware of "xxx deleted" events
	if unknown, ok := obj.(cache.DeletedFinalStateUnknown); ok && unknown.Obj != nil {
		obj = unknown.Obj
	}
	if snapshot, ok := obj.(*backupdriverapi.Snapshot); ok {
		ctrl.logger.Debugf("enqueueSnapshot: %s", snapshot.Name)
		objName, err := cache.DeletionHandlingMetaNamespaceKeyFunc(snapshot)
		if err != nil {
			ctrl.logger.Errorf("failed to get key from object: %v, %v", err, snapshot)
			return
		}
		ctrl.logger.Debugf("enqueueSnapshot: enqueued %q for sync", objName)
		ctrl.snapshotQueue.Add(objName)
	}
}

// delSnapshot process the delete of a snapshot CR
func (ctrl *backupDriverController) delSnapshot(obj interface{}) {
	// In case of guest cluster, cleanup the supervisor snapshot map
	if ctrl.svcKubeConfig != nil {
		if unknown, ok := obj.(cache.DeletedFinalStateUnknown); ok && unknown.Obj != nil {
			obj = unknown.Obj
		}
		if snapshot, ok := obj.(*backupdriverapi.Snapshot); ok {
			if snapshot.Status.SvcSnapshotName != "" {
				if _, ok := ctrl.svcSnapshotMap[snapshot.Status.SvcSnapshotName]; ok {
					ctrl.logger.Infof("Deleting supervisor snapshot %s to guest snapshot %s mapping",
						snapshot.Status.SvcSnapshotName, ctrl.svcSnapshotMap[snapshot.Status.SvcSnapshotName])
					delete(ctrl.svcSnapshotMap, snapshot.Status.SvcSnapshotName)
				}
			}
		}
	}
}

func (ctrl *backupDriverController) pvcWorker() {
}

func (ctrl *backupDriverController) svcPvcWorker() {
}

func (ctrl *backupDriverController) pvWorker() {
}

// cloneFromSnapshotWorker is the main worker for restore request.
func (ctrl *backupDriverController) cloneFromSnapshotWorker() {
	ctrl.logger.Debug("cloneFromSnapshotWorker: Enter cloneFromSnapshotWorker")

	key, quit := ctrl.cloneFromSnapshotQueue.Get()
	if quit {
		return
	}
	defer ctrl.cloneFromSnapshotQueue.Done(key)

	if err := ctrl.syncCloneFromSnapshotByKey(key.(string)); err != nil {
		// Put cloneFromSnapshot back to the queue so that we can retry later.
		ctrl.cloneFromSnapshotQueue.AddRateLimited(key)
	} else {
		ctrl.cloneFromSnapshotQueue.Forget(key)
	}
}

// syncCloneFromSnapshotByKey processes one CloneFromSnapshot CRD
func (ctrl *backupDriverController) syncCloneFromSnapshotByKey(key string) error {
	ctrl.logger.Debugf("syncCloneFromSnapshotByKey: Started CloneFromSnapshot processing %s", key)

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		ctrl.logger.Errorf("Split meta namespace key of CloneFromSnapshot %s failed: %v", key, err)
		return err
	}

	cloneFromSnapshot, err := ctrl.backupdriverClient.CloneFromSnapshots(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			ctrl.logger.Infof("CloneFromSnapshot %s/%s is deleted, no need to process it", namespace, name)
			return nil
		}
		ctrl.logger.Errorf("Get CloneFromSnapshot %s/%s failed: %v", namespace, name, err)
		return err
	}

	switch phase := cloneFromSnapshot.Status.Phase; phase {
	case backupdriverapi.ClonePhaseInProgress:
	case backupdriverapi.ClonePhaseFailed:
	case backupdriverapi.ClonePhaseCanceling:
	case backupdriverapi.ClonePhaseCanceled:
		ctrl.logger.Debugf("Skipping cloneFromSnapshot %s which is not in New or Retry phase. Current phase: %v", key, cloneFromSnapshot.Status.Phase)
		return nil
	case backupdriverapi.ClonePhaseCompleted:
		// If CloneFromSnapshot CR status reaches terminal state, Snapshot CR should be deleted after clean up window
		ctrl.logger.Debug("CloneFromSnapshot CR is in completed phase.")
		now := time.Now()
		if now.After(cloneFromSnapshot.Status.CompletionTimestamp.Add(constants.DefaultCRCleanUpWindow * time.Hour)) {
			ctrl.logger.Infof("CloneFromSnapshot CR %s reaches phase %v more than %v hours, deleting this CR.", cloneFromSnapshot.Name, cloneFromSnapshot.Status.Phase, constants.DefaultCRCleanUpWindow)
			err := ctrl.backupdriverClient.CloneFromSnapshots(cloneFromSnapshot.Namespace).Delete(context.TODO(), cloneFromSnapshot.Name, metav1.DeleteOptions{})
			if err != nil {
				ctrl.logger.WithError(err).Errorf("Failed to delete CloneFromSnapshot CR which is in %v phase.", cloneFromSnapshot.Status.Phase)
			}
		}
		return nil
	default:
		// If phase is ClonePhaseNew or ClonePhaseRetry,
		// or if phase is not initialized, proceed
		// to clone from snapshot
	}

	if cloneFromSnapshot.Spec.CloneCancel == false {
		ctrl.logger.Infof("syncCloneFromSnapshotByKey: calling CloneFromSnapshot %s/%s", cloneFromSnapshot.Namespace, cloneFromSnapshot.Name)
		err := ctrl.cloneFromSnapshot(cloneFromSnapshot)
		if err != nil {
			ctrl.logger.Errorf("cloneFromSnapshot %s/%s failed: %v", namespace, name, err)
			return err
		}
	}

	return nil
}

// enqueueCloneFromSnapshot adds CloneFromSnapshotto given work queue.
func (ctrl *backupDriverController) enqueueCloneFromSnapshot(obj interface{}) {
	ctrl.logger.Debugf("enqueueCloneFromSnapshot: %+v", obj)

	// Beware of "xxx deleted" events
	if unknown, ok := obj.(cache.DeletedFinalStateUnknown); ok && unknown.Obj != nil {
		obj = unknown.Obj
	}
	if cloneFromSnapshot, ok := obj.(*backupdriverapi.CloneFromSnapshot); ok {
		objName, err := cache.DeletionHandlingMetaNamespaceKeyFunc(cloneFromSnapshot)
		if err != nil {
			ctrl.logger.Errorf("failed to get key from object: %v, %v", err, cloneFromSnapshot)
			return
		}
		ctrl.logger.Debugf("enqueueCloneFromSnapshot: enqueued %q for sync", objName)
		ctrl.cloneFromSnapshotQueue.Add(objName)
	}
}

func (ctrl *backupDriverController) svcCloneFromSnapshotWorker() {
}

func (ctrl *backupDriverController) backupRepositoryClaimWorker() {
	ctrl.logger.Debug("backupRepositoryClaimWorker: Enter backupRepositoryClaimWorker")

	key, quit := ctrl.backupRepositoryClaimQueue.Get()
	if quit {
		return
	}
	defer ctrl.backupRepositoryClaimQueue.Done(key)

	if err := ctrl.syncBackupRepositoryClaimByKey(key.(string)); err != nil {
		// Put backuprepositoryclaim back to the queue so that we can retry later.
		ctrl.backupRepositoryClaimQueue.AddRateLimited(key)
	} else {
		ctrl.backupRepositoryClaimQueue.Forget(key)
	}

}

// syncBackupRepositoryClaim processes one BackupRepositoryClaim CRD
func (ctrl *backupDriverController) syncBackupRepositoryClaimByKey(key string) error {
	ctrl.logger.Debugf("syncBackupRepositoryClaimByKey: Started BackupRepositoryClaim processing %s", key)

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		ctrl.logger.Errorf("Split meta namespace key of backupRepositoryClaim %s failed: %v", key, err)
		return err
	}

	brc, err := ctrl.backupRepositoryClaimLister.BackupRepositoryClaims(namespace).Get(name)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			ctrl.logger.Infof("syncBackupRepositoryClaimByKey: BackupRepositoryClaim %s/%s is deleted, no need to process it", namespace, name)
			return nil
		}
		ctrl.logger.Errorf("Get BackupRepositoryClaim %s/%s failed: %v", namespace, name, err)
		return err
	}

	if brc.ObjectMeta.DeletionTimestamp == nil {
		ctx := context.Background()
		var svcBackupRepositoryName string
		// In case of guest clusters, create BackupRepositoryClaim in the supervisor namespace
		if ctrl.svcKubeConfig != nil {
			svcBackupRepositoryName, err = backuprepository.ClaimSvcBackupRepository(ctx, brc, ctrl.svcKubeConfig, ctrl.svcNamespace, ctrl.logger)
			if err != nil {
				ctrl.logger.Errorf("Failed to create Supervisor BackupRepositoryClaim")
				return err
			}
			ctrl.logger.Infof("Created Supervisor BackupRepositoryClaim with BackupRepository %s", svcBackupRepositoryName)
		}

		// Create BackupRepository when a new BackupRepositoryClaim is added
		// Save the supervisor backup repository name to be passed to snapshot manager
		ctrl.logger.Infof("syncBackupRepositoryClaimByKey: Create BackupRepository for BackupRepositoryClaim %s/%s", brc.Namespace, brc.Name)
		// Create BackupRepository when a new BackupRepositoryClaim is added and if the BackupRepository is not already created
		br, err := backuprepository.CreateBackupRepository(ctx, brc, svcBackupRepositoryName, ctrl.backupdriverClient, ctrl.logger)
		if err != nil {
			ctrl.logger.Errorf("Failed to create BackupRepository")
			return err
		}

		err = backuprepository.PatchBackupRepositoryClaim(brc, br.Name, brc.Namespace, ctrl.backupdriverClient)
		if err != nil {
			return err
		}
	}

	return nil
}

// enqueueBackupRepositoryClaimWork adds BackupRepositoryClaim to given work queue.
func (ctrl *backupDriverController) enqueueBackupRepositoryClaim(obj interface{}) {
	ctrl.logger.Debug("Entering enqueueBackupRepositoryClaim")

	// Beware of "xxx deleted" events
	if unknown, ok := obj.(cache.DeletedFinalStateUnknown); ok && unknown.Obj != nil {
		obj = unknown.Obj
	}
	if brc, ok := obj.(*backupdriverapi.BackupRepositoryClaim); ok {
		objName, err := cache.DeletionHandlingMetaNamespaceKeyFunc(brc)
		if err != nil {
			ctrl.logger.Errorf("failed to get key from object: %v, %v", err, brc)
			return
		}
		ctrl.logger.Debugf("enqueueBackupRepositoryClaim: enqueued %q for sync", objName)
		ctrl.backupRepositoryClaimQueue.Add(objName)
	}
}

func (ctrl *backupDriverController) dequeBackupRepositoryClaim(obj interface{}) {
	ctrl.logger.Debugf("Entering dequeBackupRepositoryClaim: Remove BackupRepositoryClaim from queue")

	var key string
	var err error
	if key, err = cache.DeletionHandlingMetaNamespaceKeyFunc(obj); err != nil {
		return
	}

	brc, ok := obj.(*backupdriverapi.BackupRepositoryClaim)
	if !ok || brc == nil {
		return
	}
	// Delete BackupRepository from API server
	if brc.BackupRepository != "" {
		ctrl.logger.Infof("dequeBackupRepositoryClaim: Delete BackupRepository %s for BackupRepositoryClaim %s/%s", brc.BackupRepository, brc.Namespace, brc.Name)
		err = ctrl.backupdriverClient.BackupRepositories().Delete(context.TODO(), brc.BackupRepository, metav1.DeleteOptions{})
		if err != nil && !k8serrors.IsNotFound(err) {
			ctrl.logger.Errorf("Delete BackupRepository %s failed: %v", brc.BackupRepository, err)
			return
		}
	}

	ctrl.backupRepositoryClaimQueue.Forget(key)
	ctrl.backupRepositoryClaimQueue.Done(key)
}

func (ctrl *backupDriverController) deleteSnapshotWorker() {
	ctrl.logger.Debug("deleteSnapshotWorker: Enter deleteSnapshotWorker")

	key, quit := ctrl.deleteSnapshotQueue.Get()
	if quit {
		return
	}
	defer ctrl.deleteSnapshotQueue.Done(key)

	if err := ctrl.syncDeleteSnapshotByKey(key.(string)); err != nil {
		// Put deleteSnapshot back to the queue so that we can retry later.
		ctrl.deleteSnapshotQueue.AddRateLimited(key)
	} else {
		ctrl.deleteSnapshotQueue.Forget(key)
	}
}

func (ctrl *backupDriverController) syncDeleteSnapshotByKey(key string) error {
	ctrl.logger.Debugf("syncDeleteSnapshotByKey: Started DeleteSnapshot processing %s", key)
	//ctx := context.Background()
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		ctrl.logger.Errorf("Split meta namespace key of DeleteSnapshot %s failed: %v", key, err)
		return err
	}

	delSnapshot, err := ctrl.backupdriverClient.DeleteSnapshots(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			ctrl.logger.Infof("DeleteSnapshot %s/%s is deleted, no need to process it", namespace, name)
			return nil
		}
		ctrl.logger.Errorf("Get DeleteSnapshot %s/%s failed: %v", namespace, name, err)
		return err
	}

	if delSnapshot.Status.Phase != backupdriverapi.DeleteSnapshotPhaseNew {
		ctrl.logger.Debugf("Skipping DeleteSnapshot, %v, which is not in New phase. Current phase: %v",
			key, delSnapshot.Status.Phase)
		if delSnapshot.Status.Phase == backupdriverapi.DeleteSnapshotPhaseCompleted {
			// If DeleteSnapshot CR status reaches terminal state, DeleteSnapshot CR should be deleted after clean up window
			ctrl.logger.Debug("DeleteSnapshot CR is in Completed phase.")
			now := time.Now()
			if now.After(delSnapshot.Status.CompletionTimestamp.Add(constants.DefaultCRCleanUpWindow * time.Hour)) {
				ctrl.logger.Infof("DeleteSnapshot CR %s reaches phase %v more than %v hours, deleting this CR.", delSnapshot.Name, delSnapshot.Status.Phase, constants.DefaultCRCleanUpWindow)
				err := ctrl.backupdriverClient.DeleteSnapshots(delSnapshot.Namespace).Delete(context.TODO(), delSnapshot.Name, metav1.DeleteOptions{})
				if err != nil {
					ctrl.logger.WithError(err).Errorf("Failed to delete DeleteSnapshot CR which is in %v phase.", delSnapshot.Status.Phase)
				}
			}
		}
		return nil
	}

	if delSnapshot.ObjectMeta.DeletionTimestamp == nil {
		ctrl.logger.Infof("syncDeleteSnapshotByKey: calling deleteSnapshot %s/%s ", delSnapshot.Namespace, delSnapshot.Name)
		return ctrl.deleteSnapshot(delSnapshot)
	}

	return nil
}

// enqueueDeleteSnapshot adds DeleteSnap given work queue.
func (ctrl *backupDriverController) enqueueDeleteSnapshot(obj interface{}) {
	ctrl.logger.Debugf("enqueueDeleteSnapshot: %+v", obj)
	if unknown, ok := obj.(cache.DeletedFinalStateUnknown); ok && unknown.Obj != nil {
		obj = unknown.Obj
	}
	if brc, ok := obj.(*backupdriverapi.DeleteSnapshot); ok {
		objName, err := cache.DeletionHandlingMetaNamespaceKeyFunc(brc)
		if err != nil {
			ctrl.logger.Errorf("failed to get key from object: %v, %v", err, brc)
			return
		}
		ctrl.logger.Debugf("enqueueDeleteSnapshot: enqueued %q for sync", objName)
		ctrl.deleteSnapshotQueue.Add(objName)
	}
}

func (ctrl *backupDriverController) dequeDeleteSnapshot(obj interface{}) {
	ctrl.logger.Debugf("dequeDeleteSnapshot: Remove DeleteSnapshot %v from queue", obj)

	var key string
	var err error
	if key, err = cache.DeletionHandlingMetaNamespaceKeyFunc(obj); err != nil {
		return
	}

	delSnap, ok := obj.(*backupdriverapi.DeleteSnapshot)
	if !ok || delSnap == nil {
		return
	}
	ctrl.logger.Infof("dequeDeleteSnapshot: Delete DeleteSnapshot for SnapshotID: %s with Namespace: %s Name: %s",
		delSnap.Spec.SnapshotID, delSnap.Namespace, delSnap.Name)
	err = ctrl.backupdriverClient.DeleteSnapshots(delSnap.Namespace).Delete(context.TODO(), delSnap.Name, metav1.DeleteOptions{})
	if err != nil && !k8serrors.IsNotFound(err) {
		ctrl.logger.Errorf("Delete DeleteSnapshot SnapshotID: %s Name: %s failed: %v",
			delSnap.Spec.SnapshotID, delSnap.Name, err)
		return
	}

	ctrl.deleteSnapshotQueue.Forget(key)
	ctrl.deleteSnapshotQueue.Done(key)
}

// uploadWorker is the main worker for upload request.
func (ctrl *backupDriverController) uploadWorker() {
	ctrl.logger.Debugf("uploadWorker: Enter uploadWorker")

	key, quit := ctrl.uploadQueue.Get()
	if quit {
		return
	}
	defer ctrl.uploadQueue.Done(key)

	if err := ctrl.syncUploadByKey(key.(string)); err != nil {
		// Put upload back to the queue so that we can retry later.
		ctrl.uploadQueue.AddRateLimited(key)
	} else {
		ctrl.uploadQueue.Forget(key)
	}
}

// syncUploadByKey processes one Upload CRD
func (ctrl *backupDriverController) syncUploadByKey(key string) error {
	ctrl.logger.Debugf("syncUploadByKey: Started Upload processing %s", key)
	ctx := context.Background()
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		ctrl.logger.Errorf("Split meta namespace key of upload %s failed: %v", key, err)
		return err
	}

	// Always retrieve up-to-date upload CR from API server
	upload, err := ctrl.datamoverClient.Uploads(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			ctrl.logger.Infof("Upload %s/%s is deleted, no need to process it", namespace, name)
			return nil
		}
		ctrl.logger.Errorf("Get Upload %s/%s failed: %v", namespace, name, err)
		return err
	}
	if upload.Spec.SnapshotReference == "" {
		ctrl.logger.Info("No snapshot reference specified. Skipping updating snapshot")
		return nil
	}
	snapshotParts := strings.Split(upload.Spec.SnapshotReference, "/")
	if len(snapshotParts) != 2 {
		ctrl.logger.Info("Invalid snapshot reference %s specified. Skipping updating snapshot", upload.Spec.SnapshotReference)
		return nil

	}

	// Get the corresponding snapshot
	snapshot, err := ctrl.backupdriverClient.Snapshots(snapshotParts[0]).Get(ctx, snapshotParts[1], metav1.GetOptions{})
	if err != nil {
		ctrl.logger.WithError(err).Debug("No matching snapshot found. Skipping updating snapshot")
		return nil
	}

	if snapshot.ObjectMeta.DeletionTimestamp != nil {
		// Snapshot is being deleted. Do nothing
		return nil
	}

	var newSnapshotStatusPhase backupdriverapi.SnapshotPhase
	switch upload.Status.Phase {
	case datamoverapi.UploadPhaseInProgress:
		newSnapshotStatusPhase = backupdriverapi.SnapshotPhaseUploading
	case datamoverapi.UploadPhaseCompleted:
		newSnapshotStatusPhase = backupdriverapi.SnapshotPhaseUploaded
	case datamoverapi.UploadPhaseCanceled:
		newSnapshotStatusPhase = backupdriverapi.SnapshotPhaseCanceled
	case datamoverapi.UploadPhaseCanceling:
		newSnapshotStatusPhase = backupdriverapi.SnapshotPhaseCanceling
	case datamoverapi.UploadPhaseUploadError:
		newSnapshotStatusPhase = backupdriverapi.SnapshotPhaseUploadFailed
	case datamoverapi.UploadPhaseCleanupFailed:
		newSnapshotStatusPhase = backupdriverapi.SnapshotPhaseCleanupFailed
	default:
		ctrl.logger.Debugf("syncUploadByKey: No change needed for upload phase %s", upload.Status.Phase)
		return nil
	}
	snapshotStatusFields := make(map[string]interface{})
	ctrl.logger.Debugf("syncUploadByKey: calling updateSnapshotStatusPhase %s/%s", snapshot.Namespace, snapshot.Name)
	_, err = ctrl.updateSnapshotStatusPhase(ctx, snapshot.Namespace, snapshot.Name, newSnapshotStatusPhase, snapshotStatusFields)
	return err
}

// updateUpload adds updated Uploads to given work queue.
func (ctrl *backupDriverController) updateUpload(newObj interface{}) {
	ctrl.logger.Debugf("updateUpload: New: %+v", newObj)

	// Beware of "xxx deleted" events
	if unknown, ok := newObj.(cache.DeletedFinalStateUnknown); ok && unknown.Obj != nil {
		newObj = unknown.Obj
	}
	uploadNew, okNew := newObj.(*datamoverapi.Upload)

	if okNew {
		objName, err := cache.DeletionHandlingMetaNamespaceKeyFunc(uploadNew)
		if err != nil {
			ctrl.logger.Errorf("failed to get key from object: %v, %v", err, uploadNew)
			return
		}
		ctrl.logger.Debugf("updateUpload: enqueued %q for sync", objName)
		ctrl.uploadQueue.Add(objName)
	}
}

// svcSnapshotWorker is the main worker for supervisor Snapshot update request.
func (ctrl *backupDriverController) svcSnapshotWorker() {
	ctrl.logger.Debug("svcSnapshotWorker: Enter svcSnapshotWorker")

	key, quit := ctrl.svcSnapshotQueue.Get()
	if quit {
		return
	}
	defer ctrl.svcSnapshotQueue.Done(key)

	if err := ctrl.syncSvcSnapshotByKey(key.(string)); err != nil {
		// Put upload back to the queue so that we can retry later.
		ctrl.svcSnapshotQueue.AddRateLimited(key)
	} else {
		ctrl.svcSnapshotQueue.Forget(key)
	}
}

// syncSvcSnapshotByKey processes one supervisor snapshot CRD
func (ctrl *backupDriverController) syncSvcSnapshotByKey(key string) error {
	ctx := context.Background()
	ctrl.logger.Debugf("syncSvcSnapshotByKey: Started SvcSnapshot processing %s", key)

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		ctrl.logger.Errorf("Split meta namespace key of SvcSnapshot %s failed: %v", key, err)
		return err
	}

	// Always retrieve up-to-date SvcSnapshot CR from API server
	svcSnapshot, err := ctrl.svcBackupdriverClient.Snapshots(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			ctrl.logger.Infof("SvcSnapshot %s/%s is deleted, no need to process it", namespace, name)
			return nil
		}
		ctrl.logger.Errorf("Get SvcSnapshot %s/%s failed: %v", namespace, name, err)
		return err
	}

	// Get the corresponding guest snapshot
	if _, ok := ctrl.svcSnapshotMap[svcSnapshot.Name]; !ok {
		ctrl.logger.Debugf("The supervisor snapshot %s does not belong to this cluster, or is still not Snapshotted. Skipping updating snapshot status", svcSnapshot.Name)
		return nil
	}

	snapshotParts := strings.Split(ctrl.svcSnapshotMap[svcSnapshot.Name], ":")
	snapshotNamespace := snapshotParts[0]
	snapshotName := snapshotParts[1]
	snapshot, err := ctrl.backupdriverClient.Snapshots(snapshotNamespace).Get(context.TODO(), snapshotName, metav1.GetOptions{})
	if err != nil {
		ctrl.logger.WithError(err).Debug("No matching snapshot found. Skipping updating snapshot")
		return nil
	}
	if snapshot.ObjectMeta.DeletionTimestamp != nil {
		// Snapshot is being deleted. Do nothing
		return nil
	}

	var newSnapshotStatusPhase backupdriverapi.SnapshotPhase
	switch svcSnapshot.Status.Phase {
	case backupdriverapi.SnapshotPhaseUploading:
		fallthrough
	case backupdriverapi.SnapshotPhaseUploaded:
		fallthrough
	case backupdriverapi.SnapshotPhaseCanceled:
		fallthrough
	case backupdriverapi.SnapshotPhaseCanceling:
		fallthrough
	case backupdriverapi.SnapshotPhaseCleanupFailed:
		fallthrough
	case backupdriverapi.SnapshotPhaseUploadFailed:
		fallthrough
	case backupdriverapi.SnapshotPhaseSnapshotFailed:
		newSnapshotStatusPhase = svcSnapshot.Status.Phase
	default:
		ctrl.logger.Debugf("syncSvcSnapshotByKey: No change needed for snapshot phase %s", svcSnapshot.Status.Phase)
		return nil
	}
	snapshotStatusFields := make(map[string]interface{})
	ctrl.logger.Debugf("syncSvcSnapshotByKey: calling updateSnapshotStatusPhase %s/%s", snapshot.Namespace, snapshot.Name)
	_, err = ctrl.updateSnapshotStatusPhase(ctx, snapshot.Namespace, snapshot.Name, newSnapshotStatusPhase, snapshotStatusFields)
	return err
}

// updateSvcSnapshot adds supervisor snapshots that have been updated to given work queue.
func (ctrl *backupDriverController) updateSvcSnapshot(newObj interface{}) {
	ctrl.logger.Debugf("updateSvcSnapshot: New: %+v", newObj)

	// Beware of "xxx deleted" events
	if unknown, ok := newObj.(cache.DeletedFinalStateUnknown); ok && unknown.Obj != nil {
		newObj = unknown.Obj
	}
	svcSnapshotNew, okNew := newObj.(*backupdriverapi.Snapshot)

	if okNew {
		// Skip snapshots not in the svc snapshot map
		// TODO: Add filter to informer to get snapshot owned by this cluster
		if _, ok := ctrl.svcSnapshotMap[svcSnapshotNew.Name]; !ok {
			ctrl.logger.Debugf("The supervisor snapshot %s either does not belong to this cluster, or is still not snapshotted. Skipping updating snapshot status", svcSnapshotNew.Name)
			return
		}
		objName, err := cache.DeletionHandlingMetaNamespaceKeyFunc(svcSnapshotNew)
		if err != nil {
			ctrl.logger.Errorf("failed to get key from object: %v, %v", err, svcSnapshotNew)
			return
		}
		ctrl.logger.Debugf("updateSvcSnapshot: enqueued %q for sync", objName)
		ctrl.svcSnapshotQueue.Add(objName)
	}
}
