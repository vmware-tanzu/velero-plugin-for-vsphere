package snapshotUtils

import (
	"context"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	backupdriverv1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/backupdriver/v1"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/builder"
	v1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned/typed/backupdriver/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/tools/cache"
	"time"
)

type waitDeleteSnapshotResult struct {
	item  interface{}
	err   error
}

func checkDeleteSnapshotPhasesAndSendResult(
	waitForPhases []backupdriverv1.DeleteSnapshotPhase,
	deleteSnapshot *backupdriverv1.DeleteSnapshot,
	results chan waitDeleteSnapshotResult) {
	for _, checkPhase := range waitForPhases {
		if deleteSnapshot.Status.Phase == checkPhase {
			results <- waitDeleteSnapshotResult{
				item:  deleteSnapshot,
				err:   nil,
			}
		}
	}
}

func DeleteSnapshotRef(
	ctx context.Context,
	clientSet *v1.BackupdriverV1Client,
	snapshotID string,
	namespace string,
	repo BackupRepository,
	waitForPhases []backupdriverv1.DeleteSnapshotPhase,
	logger logrus.FieldLogger) (*backupdriverv1.DeleteSnapshot, error) {

	deleteSnapshotUUID, err := uuid.NewRandom()
	if err != nil {
		return nil, errors.Wrapf(err, "Could not create deleteSnapshot UUID")
	}
	deleteSnapshotCR := builder.ForDeleteSnapshot(namespace, deleteSnapshotUUID.String()).
		SnapshotID(snapshotID).
		BackupRepository(repo.backupRepositoryName).
		Result()

	writtenDeleteSnapCR, err := clientSet.DeleteSnapshots(namespace).Create(deleteSnapshotCR)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to create DeleteSnapshot record")
	}
	logger.Infof("DeleteSnapshot record, Name: %s Phase: %s SnapshotID: %s created",
		writtenDeleteSnapCR.Name, writtenDeleteSnapCR.Status.Phase, writtenDeleteSnapCR.Spec.SnapshotID)

	updatedDeleteSnapCR, err := WaitForDeleteSnapshotPhases(ctx, clientSet, *writtenDeleteSnapCR, waitForPhases, namespace, logger)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to wait for expected delete-snapshot phases")
	}
	return updatedDeleteSnapCR, err
}

func WaitForDeleteSnapshotPhases(ctx context.Context,
	clientSet *v1.BackupdriverV1Client,
	deleteSnapshotToWait backupdriverv1.DeleteSnapshot,
	waitForPhases []backupdriverv1.DeleteSnapshotPhase,
	namespace string, logger logrus.FieldLogger) (*backupdriverv1.DeleteSnapshot, error) {
	results := make(chan waitDeleteSnapshotResult)
	watchlist := cache.NewListWatchFromClient(clientSet.RESTClient(), "deleteSnapshots", namespace,
		fields.Everything())
	_, controller := cache.NewInformer(
		watchlist,
		&backupdriverv1.DeleteSnapshot{},
		time.Second*0,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				deleteSnapshot := obj.(*backupdriverv1.DeleteSnapshot)
				if deleteSnapshot.Name != deleteSnapshotToWait.Name {
					return
				}
				logger.Infof("deleteSnapshot added: %v", deleteSnapshot)
				logger.Infof("phase = %s", deleteSnapshot.Status.Phase)
				checkDeleteSnapshotPhasesAndSendResult(waitForPhases, deleteSnapshot, results)
			},
			DeleteFunc: func(obj interface{}) {
				deleteSnapshot := obj.(*backupdriverv1.DeleteSnapshot)
				if deleteSnapshot.Name != deleteSnapshotToWait.Name {
					return
				}
				logger.Infof("deleteSnapshot deleted: %s", obj)
				results <- waitDeleteSnapshotResult{
					item:  nil,
					err:   errors.New("DeleteSnapshot CRD deleted"),
				}
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				deleteSnapshot := newObj.(*backupdriverv1.DeleteSnapshot)
				if deleteSnapshot.Name != deleteSnapshotToWait.Name {
					return
				}
				logger.Infof("deleteSnapshot changed: %v", deleteSnapshot)
				logger.Infof("phase = %s", deleteSnapshot.Status.Phase)
				checkDeleteSnapshotPhasesAndSendResult(waitForPhases, deleteSnapshot, results)

			},
		},
	)
	stop := make(chan struct{})
	go controller.Run(stop)
	select {
	case <-ctx.Done():
		stop <- struct{}{}
		return nil, ctx.Err()
	case result := <-results:
		return result.item.(*backupdriverv1.DeleteSnapshot), result.err
	}
}
