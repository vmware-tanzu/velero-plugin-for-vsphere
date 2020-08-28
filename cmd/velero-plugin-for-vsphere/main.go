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
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	plugins_pkg "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/plugin"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/utils"
	"github.com/vmware-tanzu/velero/pkg/features"
	veleroplugin "github.com/vmware-tanzu/velero/pkg/plugin/framework"
)

func main() {
	enableFeatureFlagForVSpherePlugins()
	veleroPluginServer := veleroplugin.NewServer()
	if features.IsEnabled(utils.VSphereItemActionPluginFlag) {
		veleroPluginServer = veleroPluginServer.
			RegisterBackupItemAction("velero.io/vsphere-pvc-backupper", newPVCBackupItemAction).
			RegisterRestoreItemAction("velero.io/vsphere-pvc-restorer", newPVCRestoreItemAction).
			RegisterDeleteItemAction("velero.io/vsphere-pvc-deleter", newPVCDeleteItemAction)
	} else {
		veleroPluginServer = veleroPluginServer.RegisterVolumeSnapshotter("velero.io/vsphere", newVolumeSnapshotterPlugin)
	}
	veleroPluginServer.Serve()
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

func newPVCDeleteItemAction(logger logrus.FieldLogger) (interface{}, error) {
	return &plugins_pkg.NewPVCDeleteItemAction{Log: logger}, nil
}

func enableFeatureFlagForVSpherePlugins() {
	var featureString string
	flags := pflag.CommandLine
	flags.StringVar(&featureString, "features", featureString, "list of feature flags for this plugin")
	flags.ParseErrorsWhitelist.UnknownFlags = true
	flags.Parse(os.Args[1:])

	featureFlags := strings.Split(featureString, ",")
	features.Enable(featureFlags...)
}
