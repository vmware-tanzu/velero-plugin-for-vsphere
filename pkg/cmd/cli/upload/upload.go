package upload

import (
	"github.com/spf13/cobra"

	"github.com/vmware-tanzu/velero/pkg/client"
)

func NewCommand(f client.Factory) *cobra.Command {
	c := &cobra.Command{
		Use:   "upload",
		Short: "Work with data manager upload",
		Long:  "Work with data manager upload",
	}

	//c.AddCommand(
	//	NewCreateCommand(f, "create"),
	//	NewGetCommand(f, "get"),
	//	NewDescribeCommand(f, "describe"),
	//	NewDeleteCommand(f, "delete"),
	//)

	return c
}