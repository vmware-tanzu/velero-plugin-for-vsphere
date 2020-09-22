package snapshotUtils

import (
	"context"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	backupdriverv1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/backupdriver/v1"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/builder"
	v1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned/typed/backupdriver/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/tools/cache"
	"time"
)

/*
This will wait until one of the specified waitForPhases is reached, the timeout is reached or the context is canceled.

Note: Canceling the context cancels the wait, it does not cancel the cloneFromSnapshot operation.  Use CancelClone to cancel an in-progress cloneFromSnapshot
*/

type waitCloneResult struct {
	item  interface{}
	err   error
}

func checkClonePhasesAndSendResult(waitForPhases []backupdriverv1.ClonePhase, cloneFromSnapshot *backupdriverv1.CloneFromSnapshot,
	results chan waitCloneResult) {
	for _, checkPhase := range waitForPhases {
		if cloneFromSnapshot.Status.Phase == checkPhase {
			results <- waitCloneResult{
				item:  cloneFromSnapshot,
				err:   nil,
			}
		}
	}
}

func CloneFromSnapshopRef(ctx context.Context,
	clientSet *v1.BackupdriverV1Client,
	snapshotID string, metadata []byte,
	apiGroup *string, kind string,
	namespace string,
	repo BackupRepository,
	waitForPhases []backupdriverv1.ClonePhase,
	logger logrus.FieldLogger) (*backupdriverv1.CloneFromSnapshot, error) {

	cloneFromSnapshotUUID, err := uuid.NewRandom()
	if err != nil {
		return nil, errors.Wrapf(err, "Could not create cloneFromSnapshot UUID")
	}

	cloneFromSnapshotCR := builder.ForCloneFromSnapshot(namespace, cloneFromSnapshotUUID.String(), nil).SnapshotID(snapshotID).
		Metadata(metadata).APIGroup(apiGroup).Kind(kind).BackupRepository(repo.backupRepositoryName).Result()

	writtenClone, err := clientSet.CloneFromSnapshots(namespace).Create(context.TODO(), cloneFromSnapshotCR, metav1.CreateOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to create cloneFromSnapshot record")
	}
	logger.Infof("CloneFromSnapshot record, %s, created", writtenClone.Name)

	writtenClone.Status.Phase = backupdriverv1.ClonePhaseNew
	writtenClone, err = clientSet.CloneFromSnapshots(namespace).UpdateStatus(context.TODO(), writtenClone, metav1.UpdateOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to update status of cloneFromSnapshot record")
	}
	logger.Infof("CloneFromSnapshot record, %s, status updated to %s", writtenClone.Name, writtenClone.Status.Phase)

	updatedClone, err := WaitForClonePhases(ctx, clientSet, *writtenClone, waitForPhases, namespace, logger)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to wait for expected clone phases")
	}

	return updatedClone, err
}

func WaitForClonePhases(ctx context.Context, clientSet *v1.BackupdriverV1Client, cloneToWait backupdriverv1.CloneFromSnapshot, waitForPhases []backupdriverv1.ClonePhase, namespace string, logger logrus.FieldLogger) (*backupdriverv1.CloneFromSnapshot, error) {
	results := make(chan waitCloneResult)
	watchlist := cache.NewListWatchFromClient(clientSet.RESTClient(), "cloneFromSnapshots", namespace,
		fields.Everything())
	_, controller := cache.NewInformer(
		watchlist,
		&backupdriverv1.CloneFromSnapshot{},
		time.Second*0,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				cloneFromSnapshot := obj.(*backupdriverv1.CloneFromSnapshot)
				if cloneFromSnapshot.Name != cloneToWait.Name {
					return
				}
				logger.Infof("cloneFromSnapshot added: %v", cloneFromSnapshot)
				logger.Infof("phase = %s", cloneFromSnapshot.Status.Phase)
				checkClonePhasesAndSendResult(waitForPhases, cloneFromSnapshot, results)
			},
			DeleteFunc: func(obj interface{}) {
				cloneFromSnapshot := obj.(*backupdriverv1.CloneFromSnapshot)
				if cloneFromSnapshot.Name != cloneToWait.Name {
					return
				}
				logger.Infof("cloneFromSnapshot deleted: %s", obj)
				results <- waitCloneResult{
					item:  nil,
					err:   errors.New("CloneFromSnapshot deleted"),
				}
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				cloneFromSnapshot := newObj.(*backupdriverv1.CloneFromSnapshot)
				if cloneFromSnapshot.Name != cloneToWait.Name {
					return
				}
				logger.Infof("cloneFromSnapshot changed: %v", cloneFromSnapshot)
				logger.Infof("phase = %s", cloneFromSnapshot.Status.Phase)
				checkClonePhasesAndSendResult(waitForPhases, cloneFromSnapshot, results)

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
		return result.item.(*backupdriverv1.CloneFromSnapshot), result.err
	}
}
