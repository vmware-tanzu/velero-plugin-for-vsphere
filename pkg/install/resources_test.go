/*
Copyright 2019 the Velero contributors.

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

package install

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResources(t *testing.T) {
	bsl := BackupStorageLocation("velero", "test", "test", "", make(map[string]string))

	assert.Equal(t, "velero", bsl.ObjectMeta.Namespace)
	assert.Equal(t, "test", bsl.Spec.Provider)
	assert.Equal(t, "test", bsl.Spec.StorageType.ObjectStorage.Bucket)
	assert.Equal(t, make(map[string]string), bsl.Spec.Config)

	vsl := VolumeSnapshotLocation("velero", "test", make(map[string]string))

	assert.Equal(t, "velero", vsl.ObjectMeta.Namespace)
	assert.Equal(t, "test", vsl.Spec.Provider)
	assert.Equal(t, make(map[string]string), vsl.Spec.Config)

	ns := Namespace("velero")

	assert.Equal(t, "velero", ns.Name)

	crb := ClusterRoleBinding("velero")
	// The CRB is a cluster-scoped resource
	assert.Equal(t, "", crb.ObjectMeta.Namespace)
	assert.Equal(t, "velero", crb.Subjects[0].Namespace)

	sa := ServiceAccount("velero", map[string]string{"abcd": "cbd"})
	assert.Equal(t, "velero", sa.ObjectMeta.Namespace)
	assert.Equal(t, "cbd", sa.ObjectMeta.Annotations["abcd"])
}
