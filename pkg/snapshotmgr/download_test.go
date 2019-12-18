package snapshotmgr

import (
	v1api "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/veleroplugin/v1"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/builder"
	plugin_clientset "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"testing"
	"time"
)

func TestDownload_Creation(t *testing.T) {
	path := os.Getenv("HOME") + "/.kube/config"
	config, err := clientcmd.BuildConfigFromFlags("", path)
	if err != nil {
		t.Fatal("Got error " + err.Error())
	}
	pluginClient, err := plugin_clientset.NewForConfig(config)
	if err != nil {
		t.Fatal("Got error " + err.Error())
	}

	download := builder.ForDownload("velero", "download-1").RestoreTimestamp(time.Now()).SnapshotID("ssid-1").Phase(v1api.DownloadPhaseNew).Result()

	pluginClient.VeleropluginV1().Downloads("velero").Create(download)
}