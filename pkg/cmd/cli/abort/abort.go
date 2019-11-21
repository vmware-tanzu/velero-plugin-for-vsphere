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