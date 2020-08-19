package backupdriver

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/builder"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/utils"
	"testing"
	backupdriverapi "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/backupdriver/v1"
	informers "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/informers/externalversions"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned/fake"
	veleroplugintest "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/test"
)

func defaultSnapshot() *builder.SnapshotBuilder {
	return builder.ForSnapshot(utils.DefaultNamespace, "snapshot-test-1")
}

func defaultCloneFromSnapshot() *builder.CloneFromSnapshotBuilder {
	return builder.ForCloneFromSnapshot(utils.DefaultNamespace, "clonefromsnapshot-test-1")
}

func defaultDeleteSnapshot() *builder.DeleteSnapshotBuilder {
	return builder.ForDeleteSnapshot(utils.DefaultNamespace, "delete-snapshot-test-1")
}
func TestSyncSnapshotByKeySkipCase(t *testing.T) {
	tests := []struct {
		name string
		key string
		snapshot *backupdriverapi.Snapshot
	}{
		{
			name: "If snapshot phase is SnapshotPhaseInProgress, should skip createSnapshot",
			key: "velero/snapshot-test-1",
			snapshot: defaultSnapshot().Phase(backupdriverapi.SnapshotPhaseInProgress).Result(),

		},
		{
			name: "If snapshot phase is SnapshotPhaseSnapshotted, should skip createSnapshot",
			key: "velero/snapshot-test-1",
			snapshot: defaultSnapshot().Phase(backupdriverapi.SnapshotPhaseSnapshotted).Result(),

		},
		{
			name: "If snapshot phase is SnapshotPhaseSnapshotFailed, should skip createSnapshot",
			key: "velero/snapshot-test-1",
			snapshot: defaultSnapshot().Phase(backupdriverapi.SnapshotPhaseSnapshotFailed).Result(),

		},
		{
			name: "If snapshot phase is SnapshotPhaseUploading, should skip createSnapshot",
			key: "velero/snapshot-test-1",
			snapshot: defaultSnapshot().Phase(backupdriverapi.SnapshotPhaseUploading).Result(),

		},
		{
			name: "If snapshot phase is SnapshotPhaseUploaded, should skip createSnapshot",
			key: "velero/snapshot-test-1",
			snapshot: defaultSnapshot().Phase(backupdriverapi.SnapshotPhaseUploaded).Result(),

		},
		{
			name: "If snapshot phase is SnapshotPhaseUploadFailed, should skip createSnapshot",
			key: "velero/snapshot-test-1",
			snapshot: defaultSnapshot().Phase(backupdriverapi.SnapshotPhaseUploadFailed).Result(),

		},
		{
			name: "If snapshot phase is SnapshotPhaseCanceling, should skip createSnapshot",
			key: "velero/snapshot-test-1",
			snapshot: defaultSnapshot().Phase(backupdriverapi.SnapshotPhaseCanceling).Result(),

		},
		{
			name: "If snapshot phase is SnapshotPhaseCanceled, should skip createSnapshot",
			key: "velero/snapshot-test-1",
			snapshot: defaultSnapshot().Phase(backupdriverapi.SnapshotPhaseCanceled).Result(),

		},
		{
			name: "If snapshot phase is SnapshotPhaseCleanupFailed, should skip createSnapshot",
			key: "velero/snapshot-test-1",
			snapshot: defaultSnapshot().Phase(backupdriverapi.SnapshotPhaseCleanupFailed).Result(),

		},
		{
			name: "If snapshot is set to be canceled, should skip createSnapshot",
			key: "velero/snapshot-test-1",
			snapshot: defaultSnapshot().Phase(backupdriverapi.SnapshotPhaseNew).CancelState(true).Result(),

		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var (
				clientset       = fake.NewSimpleClientset(test.snapshot)
				sharedInformers = informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
				logger          = veleroplugintest.NewLogger()
			)
			c := &backupDriverController{
				logger: logger,
				snapshotLister: sharedInformers.Backupdriver().V1().Snapshots().Lister(),
				backupdriverClient: clientset.BackupdriverV1(),
			}
			require.NoError(t, sharedInformers.Backupdriver().V1().Snapshots().Informer().GetStore().Add(test.snapshot))
			err := c.syncSnapshotByKey(test.key)
			assert.Nil(t, err)
		})
	}
}

func TestSyncCloneFromSnapshotByKeySkipCase(t *testing.T) {
	tests := []struct {
		name string
		key string
		cloneFromSnapshot *backupdriverapi.CloneFromSnapshot
	}{
		{
			name: "If clonefromsnapshot phase is ClonePhaseInProgress, should skip",
			key: "velero/cloneFromSnapshot-test-1",
			cloneFromSnapshot: defaultCloneFromSnapshot().Phase(backupdriverapi.ClonePhaseInProgress).Result(),
		},
		{
			name: "If clonefromsnapshot phase is ClonePhaseCompleted, should skip",
			key: "velero/cloneFromSnapshot-test-1",
			cloneFromSnapshot: defaultCloneFromSnapshot().Phase(backupdriverapi.ClonePhaseCompleted).Result(),
		},
		{
			name: "If clonefromsnapshot phase is ClonePhaseFailed, should skip",
			key: "velero/cloneFromSnapshot-test-1",
			cloneFromSnapshot: defaultCloneFromSnapshot().Phase(backupdriverapi.ClonePhaseFailed).Result(),
		},
		{
			name: "If clonefromsnapshot phase is ClonePhaseCanceling, should skip",
			key: "velero/cloneFromSnapshot-test-1",
			cloneFromSnapshot: defaultCloneFromSnapshot().Phase(backupdriverapi.ClonePhaseCanceling).Result(),
		},
		{
			name: "If clonefromsnapshot phase is ClonePhaseCanceled, should skip",
			key: "velero/cloneFromSnapshot-test-1",
			cloneFromSnapshot: defaultCloneFromSnapshot().Phase(backupdriverapi.ClonePhaseCanceled).Result(),
		},
		{
			name: "If clonefromsnapshot is set to be canceled, should skip",
			key: "velero/snapshot-test-1",
			cloneFromSnapshot: defaultCloneFromSnapshot().Phase(backupdriverapi.ClonePhaseNew).CancelState(true).Result(),

		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var (
				clientset       = fake.NewSimpleClientset(test.cloneFromSnapshot)
				sharedInformers = informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
				logger          = veleroplugintest.NewLogger()
			)
			c := &backupDriverController{
				logger: logger,
				snapshotLister: sharedInformers.Backupdriver().V1().Snapshots().Lister(),
				backupdriverClient: clientset.BackupdriverV1(),
			}
			require.NoError(t, sharedInformers.Backupdriver().V1().CloneFromSnapshots().Informer().GetStore().Add(test.cloneFromSnapshot))
			err := c.syncSnapshotByKey(test.key)
			assert.Nil(t, err)
		})
	}
}

func TestSyncDeleteSnapshotByKeySkipCase(t *testing.T) {
	tests := []struct {
		name string
		key string
		deleteSnapshot *backupdriverapi.DeleteSnapshot
	}{
		{
			name: "If deletesnapshot phase is DeleteSnapshotPhaseInProgress, should skip",
			key: "velero/cloneFromSnapshot-test-1",
			deleteSnapshot: defaultDeleteSnapshot().Phase(backupdriverapi.DeleteSnapshotPhaseInProgress).Result(),
		},
		{
			name: "If deletesnapshot phase is DeleteSnapshotPhaseCompleted, should skip",
			key: "velero/deleteSnapshot-test-1",
			deleteSnapshot: defaultDeleteSnapshot().Phase(backupdriverapi.DeleteSnapshotPhaseCompleted).Result(),
		},
		{
			name: "If deletesnapshot phase is DeleteSnapshotPhaseFailed, should skip",
			key: "velero/deleteSnapshot-test-1",
			deleteSnapshot: defaultDeleteSnapshot().Phase(backupdriverapi.DeleteSnapshotPhaseFailed).Result(),
		},
		{
			name: "If deletesnapshot deletion timestamp is set, should skip",
			key: "velero/snapshot-test-1",
			deleteSnapshot: defaultDeleteSnapshot().Phase(backupdriverapi.DeleteSnapshotPhaseNew).DeleteTimestamp().Result(),

		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var (
				clientset       = fake.NewSimpleClientset(test.deleteSnapshot)
				sharedInformers = informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
				logger          = veleroplugintest.NewLogger()
			)
			c := &backupDriverController{
				logger: logger,
				snapshotLister: sharedInformers.Backupdriver().V1().Snapshots().Lister(),
				backupdriverClient: clientset.BackupdriverV1(),
			}
			require.NoError(t, sharedInformers.Backupdriver().V1().DeleteSnapshots().Informer().GetStore().Add(test.deleteSnapshot))
			err := c.syncSnapshotByKey(test.key)
			assert.Nil(t, err)
		})
	}
}
