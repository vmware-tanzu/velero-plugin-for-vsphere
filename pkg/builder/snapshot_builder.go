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

package builder

import (
	backupdriverv1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/backupdriver/v1"
	core_v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SnapshotBuilder builds Snapshot objects.
type SnapshotBuilder struct {
	object *backupdriverv1.Snapshot
}

func ForSnapshot(ns, name string) *SnapshotBuilder {
	return &SnapshotBuilder{
		object: &backupdriverv1.Snapshot{
			TypeMeta: metav1.TypeMeta{
				APIVersion: backupdriverv1.SchemeGroupVersion.String(),
				Kind:       "Snapshot",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: ns,
			},
		},
	}
}

// Result returns the built Snapshot.
func (b *SnapshotBuilder) Result() *backupdriverv1.Snapshot {
	return b.object
}

// BackupRepository sets the name of the backup repository for this specific snapshot.
func (b *SnapshotBuilder) BackupRepository(backupRepositoryName string) *SnapshotBuilder {
	b.object.Spec.BackupRepository = backupRepositoryName
	return b
}

// Set the spec object reference
func (b *SnapshotBuilder) ObjectReference(objectToSnapshot core_v1.TypedLocalObjectReference) *SnapshotBuilder {
	b.object.Spec.TypedLocalObjectReference = objectToSnapshot
	return b
}

// Set the spec cancel state
func (b *SnapshotBuilder) CancelState(cancelState bool) *SnapshotBuilder {
	b.object.Spec.SnapshotCancel = cancelState
	return b
}

func (b *SnapshotBuilder) Phase(phase backupdriverv1.SnapshotPhase) *SnapshotBuilder {
	b.object.Status.Phase = phase
	return b
}