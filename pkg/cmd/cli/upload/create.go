package upload

import (
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/vmware-tanzu/velero/pkg/client"
	"github.com/vmware-tanzu/velero/pkg/cmd/util/output"
	"github.com/vmware-tanzu/velero/pkg/util/logging"
	veleroclient "github.com/vmware-tanzu/velero/pkg/generated/clientset/versioned"
	"strings"
)

func NewCreateCommand(f client.Factory, use string) *cobra.Command {
	o := NewCreateOptions()
	logLevelFlag := logging.LogLevelFlag(logrus.InfoLevel)
	formatFlag := logging.NewFormatFlag()

	c := &cobra.Command{
		Use:   use + " NAME",
		Short: "Create a backup",
		Args:  cobra.ExactArgs(1),
		Example: `	# upload a snapshot to durable storage
	datamgr upload create upload1 --snapshot-id fcd:<fcd-id>:<snapshot-id>`,
		Run: func(c *cobra.Command, args []string) {
			//cmd.CheckError(o.Complete(args, f))
			//cmd.CheckError(o.Validate(c, args, f))
			//cmd.CheckError(o.Run(c, f))
			logLevel := logLevelFlag.Parse()
			logger := logging.DefaultLogger(logLevel, formatFlag.Parse())
			logger.Debugf("setting log-level to %s", strings.ToUpper(logLevel.String()))
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
	SnapshotId              string
	StorageLocation         string
	SnapshotLocations       []string
	Node                    string
	Wait                    bool

	client veleroclient.Interface
}

func (o *CreateOptions) BindFlags(flags *pflag.FlagSet) {
	flags.StringVar(&o.SnapshotId, "snapshot-id", "", "The ID of Protected Entity to be uploaded")
	flags.StringVar(&o.StorageLocation, "storage-location", "", "location in which to store the backup")
	flags.StringVar(&o.Node, "datamgr-node", "", "node in which to do the upload operation")
	flags.StringSliceVar(&o.SnapshotLocations, "volume-snapshot-locations", o.SnapshotLocations, "list of locations (at most one per provider) where volume snapshots should be stored")
}

func NewCreateOptions() *CreateOptions {
	return &CreateOptions{}
}