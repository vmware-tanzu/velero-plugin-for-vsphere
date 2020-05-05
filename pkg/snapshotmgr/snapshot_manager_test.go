/*
 * Copyright 2019 the Velero contributors
 * SPDX-License-Identifier: Apache-2.0
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package snapshotmgr

import (
	v1api "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/veleroplugin/v1"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/builder"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
	"time"
)

func TestUpload_Creation(t *testing.T) {
	pluginClient := fake.NewSimpleClientset()
	upload := builder.ForUpload("velero", "upload-1").SnapshotID("ssid-1").BackupTimestamp(time.Now()).Phase(v1api.UploadPhaseNew).Result()

	upload2 := &v1api.Upload{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1api.SchemeGroupVersion.String(),
			Kind:       "Upload",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "velero",
			Name:      "upload-2",
		},
		Spec: v1api.UploadSpec{
			SnapshotID: "ssid-2",
			BackupTimestamp: &metav1.Time{Time: time.Now()},
		},
		Status: v1api.UploadStatus{
			Phase: v1api.UploadPhaseNew,
		},
	}
	pluginClient.VeleropluginV1().Uploads("velero").Create(upload)
	pluginClient.VeleropluginV1().Uploads("velero").Create(upload2)
}

func TestDownload_Creation(t *testing.T) {
	pluginClient := fake.NewSimpleClientset()
	download := builder.ForDownload("velero", "download-1").RestoreTimestamp(time.Now()).SnapshotID("ssid-1").Phase(v1api.DownloadPhaseNew).Result()

	pluginClient.VeleropluginV1().Downloads("velero").Create(download)
}
