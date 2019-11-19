package download

import (
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/cmd/util/output"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/util/logging"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/client"
)

func NewDescribeCommand(f client.Factory, use string) *cobra.Command {
	var listOptions metav1.ListOptions
	logLevelFlag := logging.LogLevelFlag(logrus.InfoLevel)
	formatFlag := logging.NewFormatFlag()

	c := &cobra.Command{
		Use:   use + " [NAME1] [NAME2] [NAME...]",
		Short: "Describe downloads",
		Run: func(c *cobra.Command, args []string) {
			logLevel := logLevelFlag.Parse()
			logger := logging.DefaultLogger(logLevel, formatFlag.Parse())
			logger.Infof("The command, datamgr download describe, is called")
		},
	}

	c.Flags().StringVarP(&listOptions.LabelSelector, "selector", "l", listOptions.LabelSelector, "only show items matching this label selector")

	output.BindFlags(c.Flags())

	return c
}