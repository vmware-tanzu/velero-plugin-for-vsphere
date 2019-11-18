package datamgr

import (
	"flag"
	"fmt"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/cmd/server"
	"os"

	"github.com/spf13/cobra"
	"k8s.io/klog"

	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/client"
	veleroflag "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/cmd/util/flag"
	"github.com/vmware-tanzu/velero/pkg/features"
)

func NewCommand(name string) *cobra.Command {
	// Load the config here so that we can extract features from it.
	config, err := client.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: Error reading config file: %v\n", err)
	}

	//// Declare cmdFeatures here so we can access them in the PreRun hooks
	//// without doing a chain of calls into the command's FlagSet
	var cmdFeatures veleroflag.StringArray

	c := &cobra.Command{
		Use:   name,
		Short: "Back up and restore Kubernetes cluster resources.",
		Long: `Velero is a tool for managing disaster recovery, specifically for Kubernetes
cluster resources. It provides a simple, configurable, and operationally robust
way to back up your application state and associated data.

If you're familiar with kubectl, Velero supports a similar model, allowing you to
execute commands such as 'velero get backup' and 'velero create schedule'. The same
operations can also be performed as 'velero backup get' and 'velero schedule create'.`,
		// PersistentPreRun will run before all subcommands EXCEPT in the following conditions:
		//  - a subcommand defines its own PersistentPreRun function
		//  - the command is run without arguments or with --help and only prints the usage info
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			features.Enable(config.Features()...)
			features.Enable(cmdFeatures...)
		},
	}

	f := client.NewFactory(name, config)
	f.BindFlags(c.PersistentFlags())

	// Bind features directly to the root command so it's available to all callers.
	c.PersistentFlags().Var(&cmdFeatures, "features", "Comma-separated list of features to enable for this Velero process. Combines with values from $HOME/.config/velero/config.json if present")

	c.AddCommand(
		server.NewCommand(f),
	)

	// init and add the klog flags
	klog.InitFlags(flag.CommandLine)
	c.PersistentFlags().AddGoFlagSet(flag.CommandLine)

	return c
}