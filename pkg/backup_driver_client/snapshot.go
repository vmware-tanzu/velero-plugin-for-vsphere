package backup_driver_client

import (
	"context"
	v1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/backupdriver/v1"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"time"
)

type BackupDriverClient struct {
	clientset *kubernetes.Clientset
}

func NewBackupDriverClient(clientset *kubernetes.Clientset) (*BackupDriverClient, error) {
	return &BackupDriverClient{
		clientset: clientset,
	}, nil
}

/*
Proxy for the SnapshotStatus CR.  Convenience functions for querying, canceling and waiting for phases
*/
type SnapshotStatus struct {
	backupDriverClient *BackupDriverClient
	snapshotRecord     core_v1.ObjectReference
}

/*
This will wait until one of the specified waitForPhases is reached, the timeout is reached or the context is canceled.

Note: Canceling the context cancels the wait, it does not cancel the snapshot operation.  Use CancelSnapshot to cancel
an in-progress snapshot
*/
func (this *SnapshotStatus) WaitForPhases(ctx context.Context, waitForPhases []v1.SnapshotPhase, timeout time.Duration) (v1.SnapshotPhase, error) {
	return "", nil
}

/*
CancelSnapshot cancels an on-going snapshot operation.
*/
func (this *SnapshotStatus) CancelSnapshot(ctx context.Context) error {
	return nil
}

/*
Returns the current phase and message
error is set if an error occurred while trying to retrieve status, not if the phase is an error phase
*/
func (this *SnapshotStatus) GetCurrentPhase(ctx context.Context) (v1.SnapshotPhase, string, error) {
	return "", "", nil
}

func (this *SnapshotStatus) GetProgress(ctx context.Context) (v1.SnapshotProgress, error) {
	return v1.SnapshotProgress{}, nil
}

/*
Returns the metadata that was present at the time of the snapshot.  Will return nil before SnapshotPhase has gone to

*/
func (this *SnapshotStatus) GetMetadata(ctx context.Context) ([]byte, error) {
	return nil, nil
}

func (this *SnapshotStatus) GetSnapshotID(ctx context.Context) (string, error) {
	return "", nil
}

/*
Creates a Snapshot CR and returns a SnapshotStatus that can be used to monitor/control the snapshot.
The Snapshot CR will be created in the same namespace that the ref exists in
*/
func (this *BackupDriverClient) Snapshot(ctx context.Context, ref core_v1.ObjectReference, backupRepository *BackupRepository) (*SnapshotStatus, error) {
	return nil, nil
}

type CloneFromSnapshotStatus struct {
}

func (this *CloneFromSnapshotStatus) WaitForPhases(ctx context.Context, waitForPhases []v1.ClonePhase, timeout time.Duration) (v1.ClonePhase, error) {
	return "", nil
}

func (this *CloneFromSnapshotStatus) GetProgress(ctx context.Context) (v1.SnapshotProgress, error) {
	return v1.SnapshotProgress{}, nil
}

func (this *CloneFromSnapshotStatus) GetCurrentPhase(ctx context.Context) (v1.SnapshotPhase, core_v1.TypedLocalObjectReference, string, error) {
	return "", core_v1.TypedLocalObjectReference{}, "", nil
}

func (this *CloneFromSnapshotStatus) CancelCloneFromSnapshot(ctx context.Context) error {
	return nil
}

func (this *BackupDriverClient) CloneFromSnapshot(ctx context.Context, snapshotID string, metadata []byte, apiGroup string, kind string,
	backupRepository *BackupRepository) (*CloneFromSnapshotStatus, error) {
	return nil, nil
}
