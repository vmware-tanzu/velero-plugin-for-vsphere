package main

import (
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/cmd"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/cmd/datamgr"
	"os"
	"path/filepath"

	"k8s.io/klog"
)

func main() {
	defer klog.Flush()

	baseName := filepath.Base(os.Args[0])

	err := datamgr.NewCommand(baseName).Execute()
	cmd.CheckError(err)
}