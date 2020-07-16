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
	"time"

	"github.com/sirupsen/logrus"
	backupdriverapi "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/backupdriver/v1"
	backupdriverclientset "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned/typed/backupdriver/v1"
	backupdriverinformers "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/informers/externalversions"
	backupdriverlisters "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/listers/backupdriver/v1"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/snapshotmgr"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
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
	// KubeClient of the current cluster
	kubeClient kubernetes.Interface
	// Supervisor Cluster KubeClient from Guest Cluster
	svcKubeClient kubernetes.Interface

	backupdriverClient *backupdriverclientset.BackupdriverV1Client

	// Supervisor Cluster namespace
	supervisorNamespace string

	rateLimiter workqueue.RateLimiter

	// PV Lister
	pvLister corelisters.PersistentVolumeLister
	// PV Synced
	pvSynced cache.InformerSynced

	// PVC Lister
	pvcLister corelisters.PersistentVolumeClaimLister
	// PVC Synced
	pvcSynced cache.InformerSynced
	// claim queue
	claimQueue workqueue.RateLimitingInterface

	// Supervisor Cluster PVC Lister
	svcPVCLister corelisters.PersistentVolumeClaimLister
	// Supervisor Cluster PVC Synced
	svcPVCSynced cache.InformerSynced

	// BackupRepositoryClaim Lister
	backupRepositoryClaimLister backupdriverlisters.BackupRepositoryClaimLister
	// BackupRepositoryClaim Synced
	backupRepositoryClaimSynced cache.InformerSynced
	// backupRepositoryClaim queue
	backupRepositoryClaimQueue workqueue.RateLimitingInterface

	// Supervisor Cluster BackupRepositoryClaim Lister
	svcBackupRepositoryClaimLister backupdriverlisters.BackupRepositoryClaimLister
	// Supervisor Cluster BackupRepositoryClaim Synced
	svcBackupRepositoryClaimSynced cache.InformerSynced

	// BackupRepository Lister
	backupRepositoryLister backupdriverlisters.BackupRepositoryLister
	// BackupRepository Synced
	backupRepositorySynced cache.InformerSynced

	// Snapshot queue
	snapshotQueue workqueue.RateLimitingInterface
	// Snapshot Lister
	snapshotLister backupdriverlisters.SnapshotLister
	// Snapshot Synced
	snapshotSynced cache.InformerSynced

	// Supervisor Cluster Snapshot Lister
	svcSnapshotLister backupdriverlisters.SnapshotLister
	// Supervisor Cluster Snapshot Synced
	svcSnapshotSynced cache.InformerSynced

	// CloneFromSnapshot queue
	cloneFromSnapshotQueue workqueue.RateLimitingInterface
	// CloneFromSnapshot Lister
	cloneFromSnapshotLister backupdriverlisters.CloneFromSnapshotLister
	// CloneFromSnapshot Synced
	cloneFromSnapshotSynced cache.InformerSynced

	// Supervisor Cluster CloneFromSnapshot Lister
	svcCloneFromSnapshotLister backupdriverlisters.CloneFromSnapshotLister
	// Supervisor Cluster CloneFromSnapshot Synced
	svcCloneFromSnapshotSynced cache.InformerSynced

	// Snapshot manager
	snapManager *snapshotmgr.SnapshotManager
}

// NewBackupDriverController returns a BackupDriverController.
func NewBackupDriverController(
	name string,
	logger logrus.FieldLogger,
	// Kubernetes Cluster KubeClient
	kubeClient kubernetes.Interface,
	backupdriverClient *backupdriverclientset.BackupdriverV1Client,
	resyncPeriod time.Duration,
	informerFactory informers.SharedInformerFactory,
	backupdriverInformerFactory backupdriverinformers.SharedInformerFactory,
	snapManager *snapshotmgr.SnapshotManager,
	rateLimiter workqueue.RateLimiter) BackupDriverController {
	var supervisorNamespace string
	// Supervisor Cluster KubeClient
	var svcKubeClient kubernetes.Interface
	// TODO: Fix svcPVCInformer to use svcInformerFactory
	svcPVCInformer := informerFactory.Core().V1().PersistentVolumeClaims()
	//svcPVCInformer := svcInformerFactory.Core().V1().PersistentVolumeClaims()
	pvcInformer := informerFactory.Core().V1().PersistentVolumeClaims()
	pvInformer := informerFactory.Core().V1().PersistentVolumes()

	backupRepositoryInformer := backupdriverInformerFactory.Backupdriver().V1().BackupRepositories()

	backupRepositoryClaimInformer := backupdriverInformerFactory.Backupdriver().V1().BackupRepositoryClaims()
	// TODO: Use svcBackupdriverInformerFactory for svcBackupRepositoryClaimInformer
	svcBackupRepositoryClaimInformer := backupdriverInformerFactory.Backupdriver().V1().BackupRepositoryClaims()

	snapshotInformer := backupdriverInformerFactory.Backupdriver().V1().Snapshots()
	// TODO: Use svcBackupdriverInformerFactory for svcSnapshotInformer
	svcSnapshotInformer := backupdriverInformerFactory.Backupdriver().V1().Snapshots()

	cloneFromSnapshotInformer := backupdriverInformerFactory.Backupdriver().V1().CloneFromSnapshots()
	// TODO: Use svcBackupdriverInformerFactor for svcCloneFromSnapshotInformer
	svcCloneFromSnapshotInformer := backupdriverInformerFactory.Backupdriver().V1().CloneFromSnapshots()

	claimQueue := workqueue.NewNamedRateLimitingQueue(
		rateLimiter, "backup-driver-claim-queue")
	snapshotQueue := workqueue.NewNamedRateLimitingQueue(
		rateLimiter, "backup-driver-snapshot-queue")
	cloneFromSnapshotQueue := workqueue.NewNamedRateLimitingQueue(
		rateLimiter, "backup-driver-clone-queue")
	backupRepositoryClaimQueue := workqueue.NewNamedRateLimitingQueue(
		rateLimiter, "backup-driver-brc-queue")

	ctrl := &backupDriverController{
		name:                           name,
		logger:                         logger.WithField("controller", name),
		kubeClient:                     kubeClient,
		svcKubeClient:                  svcKubeClient,
		backupdriverClient:             backupdriverClient,
		snapManager:                    snapManager,
		pvLister:                       pvInformer.Lister(),
		pvSynced:                       pvInformer.Informer().HasSynced,
		pvcLister:                      pvcInformer.Lister(),
		pvcSynced:                      pvcInformer.Informer().HasSynced,
		claimQueue:                     claimQueue,
		svcPVCLister:                   svcPVCInformer.Lister(),
		svcPVCSynced:                   svcPVCInformer.Informer().HasSynced,
		supervisorNamespace:            supervisorNamespace,
		snapshotLister:                 snapshotInformer.Lister(),
		snapshotSynced:                 snapshotInformer.Informer().HasSynced,
		snapshotQueue:                  snapshotQueue,
		svcSnapshotLister:              svcSnapshotInformer.Lister(),
		svcSnapshotSynced:              svcSnapshotInformer.Informer().HasSynced,
		cloneFromSnapshotLister:        cloneFromSnapshotInformer.Lister(),
		cloneFromSnapshotSynced:        cloneFromSnapshotInformer.Informer().HasSynced,
		cloneFromSnapshotQueue:         cloneFromSnapshotQueue,
		svcCloneFromSnapshotLister:     svcCloneFromSnapshotInformer.Lister(),
		svcCloneFromSnapshotSynced:     svcCloneFromSnapshotInformer.Informer().HasSynced,
		backupRepositoryLister:         backupRepositoryInformer.Lister(),
		backupRepositorySynced:         backupRepositoryInformer.Informer().HasSynced,
		backupRepositoryClaimLister:    backupRepositoryClaimInformer.Lister(),
		backupRepositoryClaimSynced:    backupRepositoryClaimInformer.Informer().HasSynced,
		backupRepositoryClaimQueue:     backupRepositoryClaimQueue,
		svcBackupRepositoryClaimLister: svcBackupRepositoryClaimInformer.Lister(),
		svcBackupRepositoryClaimSynced: svcBackupRepositoryClaimInformer.Informer().HasSynced,
	}

	pvcInformer.Informer().AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		//AddFunc:    rc.addPVC,
		//UpdateFunc: rc.updatePVC,
		//DeleteFunc: rc.deletePVC,
	}, resyncPeriod)

	svcPVCInformer.Informer().AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		//AddFunc:    rc.svcAddPVC,
		//UpdateFunc: rc.svcUpdatePVC,
		//DeleteFunc: rc.svcDeletePVC,
	}, resyncPeriod)

	pvInformer.Informer().AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		//AddFunc:    rc.addPV,
		//UpdateFunc: rc.updatePV,
		//DeleteFunc: rc.deletePV,
	}, resyncPeriod)

	snapshotInformer.Informer().AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    ctrl.addSnapshot,
		UpdateFunc: ctrl.updateSnapshot,
		DeleteFunc: ctrl.deleteSnapshot,
	}, resyncPeriod)

	svcSnapshotInformer.Informer().AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		//AddFunc:    rc.svcAddSnapshot,
		//UpdateFunc: rc.svcUpdateSnapshot,
		//DeleteFunc: rc.svcDeleteSnapshot,
	}, resyncPeriod)

	cloneFromSnapshotInformer.Informer().AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		//AddFunc:    rc.addCloneFromSnapshot,
		//UpdateFunc: rc.updateCloneFromSnapshot,
		//DeleteFunc: rc.deleteCloneFromSnapshot,
	}, resyncPeriod)

	svcCloneFromSnapshotInformer.Informer().AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		//AddFunc:    rc.svcAddCloneFromSnapshot,
		//UpdateFunc: rc.svcUpdateCloneFromSnapshot,
		//DeleteFunc: rc.svcDeleteCloneFromSnapshot,
	}, resyncPeriod)

	backupRepositoryInformer.Informer().AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		//AddFunc:    rc.addBackupRepository,
		//UpdateFunc: rc.updateBackupRepository,
		//DeleteFunc: rc.deleteBackupRepository,
	}, resyncPeriod)

	backupRepositoryClaimInformer.Informer().AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    ctrl.addBackupRepositoryClaim,
		UpdateFunc: ctrl.updateBackupRepositoryClaim,
		DeleteFunc: ctrl.deleteBackupRepositoryClaim,
	}, resyncPeriod)

	svcBackupRepositoryClaimInformer.Informer().AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		//AddFunc:    rc.svcAddBackupRepositoryClaim,
		//UpdateFunc: rc.svcUpdateBackupRepositoryClaim,
		//DeleteFunc: rc.svcDeleteBackupRepositoryClaim,
	}, resyncPeriod)

	return ctrl
}

func (ctrl *backupDriverController) addSnapshot(obj interface{}) {
	ctrl.logger.Debugf("addSnapshot: %+v", obj)
	objKey, err := ctrl.getKey(obj)
	if err != nil {
		return
	}
	ctrl.logger.Infof("addSnapshot: add %s to Snapshot queue", objKey)
	ctrl.snapshotQueue.Add(objKey)
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

func (ctrl *backupDriverController) updateSnapshot(oldObj, newObj interface{}) {
	ctrl.logger.Debugf("updateSnapshot: old [%+v] new [%+v]", oldObj, newObj)
	oldSnapshot, ok := oldObj.(*backupdriverapi.Snapshot)
	if !ok || oldSnapshot == nil {
		return
	}
	ctrl.logger.Debugf("updateSnapshot: old Snapshot %+v", oldSnapshot)

	newSnapshot, ok := newObj.(*backupdriverapi.Snapshot)
	if !ok || newSnapshot == nil {
		return
	}
	ctrl.logger.Debugf("updateSnapshot: new Snapshot %+v", newSnapshot)

	ctrl.addSnapshot(newObj)
}

func (ctrl *backupDriverController) deleteSnapshot(obj interface{}) {
}

// Run starts the controller.
func (ctrl *backupDriverController) Run(
	ctx context.Context, workers int) {
	defer ctrl.claimQueue.ShutDown()
	defer ctrl.snapshotQueue.ShutDown()
	defer ctrl.cloneFromSnapshotQueue.ShutDown()

	ctrl.logger.Infof("Starting backup driver controller")
	defer ctrl.logger.Infof("Shutting down backup driver controller")

	stopCh := ctx.Done()

	ctrl.logger.Infof("Waiting for caches to sync")
	if !cache.WaitForCacheSync(stopCh, ctrl.pvSynced, ctrl.pvcSynced, ctrl.svcPVCSynced, ctrl.backupRepositorySynced, ctrl.backupRepositoryClaimSynced, ctrl.svcBackupRepositoryClaimSynced, ctrl.snapshotSynced, ctrl.svcSnapshotSynced, ctrl.cloneFromSnapshotSynced, ctrl.svcCloneFromSnapshotSynced) {
		ctrl.logger.Errorf("Cannot sync caches")
		return
	}
	ctrl.logger.Infof("Caches are synced")

	for i := 0; i < workers; i++ {
		//go wait.Until(ctrl.pvcWorker, 0, stopCh)
		//go wait.Until(ctrl.pvWorker, 0, stopCh)
		//go wait.Until(ctrl.svcPvcWorker, 0, stopCh)
		go wait.Until(ctrl.snapshotWorker, 0, stopCh)
		//go wait.Until(ctrl.svcSnapshotWorker, 0, stopCh)
		//go wait.Until(ctrl.cloneFromSnapshotWorker, 0, stopCh)
		//go wait.Until(ctrl.svcCloneFromSnapshotWorker, 0, stopCh)
		go wait.Until(ctrl.backupRepositoryClaimWorker, 0, stopCh)
	}

	<-stopCh
}

// snapshotWorker is the main worker for snapshot request.
func (ctrl *backupDriverController) snapshotWorker() {
	ctrl.logger.Infof("snapshotWorker: Enter snapshotWorker")

	key, quit := ctrl.snapshotQueue.Get()
	if quit {
		return
	}
	defer ctrl.snapshotQueue.Done(key)

	if err := ctrl.syncSnapshot(key.(string)); err != nil {
		// Put snapshot back to the queue so that we can retry later.
		ctrl.snapshotQueue.AddRateLimited(key)
	} else {
		ctrl.snapshotQueue.Forget(key)
	}
}

// syncSnapshot processes one Snapshot CRD
func (ctrl *backupDriverController) syncSnapshot(key string) error {
	ctrl.logger.Debugf("syncSnapshot: Started Snapshot processing %s", key)

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		ctrl.logger.Errorf("Split meta namespace key of snapshot %s failed: %v", key, err)
		return err
	}

	snapshot, err := ctrl.snapshotLister.Snapshots(namespace).Get(name)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			ctrl.logger.Infof("Snapshot %s/%s is deleted, no need to process it", namespace, name)
			return nil
		}
		ctrl.logger.Errorf("Get Snapshot %s/%s failed: %v", namespace, name, err)
		return err
	}

	err = ctrl.CreateSnapshot(snapshot)
	if err != nil {
		ctrl.logger.Errorf("Create snapshot %s/%s failed: %v", namespace, name, err)
		return err
	}

	return nil
}

func (ctrl *backupDriverController) pvcWorker() {
}

func (ctrl *backupDriverController) svcPvcWorker() {
}

func (ctrl *backupDriverController) pvWorker() {
}

func (ctrl *backupDriverController) svcSnapshotWorker() {
}

func (ctrl *backupDriverController) cloneFromSnapshotWorker() {
}

func (ctrl *backupDriverController) svcCloneFromSnapshotWorker() {
}

func (ctrl *backupDriverController) backupRepositoryClaimWorker() {
	ctrl.logger.Debugf("backupRepositoryClaim: Enter backupRepositoryClaimWorker")

	key, quit := ctrl.backupRepositoryClaimQueue.Get()
	if quit {
		return
	}
	defer ctrl.backupRepositoryClaimQueue.Done(key)

	if err := ctrl.syncBackupRepositoryClaim(key.(string)); err != nil {
		// Put backuprepositoryclaim back to the queue so that we can retry later.
		ctrl.backupRepositoryClaimQueue.AddRateLimited(key)
	} else {
		ctrl.backupRepositoryClaimQueue.Forget(key)
	}

}

// syncBackupRepositoryClaim processes one BackupRepositoryClaim CRD
func (ctrl *backupDriverController) syncBackupRepositoryClaim(key string) error {
	ctrl.logger.Infof("syncBackupRepositoryClaim: Started BackupRepositoryClaim processing %s", key)

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		ctrl.logger.Errorf("Split meta namespace key of backupRepositoryClaim %s failed: %v", key, err)
		return err
	}

	brc, err := ctrl.backupRepositoryClaimLister.BackupRepositoryClaims(namespace).Get(name)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			ctrl.logger.Infof("BackupRepositoryClaim %s/%s is deleted, no need to process it", namespace, name)
			return nil
		}
		ctrl.logger.Errorf("Get BackupRepositoryClaim %s/%s failed: %v", namespace, name, err)
		return err
	}

	ctx := context.Background()
	// Create BackupRepository when a new BackupRepositoryClaim is added
	br, err := CreateBackupRepository(ctx, brc, ctrl.backupdriverClient, ctrl.logger)
	if err != nil {
		ctrl.logger.Errorf("Failed to create BackupRepository")
		return err
	}

	err = PatchBackupRepositoryClaim(brc, br.Name, brc.Namespace, ctrl.backupdriverClient)
	if err != nil {
		return err
	}

	return nil
}

func (ctrl *backupDriverController) addBackupRepositoryClaim(obj interface{}) {
	ctrl.logger.Debugf("addBackupRepositoryClaim: %+v", obj)
	objKey, err := ctrl.getKey(obj)
	if err != nil {
		return
	}
	ctrl.logger.Infof("addBackupRepositoryClaim: add %s to BackupRepositoryClaim queue", objKey)
	ctrl.backupRepositoryClaimQueue.Add(objKey)
}

func (ctrl *backupDriverController) updateBackupRepositoryClaim(oldObj, newObj interface{}) {
	ctrl.logger.Debugf("updateBackupRepositoryClaim: old [%+v] new [%+v]", oldObj, newObj)
	oldBackupRepositoryClaim, ok := oldObj.(*backupdriverapi.BackupRepositoryClaim)
	if !ok || oldBackupRepositoryClaim == nil {
		return
	}

	newBackupRepositoryClaim, ok := newObj.(*backupdriverapi.BackupRepositoryClaim)
	if !ok || newBackupRepositoryClaim == nil {
		return
	}
	ctrl.logger.Debugf("updateBackupRepositoryClaim: new BackupRepositoryClaim %+v", newBackupRepositoryClaim)

	ctrl.addBackupRepositoryClaim(newObj)
}

func (ctrl *backupDriverController) deleteBackupRepositoryClaim(obj interface{}) {
}
