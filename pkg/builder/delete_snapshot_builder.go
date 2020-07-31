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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DeleteSnapshotBuilder builds DeleteSnapshot objects.
type DeleteSnapshotBuilder struct {
	object *backupdriverv1.DeleteSnapshot
}

func ForDeleteSnapshot(ns, name string) *DeleteSnapshotBuilder {
	return &DeleteSnapshotBuilder{
		object: &backupdriverv1.DeleteSnapshot{
			TypeMeta: metav1.TypeMeta{
				APIVersion: backupdriverv1.SchemeGroupVersion.String(),
				Kind:       "DeleteSnapshot",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: ns,
			},
		},
	}
}

// Result returns the built DeleteSnapshot.
func (b *DeleteSnapshotBuilder) Result() *backupdriverv1.DeleteSnapshot {
	// Builder can only create a new CRD with "New" phase
	b.object.Status = backupdriverv1.DeleteSnapshotStatus{
		Phase:   backupdriverv1.DeleteSnapshotPhaseNew,
	}
	return b.object
}

// SnapshotID set the snap-id for this specific delete snapshot request.
func (b *DeleteSnapshotBuilder) SnapshotID(snapshotID string) *DeleteSnapshotBuilder {
	b.object.Spec.SnapshotID = snapshotID
	return b
}

// BackupRepository sets the name of the backup repository for this specific delete snapshot.
func (b *DeleteSnapshotBuilder) BackupRepository(backupRepositoryName string) *DeleteSnapshotBuilder {
	b.object.Spec.BackupRepository = backupRepositoryName
	return b
}

