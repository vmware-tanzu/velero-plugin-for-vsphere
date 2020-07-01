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
	backupdriverinformers "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/informers/externalversions"
	backupdriverlisters "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/listers/backupdriver/v1"
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
}

// NewBackupDriverController returns a BackupDriverController.
func NewBackupDriverController(
	name string,
	logger logrus.FieldLogger,
	// Kubernetes Cluster KubeClient
	kubeClient kubernetes.Interface,
	resyncPeriod time.Duration,
	informerFactory informers.SharedInformerFactory,
	backupdriverInformerFactory backupdriverinformers.SharedInformerFactory,
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
		rateLimiter, "backup-driver")
	snapshotQueue := workqueue.NewNamedRateLimitingQueue(
		rateLimiter, "backup-driver")
	cloneFromSnapshotQueue := workqueue.NewNamedRateLimitingQueue(
		rateLimiter, "backup-driver")

	rc := &backupDriverController{
		name:                           name,
		logger:                         logger.WithField("controller", name),
		kubeClient:                     kubeClient,
		svcKubeClient:                  svcKubeClient,
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
		AddFunc:    rc.addSnapshot,
		UpdateFunc: rc.updateSnapshot,
		DeleteFunc: rc.deleteSnapshot,
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
		//AddFunc:    rc.addBackupRepositoryClaim,
		//UpdateFunc: rc.updateBackupRepositoryClaim,
		//DeleteFunc: rc.deleteBackupRepositoryClaim,
	}, resyncPeriod)

	svcBackupRepositoryClaimInformer.Informer().AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		//AddFunc:    rc.svcAddBackupRepositoryClaim,
		//UpdateFunc: rc.svcUpdateBackupRepositoryClaim,
		//DeleteFunc: rc.svcDeleteBackupRepositoryClaim,
	}, resyncPeriod)

	return rc
}

func (rc *backupDriverController) addSnapshot(obj interface{}) {
	rc.logger.Debugf("addSnapshot: %+v", obj)
	objKey, err := rc.getKey(obj)
	if err != nil {
		return
	}
	rc.logger.Infof("addSnapshot: add %s to Snapshot queue", objKey)
	rc.snapshotQueue.Add(objKey)
}

// getKey helps to get the resource name from resource object
func (rc *backupDriverController) getKey(obj interface{}) (string, error) {
	if unknown, ok := obj.(cache.DeletedFinalStateUnknown); ok && unknown.Obj != nil {
		obj = unknown.Obj
	}
	objKey, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		rc.logger.Errorf("Failed to get key from object: %v", err)
		return "", err
	}
	rc.logger.Infof("getKey: key %s", objKey)
	return objKey, nil
}

func (rc *backupDriverController) updateSnapshot(oldObj, newObj interface{}) {
	rc.logger.Debugf("updateSnapshot: old [%+v] new [%+v]", oldObj, newObj)
	oldSnapshot, ok := oldObj.(*backupdriverapi.Snapshot)
	if !ok || oldSnapshot == nil {
		return
	}
	rc.logger.Debugf("updateSnapshot: old Snapshot %+v", oldSnapshot)

	newSnapshot, ok := newObj.(*backupdriverapi.Snapshot)
	if !ok || newSnapshot == nil {
		return
	}
	rc.logger.Debugf("updateSnapshot: new Snapshot %+v", newSnapshot)

	rc.addSnapshot(newObj)
}

func (rc *backupDriverController) deleteSnapshot(obj interface{}) {
}

// Run starts the controller.
func (rc *backupDriverController) Run(
	ctx context.Context, workers int) {
	defer rc.claimQueue.ShutDown()
	defer rc.snapshotQueue.ShutDown()
	defer rc.cloneFromSnapshotQueue.ShutDown()

	rc.logger.Infof("Starting backup driver controller")
	defer rc.logger.Infof("Shutting down backup driver controller")

	stopCh := ctx.Done()

	rc.logger.Infof("Waiting for caches to sync")
	if !cache.WaitForCacheSync(stopCh, rc.pvSynced, rc.pvcSynced, rc.svcPVCSynced, rc.backupRepositorySynced, rc.backupRepositoryClaimSynced, rc.svcBackupRepositoryClaimSynced, rc.snapshotSynced, rc.svcSnapshotSynced, rc.cloneFromSnapshotSynced, rc.svcCloneFromSnapshotSynced) {
		rc.logger.Errorf("Cannot sync caches")
		return
	}
	rc.logger.Infof("Caches are synced")

	for i := 0; i < workers; i++ {
		//go wait.Until(rc.pvcWorker, 0, stopCh)
		//go wait.Until(rc.pvWorker, 0, stopCh)
		//go wait.Until(rc.svcPvcWorker, 0, stopCh)
		go wait.Until(rc.snapshotWorker, 0, stopCh)
		//go wait.Until(rc.svcSnapshotWorker, 0, stopCh)
		//go wait.Until(rc.cloneFromSnapshotWorker, 0, stopCh)
		//go wait.Until(rc.svcCloneFromSnapshotWorker, 0, stopCh)
		//go wait.Until(rc.backupRepositoryClaimWorker, 0, stopCh)
	}

	<-stopCh
}

// snapshotWorker is the main worker for snapshot request.
func (rc *backupDriverController) snapshotWorker() {
	rc.logger.Debugf("snapshotWorkder: Enter snapshotWorker")

	key, quit := rc.snapshotQueue.Get()
	if quit {
		return
	}
	defer rc.snapshotQueue.Done(key)

	if err := rc.syncSnapshot(key.(string)); err != nil {
		// Put snapshot back to the queue so that we can retry later.
		rc.snapshotQueue.AddRateLimited(key)
	} else {
		rc.snapshotQueue.Forget(key)
	}
}

// syncSnapshot processes one Snapshot CRD
func (rc *backupDriverController) syncSnapshot(key string) error {
	rc.logger.Debugf("syncSnapshot: Started Snapshot processing %s", key)

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		rc.logger.Errorf("Split meta namespace key of snapshot %s failed: %v", key, err)
		return err
	}

	// TODO: Replace _ with snapshot when we start to use it
	_, err = rc.snapshotLister.Snapshots(namespace).Get(name)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			rc.logger.Infof("Snapshot %s/%s is deleted, no need to process it", namespace, name)
			return nil
		}
		rc.logger.Errorf("Get Snapshot %s/%s failed: %v", namespace, name, err)
		return err
	}

	// TODO: Call other function to process create snapshot request

	return nil
}

func (rc *backupDriverController) pvcWorker() {
}

func (rc *backupDriverController) svcPvcWorker() {
}

func (rc *backupDriverController) pvWorker() {
}

func (rc *backupDriverController) svcSnapshotWorker() {
}

func (rc *backupDriverController) cloneFromSnapshotWorker() {
}

func (rc *backupDriverController) svcCloneFromSnapshotWorker() {
}

func (rc *backupDriverController) backupRepositoryClaimWorker() {
}
