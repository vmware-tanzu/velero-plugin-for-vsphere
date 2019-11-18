package main

import (
	"os"
	"path/filepath"

	"k8s.io/klog"

	// TODO: replace the following two deps
	"github.com/vmware-tanzu/velero/pkg/cmd"
	"github.com/vmware-tanzu/velero/pkg/cmd/velero"
)

func main() {
	defer klog.Flush()

	baseName := filepath.Base(os.Args[0])

	// TODO: replace the following lines with
	err := velero.NewCommand(baseName).Execute()
	cmd.CheckError(err)
}