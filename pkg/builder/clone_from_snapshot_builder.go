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
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SnapshotBuilder builds Snapshot objects.
type CloneFromSnapshotBuilder struct {
	object *backupdriverv1.CloneFromSnapshot
}

func ForCloneFromSnapshot(ns, name string, labels map[string]string) *CloneFromSnapshotBuilder {
	return &CloneFromSnapshotBuilder{
		object: &backupdriverv1.CloneFromSnapshot{
			TypeMeta: metav1.TypeMeta{
				APIVersion: backupdriverv1.SchemeGroupVersion.String(),
				Kind:       "CloneFromSnapshot",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: ns,
				Labels:    utils.AppendVeleroExcludeLabels(labels),
			},
		},
	}
}

// Result returns the built Snapshot.
func (b *CloneFromSnapshotBuilder) Result() *backupdriverv1.CloneFromSnapshot {
	return b.object
}

// BackupRepository sets the name of the backup repository for this specific snapshot.
func (b *CloneFromSnapshotBuilder) BackupRepository(backupRepositoryName string) *CloneFromSnapshotBuilder {
	b.object.Spec.BackupRepository = backupRepositoryName
	return b
}

func (b *CloneFromSnapshotBuilder) SnapshotID(snapshotID string) *CloneFromSnapshotBuilder {
	b.object.Spec.SnapshotID = snapshotID
	return b
}

func (b *CloneFromSnapshotBuilder) Metadata(metadata []byte) *CloneFromSnapshotBuilder {
	b.object.Spec.Metadata = metadata
	return b
}

func (b *CloneFromSnapshotBuilder) APIGroup(apiGroup *string) *CloneFromSnapshotBuilder {
	b.object.Spec.APIGroup = apiGroup
	return b
}

func (b *CloneFromSnapshotBuilder) Kind(kind string) *CloneFromSnapshotBuilder {
	b.object.Spec.Kind = kind
	return b
}

// Set the spec cancel state
func (b *CloneFromSnapshotBuilder) CancelState(cancelState bool) *CloneFromSnapshotBuilder {
	b.object.Spec.CloneCancel = cancelState
	return b
}
