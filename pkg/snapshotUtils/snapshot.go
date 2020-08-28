package snapshotUtils

import (
	"context"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"time"

	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/builder"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	backupdriverv1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/backupdriver/v1"
	v1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned/typed/backupdriver/v1"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/tools/cache"
)

/*
This will wait until one of the specified waitForPhases is reached, the timeout is reached or the context is canceled.

Note: Canceling the context cancels the wait, it does not cancel the snapshot operation.  Use CancelSnapshot to cancel
an in-progress snapshot
*/

type waitResult struct {
	item interface{}
	err  error
}

type BackupRepository struct {
	backupRepositoryName string
}

func NewBackupRepository(backupRepositoryName string) *BackupRepository {
	return &BackupRepository{
		backupRepositoryName,
	}
}

func checkPhasesAndSendResult(waitForPhases []backupdriverv1.SnapshotPhase, snapshot *backupdriverv1.Snapshot,
	results chan waitResult) {
	for _, checkPhase := range waitForPhases {
		if snapshot.Status.Phase == checkPhase {
			results <- waitResult{
				item: snapshot,
				err:  nil,
			}
		}
	}
}

// Create a Snapshot record in the specified namespace.
func SnapshotRef(ctx context.Context,
	clientSet *v1.BackupdriverV1Client,
	objectToSnapshot core_v1.TypedLocalObjectReference,
	namespace string,
	repository BackupRepository,
	waitForPhases []backupdriverv1.SnapshotPhase,
	logger logrus.FieldLogger) (*backupdriverv1.Snapshot, error) {

	snapshotUUID, err := uuid.NewRandom()
	if err != nil {
		return nil, errors.Wrapf(err, "Could not create snapshot UUID")
	}
	snapshotName := "snap-" + snapshotUUID.String()
	snapshotReq := builder.ForSnapshot(namespace, snapshotName).
		BackupRepository(repository.backupRepositoryName).
		ObjectReference(objectToSnapshot).
		CancelState(false).Result()

	writtenSnapshot, err := clientSet.Snapshots(namespace).Create(context.TODO(), snapshotReq, metav1.CreateOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to create snapshot record")
	}
	logger.Infof("Snapshot record, %s, created", writtenSnapshot.Name)

	writtenSnapshot.Status.Phase = backupdriverv1.SnapshotPhaseNew
	writtenSnapshot, err = clientSet.Snapshots(namespace).UpdateStatus(context.TODO(), writtenSnapshot, metav1.UpdateOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to update status of snapshot record")
	}
	logger.Infof("Snapshot record, %s, status updated to %s", writtenSnapshot.Name, writtenSnapshot.Status.Phase)

	updatedSnapshot, err := WaitForPhases(ctx, clientSet, *writtenSnapshot, waitForPhases, namespace, logger)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to wait for expected snapshot phases")
	}

	return updatedSnapshot, err
}

func WaitForPhases(ctx context.Context, clientSet *v1.BackupdriverV1Client, snapshotToWait backupdriverv1.Snapshot, waitForPhases []backupdriverv1.SnapshotPhase, namespace string, logger logrus.FieldLogger) (*backupdriverv1.Snapshot, error) {
	results := make(chan waitResult)
	watchlist := cache.NewListWatchFromClient(clientSet.RESTClient(), "snapshots", namespace,
		fields.Everything())
	_, controller := cache.NewInformer(
		watchlist,
		&backupdriverv1.Snapshot{},
		time.Second*0,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				snapshot := obj.(*backupdriverv1.Snapshot)
				if snapshot.Name != snapshotToWait.Name {
					return
				}
				logger.Infof("snapshot added: %v", snapshot)
				logger.Infof("phase = %s", snapshot.Status.Phase)
				checkPhasesAndSendResult(waitForPhases, snapshot, results)
			},
			DeleteFunc: func(obj interface{}) {
				snapshot := obj.(*backupdriverv1.Snapshot)
				if snapshot.Name != snapshotToWait.Name {
					return
				}
				logger.Infof("snapshot deleted: %s", obj)
				results <- waitResult{
					item: nil,
					err:  errors.New("Snapshot deleted"),
				}
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				snapshot := newObj.(*backupdriverv1.Snapshot)
				if snapshot.Name != snapshotToWait.Name {
					return
				}
				logger.Infof("snapshot= %s changed, phase = %s", snapshot.Name, snapshot.Status.Phase)
				checkPhasesAndSendResult(waitForPhases, snapshot, results)

			},
		},
	)
	stop := make(chan struct{})
	defer close(stop)

	go controller.Run(stop)
	select {
	case <-ctx.Done():
		stop <- struct{}{}
		return nil, ctx.Err()
	case result := <-results:
		return result.item.(*backupdriverv1.Snapshot), result.err
	}
}
