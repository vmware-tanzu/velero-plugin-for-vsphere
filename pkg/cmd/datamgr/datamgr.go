package datamgr

import (
	"flag"
	"fmt"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/cmd/cli/install"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/cmd/server"
	"os"

	"github.com/spf13/cobra"
	"k8s.io/klog"

	"github.com/vmware-tanzu/velero/pkg/client"
	//"github.com/vmware-tanzu/velero/pkg/features"
)

func NewCommand(name string) *cobra.Command {
	// Load the config here so that we can extract features from it.
	config, err := client.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: Error reading config file: %v\n", err)
	}

	//// Declare cmdFeatures here so we can access them in the PreRun hooks
	//// without doing a chain of calls into the command's FlagSet
	//var cmdFeatures veleroflag.StringArray

	c := &cobra.Command{
		Use:   name,
		Short: "Upload and download snapshots of persistent volume on vSphere kubernetes cluster",
		Long: `Data manager is a component in Velero vSphere plugin for
			moving local snapshotted data from/to remote durable persistent storage. 
			Specifically, the data manager component is supposed to be running
			in separate container from the velero server`,
		//// PersistentPreRun will run before all subcommands EXCEPT in the following conditions:
		////  - a subcommand defines its own PersistentPreRun function
		////  - the command is run without arguments or with --help and only prints the usage info
		//PersistentPreRun: func(cmd *cobra.Command, args []string) {
		//	features.Enable(config.Features()...)
		//	features.Enable(cmdFeatures...)
		//},
	}

	f := client.NewFactory(name, config)
	f.BindFlags(c.PersistentFlags())

	// Bind features directly to the root command so it's available to all callers.
	//c.PersistentFlags().Var(&cmdFeatures, "features", "Comma-separated list of features to enable for this Velero process. Combines with values from $HOME/.config/velero/config.json if present")

	c.AddCommand(
		server.NewCommand(f),
		install.NewCommand(f),
//		upload.NewCommand(f),
//		download.NewCommand(f),
//		abort.NewCommand(f),
	)

	// init and add the klog flags
	klog.InitFlags(flag.CommandLine)
	c.PersistentFlags().AddGoFlagSet(flag.CommandLine)

	return c
}