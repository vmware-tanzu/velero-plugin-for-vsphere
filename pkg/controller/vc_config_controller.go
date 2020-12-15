package controller

import (
	"github.com/sirupsen/logrus"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/constants"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/dataMover"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/snapshotmgr"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/tools/cache"
	"time"
)

type vcConfigController struct {
	*genericController
	dataMover *dataMover.DataMover
	snapMgr   *snapshotmgr.SnapshotManager
}

func NewVcConfigController(
	logger logrus.FieldLogger,
	secretInformer v1.SecretInformer,
	dataMover *dataMover.DataMover,
	snapMgr *snapshotmgr.SnapshotManager,
) Interface {
	v := &vcConfigController{
		genericController: newGenericController("vc-config", logger),
		dataMover:         dataMover,
		snapMgr:           snapMgr,
	}
	v.syncHandler = v.processVcConfigSecretItem
	v.retryHandler = v.exponentialBackoffHandler
	v.cacheSyncWaiters = append(
		v.cacheSyncWaiters,
		secretInformer.Informer().HasSynced,
	)

	secretInformer.Informer().AddEventHandlerWithResyncPeriod(
		cache.FilteringResourceEventHandler{
			FilterFunc: utils.GetVcConfigSecretFilterFunc(logger),
			Handler: cache.ResourceEventHandlerFuncs{
				AddFunc: func(obj interface{}) { v.enqueueVcConfigSecret(obj) },
				UpdateFunc: func(oldObj, newObj interface{}) { v.enqueueVcConfigSecret(newObj) },
			},
		},
		constants.DefaultSecretResyncPeriod)
	return v
}

func (v *vcConfigController) enqueueVcConfigSecret(obj interface{}) {
	// Beware of "xxx deleted" events
	if unknown, ok := obj.(cache.DeletedFinalStateUnknown); ok && unknown.Obj != nil {
		obj = unknown.Obj
	}
	if secretItem, ok := obj.(*corev1.Secret); ok {
		v.logger.Debugf("enqueueSecret on update: %s", secretItem.Name)
		objName, err := cache.DeletionHandlingMetaNamespaceKeyFunc(secretItem)
		if err != nil {
			v.logger.Errorf("failed to get key from object: %v, %v", err, secretItem)
			return
		}
		v.logger.Debugf("enqueueVcConfigSecret: enqueued %q for sync", objName)
		v.enqueue(obj)
	}
}

func (v *vcConfigController) processVcConfigSecretItem(key string) error {
	log := v.logger.WithField("key", key)
	log.Debug("Running processVcConfigSecretItem")

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		log.WithError(err).Error("Failed to split the key of queue item")
		return nil
	}
	// Retrieve the latest Secret.
	ivdParams := make(map[string]interface{})
	err = utils.RetrieveVcConfigSecret(ivdParams, nil, v.logger)
	if err != nil {
		v.logger.Errorf("Failed to retrieve the latest vc config secret")
		return err
	}
	v.logger.Debug("Successfully retrieved latest vSphere VC credentials.")
	if v.dataMover != nil {
		err = v.dataMover.ReloadDataMoverIvdPetmConfig(ivdParams)
		if err != nil {
			v.logger.Errorf("Secret %s/%s Reload on DataMover failed, err: %v", namespace, name, err)
			return err
		}
	}
	if v.snapMgr != nil {
		err = v.snapMgr.ReloadSnapshotManagerIvdPetmConfig(ivdParams)
		if err != nil {
			v.logger.Errorf("Secret %s/%s Reload on Snapshot Manager failed, err: %v", namespace, name, err)
			return err
		}
	}

	v.logger.Debugf("Successfully processed updates in vc configuration.")
	return nil
}

func (v *vcConfigController) exponentialBackoffHandler(key string) error {
	v.logger.Debug("Running exponentialBackoffHandler")
	v.logger.Debugf("Re-adding failed secret processing to the queue")
	v.queue.AddAfter(key, time.Duration(1)*time.Minute)
	return nil
}
