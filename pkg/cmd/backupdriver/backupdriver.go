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

package backupdriver

import (
	"flag"
	"fmt"
	"os"

	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/cmd/backupdriver/cli/install"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/cmd/backupdriver/cli/server"

	"github.com/spf13/cobra"
	"k8s.io/klog"

	"github.com/vmware-tanzu/velero/pkg/client"
)

func NewCommand(name string) *cobra.Command {
	// Load the config here so that we can extract features from it.
	config, err := client.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: Error reading config file: %v\n", err)
	}

	c := &cobra.Command{
		Use:   name,
		Short: "Create, Clone and Delete snapshots on vSphere kubernetes cluster",
		Long: `Backup driver is a component in Velero vSphere plugin for
			creating, cloning and deleting snapshots on vsphere storage. It does not move the
		    snapshot data between the local and remote durable storage, but creates CRs for
		    those tasks. The backup driver runs in separate container from the velero server`,
	}

	f := client.NewFactory(name, "", config)
	f.BindFlags(c.PersistentFlags())

	c.AddCommand(
		server.NewCommand(f),
		install.NewCommand(f),
	)

	// init and add the klog flags
	klog.InitFlags(flag.CommandLine)
	c.PersistentFlags().AddGoFlagSet(flag.CommandLine)

	return c
}
