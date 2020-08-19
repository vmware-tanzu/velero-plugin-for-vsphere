package builder

import (
	backupdriverv1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/backupdriver/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CloneFromSnapshotBuilder builds CloneFromSnapshot objects.
type CloneFromSnapshotBuilder struct {
	object *backupdriverv1.CloneFromSnapshot
}

func ForCloneFromSnapshot(ns, name string) *CloneFromSnapshotBuilder {
	return &CloneFromSnapshotBuilder{
		object: &backupdriverv1.CloneFromSnapshot{
			TypeMeta: metav1.TypeMeta{
				APIVersion: backupdriverv1.SchemeGroupVersion.String(),
				Kind:       "CloneFromSnapshot",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: ns,
			},
		},
	}
}

func (b *CloneFromSnapshotBuilder) Result() *backupdriverv1.CloneFromSnapshot {
	return b.object
}

func (b *CloneFromSnapshotBuilder) Phase(phase backupdriverv1.ClonePhase) *CloneFromSnapshotBuilder {
	b.object.Status.Phase = phase
	return b
}

// Set the spec cancel state
func (b *CloneFromSnapshotBuilder) CancelState(cancelState bool) *CloneFromSnapshotBuilder {
	b.object.Spec.CloneCancel = cancelState
	return b
}