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

package main

import (
	veleroplugin "github.com/vmware-tanzu/velero/pkg/plugin/framework"
	"github.com/sirupsen/logrus"
	plugins_pkg "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/plugin"
)

func main() {
	veleroplugin.NewServer().
		RegisterVolumeSnapshotter("velero.io/vsphere", newVolumeSnapshotterPlugin).
		RegisterBackupItemAction("velero.io/vsphere-pvc-backupper", newPVCBackupItemAction).
		RegisterBackupItemAction("velero.io/vsphere-pvc-restorer", newPVCRestoreItemAction).
		Serve()
}

func newVolumeSnapshotterPlugin(logger logrus.FieldLogger) (interface{}, error) {
	return &plugins_pkg.NewVolumeSnapshotter{FieldLogger: logger}, nil
}

func newPVCBackupItemAction(logger logrus.FieldLogger) (interface{}, error) {
	return &plugins_pkg.NewPVCBackupItemAction{Log: logger}, nil
}

func newPVCRestoreItemAction(logger logrus.FieldLogger) (interface{}, error) {
	return &plugins_pkg.NewPVCRestoreItemAction{Log: logger}, nil
}