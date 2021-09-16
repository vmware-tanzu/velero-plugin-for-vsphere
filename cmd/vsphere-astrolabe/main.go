/*
 * Copyright the Astrolabe contributors
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
	"github.com/vmware-tanzu/astrolabe/pkg/cmd"
	"github.com/vmware-tanzu/astrolabe/pkg/server"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/ivd"
)

func main() {
	addonInits := map[string]server.InitFunc{
		"ivd": ivd.NewIVDProtectedEntityTypeManager,
	}
	cmd.CliCore(addonInits)
}
