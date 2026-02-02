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
	// enable fips only mode
	_ "crypto/tls/fipsonly"

	"github.com/sirupsen/logrus"
	velero_plugin "github.com/vmware-tanzu/velero/pkg/plugin/framework"
	plugin_common "github.com/vmware-tanzu/velero/pkg/plugin/framework/common"
	"k8s.io/client-go/restmapper"

	plugins_pkg "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/plugin"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/utils"
)

func main() {
	logger := logrus.StandardLogger()

	if err := utils.CreateBlockListConfigMap(logger); err != nil {
		logrus.Fatalf("Failed to create block list config map: %v", err)
	}

	mapper, err := utils.GetCachedMapper()
	if err != nil {
		logrus.Fatalf("Failed to create cached mapper: %v", err)
	}

	veleroPluginServer := velero_plugin.NewServer()
	veleroPluginServer = veleroPluginServer.
		RegisterBackupItemAction(
			"velero.io/vsphere-backupper",
			newBackupItemAction(mapper),
		).
		RegisterRestoreItemAction(
			"velero.io/vsphere-restorer",
			newRestoreItemAction(mapper),
		)

	veleroPluginServer.Serve()
}

func newBackupItemAction(mapper *restmapper.DeferredDiscoveryRESTMapper) plugin_common.HandlerInitializer {
	return func(logger logrus.FieldLogger) (any, error) {
		return &plugins_pkg.NewBackupItemAction{
			Log:    logger,
			Mapper: mapper,
		}, nil
	}
}

func newRestoreItemAction(mapper *restmapper.DeferredDiscoveryRESTMapper) plugin_common.HandlerInitializer {
	return func(logger logrus.FieldLogger) (any, error) {
		return &plugins_pkg.NewRestoreItemAction{
			Log:    logger,
			Mapper: mapper,
		}, nil
	}
}
