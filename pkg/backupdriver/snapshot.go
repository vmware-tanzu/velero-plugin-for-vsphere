package backupdriver

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	backupdriverv1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/backupdriver/v1"
	v1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned/typed/backupdriver/v1"
	core_v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/tools/cache"
	"time"
)

/*
This will wait until one of the specified waitForPhases is reached, the timeout is reached or the context is canceled.

Note: Canceling the context cancels the wait, it does not cancel the snapshot operation.  Use CancelSnapshot to cancel
an in-progress snapshot
*/

type waitResult struct {
	phase backupdriverv1.SnapshotPhase
	err   error
}

type BackupRepository struct {
	backupRepository string
}

func checkPhasesAndSendResult(waitForPhases []backupdriverv1.SnapshotPhase, snapshot *backupdriverv1.Snapshot,
	results chan waitResult) {
	for _, checkPhase := range waitForPhases {
		if snapshot.Status.Phase == checkPhase {
			results <- waitResult{
				phase: snapshot.Status.Phase,
				err:   nil,
			}
		}
	}
}

func SnapshopRef(ctx context.Context, clientSet *v1.BackupdriverV1Client,
	objectToSnapshot core_v1.TypedLocalObjectReference, namespace string,
	repository BackupRepository,
	waitForPhases []backupdriverv1.SnapshotPhase) (backupdriverv1.Snapshot, error) {

	snapshotUUID, err := uuid.NewRandom()

	if err != nil {
		return backupdriverv1.Snapshot{}, errors.Wrapf(err, "Could not create snapshot UUID")
	}
	snapshotCR :=
		backupdriverv1.Snapshot{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Snapshot",
				APIVersion: "backupdriver.io/v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: snapshotUUID.String(),
			},
			Spec: backupdriverv1.SnapshotSpec{
				TypedLocalObjectReference: objectToSnapshot,
				BackupRepository:          repository.backupRepository,
				SnapshotCancel:            false,
			},
		}

	writtenSnapshot, err := clientSet.Snapshots(namespace).Create(&snapshotCR)
	if err != nil {
		return backupdriverv1.Snapshot{}, errors.Wrapf(err, "Failed to create snapshot record")

	}

	_, err = WaitForPhases(ctx, clientSet, *writtenSnapshot, waitForPhases)
	return *writtenSnapshot, err
}

func WaitForPhases(ctx context.Context, clientSet *v1.BackupdriverV1Client, snapshot backupdriverv1.Snapshot,
	waitForPhases []backupdriverv1.SnapshotPhase) (backupdriverv1.SnapshotPhase, error) {
	results := make(chan waitResult)
	watchlist := cache.NewListWatchFromClient(clientSet.RESTClient(), "snapshots", "backup-driver",
		fields.Everything())
	_, controller := cache.NewInformer(
		watchlist,
		&backupdriverv1.Snapshot{},
		time.Second*0,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				snapshot := obj.(*backupdriverv1.Snapshot)
				fmt.Printf("snapshot added: %v \n", snapshot)
				fmt.Printf("phase = %s\n", snapshot.Status.Phase)
				checkPhasesAndSendResult(waitForPhases, snapshot, results)
			},
			DeleteFunc: func(obj interface{}) {
				fmt.Printf("snapshot deleted: %s \n", obj)
				results <- waitResult{
					phase: "",
					err:   errors.New("Snapshot deleted"),
				}
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				snapshot := newObj.(*backupdriverv1.Snapshot)
				fmt.Printf("snapshot changed: %v \n", snapshot)
				fmt.Printf("phase = %s\n", snapshot.Status.Phase)
				checkPhasesAndSendResult(waitForPhases, snapshot, results)

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
