package upload

import (
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/client"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/cmd/util/output"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/util/logging"
	// TODO: change the import package accordingly
	veleroclient "github.com/vmware-tanzu/velero/pkg/generated/clientset/versioned"
)

func NewCreateCommand(f client.Factory, use string) *cobra.Command {
	o := NewCreateOptions()
	logLevelFlag := logging.LogLevelFlag(logrus.InfoLevel)
	formatFlag := logging.NewFormatFlag()

	c := &cobra.Command{
		Use:   use + " NAME",
		Short: "Create a backup",
		Args:  cobra.ExactArgs(1),
		Example: `	# create a backup containing all resources
	datamgr upload create upload1 --protected-entity-id fcd:<fcd-id>:<snapshot-id>`,
		Run: func(c *cobra.Command, args []string) {
			//cmd.CheckError(o.Complete(args, f))
			//cmd.CheckError(o.Validate(c, args, f))
			//cmd.CheckError(o.Run(c, f))
			logLevel := logLevelFlag.Parse()
			logger := logging.DefaultLogger(logLevel, formatFlag.Parse())
			logger.Infof("Starting datamgmt server")
		},
	}

	o.BindFlags(c.Flags())
	output.BindFlags(c.Flags())
	output.ClearOutputFlagDefault(c)

	return c
}

type CreateOptions struct {
	Name                    string
	ProtectedEntityId       string
	StorageLocation         string
	SnapshotLocations       []string
	FromSchedule            string
	Wait                    bool

	client veleroclient.Interface
}

func (o *CreateOptions) BindFlags(flags *pflag.FlagSet) {
	flags.StringVar(&o.ProtectedEntityId, "protected-entity-id", "", "The ID of Protected Entity to be uploaded")
	flags.StringVar(&o.StorageLocation, "storage-location", "", "location in which to store the backup")
	flags.StringSliceVar(&o.SnapshotLocations, "volume-snapshot-locations", o.SnapshotLocations, "list of locations (at most one per provider) where volume snapshots should be stored")
}

func NewCreateOptions() *CreateOptions {
	return &CreateOptions{}
}