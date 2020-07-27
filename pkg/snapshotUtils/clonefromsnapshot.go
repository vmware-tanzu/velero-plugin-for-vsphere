package snapshotUtils

import (
	"context"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	backupdriverv1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/backupdriver/v1"
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
	phase backupdriverv1.ClonePhase
	err   error
}

type BackupRepo struct {
	backupRepositoryName string
}

func NewBackupRepo(backupRepositoryName string) *BackupRepo {
	return &BackupRepo{
		backupRepositoryName,
	}
}

func checkClonePhasesAndSendResult(waitForPhases []backupdriverv1.ClonePhase, cloneFromSnapshot *backupdriverv1.CloneFromSnapshot,
	results chan waitCloneResult) {
	for _, checkPhase := range waitForPhases {
		if cloneFromSnapshot.Status.Phase == checkPhase {
			results <- waitCloneResult{
				phase: cloneFromSnapshot.Status.Phase,
				err:   nil,
			}
		}
	}
}

func CloneFromSnapshopRef(ctx context.Context, clientSet *v1.BackupdriverV1Client, snapshotID string, metadata []byte, apiGroup *string, kind string, namespace string, repo BackupRepo, waitForPhases []backupdriverv1.ClonePhase, logger logrus.FieldLogger) (backupdriverv1.CloneFromSnapshot, error) {
	cloneFromSnapshotUUID, err := uuid.NewRandom()

	if err != nil {
		return backupdriverv1.CloneFromSnapshot{}, errors.Wrapf(err, "Could not create cloneFromSnapshot UUID")
	}
	cloneFromSnapshotCR := backupdriverv1.CloneFromSnapshot{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CloneFromSnapshot",
			APIVersion: "backupdriver.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: cloneFromSnapshotUUID.String(),
		},
		Spec: backupdriverv1.CloneFromSnapshotSpec{
			SnapshotID:       snapshotID,
			Metadata:         metadata,
			APIGroup:         apiGroup,
			Kind:             kind,
			BackupRepository: repo.backupRepositoryName,
			CloneCancel:      false,
		},
	}

	writtenClone, err := clientSet.CloneFromSnapshots(namespace).Create(&cloneFromSnapshotCR)
	if err != nil {
		return backupdriverv1.CloneFromSnapshot{}, errors.Wrapf(err, "Failed to create cloneFromSnapshot record")
	}
	logger.Infof("CloneFromSnapshot record, %s, created", writtenClone.Name)

	writtenClone.Status.Phase = backupdriverv1.ClonePhaseNew
	writtenClone, err = clientSet.CloneFromSnapshots(namespace).UpdateStatus(writtenClone)
	if err != nil {
		return backupdriverv1.CloneFromSnapshot{}, errors.Wrapf(err, "Failed to update status of cloneFromSnapshot record")
	}
	logger.Infof("CloneFromSnapshot record, %s, status updated to %s", writtenClone.Name, writtenClone.Status.Phase)

	_, err = WaitForClonePhases(ctx, clientSet, *writtenClone, waitForPhases, namespace, logger)
	return *writtenClone, err
}

func WaitForClonePhases(ctx context.Context, clientSet *v1.BackupdriverV1Client, cloneToWait backupdriverv1.CloneFromSnapshot, waitForPhases []backupdriverv1.ClonePhase, namespace string, logger logrus.FieldLogger) (backupdriverv1.ClonePhase, error) {
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
					phase: "",
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
	go controller.Run(stop)
	select {
	case <-ctx.Done():
		stop <- struct{}{}
		return "", ctx.Err()
	case result := <-results:
		return result.phase, result.err
	}
}
