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

package abort

import (
	"github.com/spf13/cobra"
	"github.com/vmware-tanzu/velero/pkg/client"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/cmd/cli/download"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/cmd/cli/upload"
)

func NewCommand(f client.Factory) *cobra.Command {
	c := &cobra.Command{
		Use:   "abort",
		Short: "Abort data manager operations",
		Long:  "Abort data manager operations",
	}

	c.AddCommand(
		upload.NewAbortCommand(f, "upload"),
		download.NewAbortCommand(f, "download"),
	)

	return c
}