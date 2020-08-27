/*
Copyright 2020 the Velero contributors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package install

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/cmd"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/install"
	pkgInstall "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/install"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/utils"
	"github.com/vmware-tanzu/velero/pkg/client"
	"github.com/vmware-tanzu/velero/pkg/cmd/util/flag"
	"github.com/vmware-tanzu/velero/pkg/cmd/util/output"
	"github.com/vmware-tanzu/velero/pkg/features"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"io/ioutil"
	"os"

	kubeutil "github.com/vmware-tanzu/velero/pkg/util/kube"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type InstallOptions struct {
	Namespace      string
	Image          string
	PodAnnotations flag.Map
	PodCPURequest  string
	PodMemRequest  string
	PodCPULimit    string
	PodMemLimit    string
	PVSecret       bool
	MasterAffinity bool
	HostNetwork    bool
	LocalMode      bool
	Features       string
}

func (o *InstallOptions) BindFlags(flags *pflag.FlagSet) {
	// TODO: remove flags that are not required by the backup-driver
	flags.StringVar(&o.Image, "image", o.Image, "image to use for the Velero and backupDriver server pods. Optional.")
	flags.StringVar(&o.PodCPURequest, "pod-cpu-request", o.PodCPURequest, `CPU request for backup-driver pod. A value of "0" is treated as unbounded. Optional.`)
	flags.StringVar(&o.PodMemRequest, "pod-mem-request", o.PodMemRequest, `memory request for backup-driver pod. A value of "0" is treated as unbounded. Optional.`)
	flags.StringVar(&o.PodCPULimit, "pod-cpu-limit", o.PodCPULimit, `CPU limit for backup-driver pod. A value of "0" is treated as unbounded. Optional.`)
	flags.StringVar(&o.PodMemLimit, "pod-mem-limit", o.PodMemLimit, `memory limit for backup-driver pod. A value of "0" is treated as unbounded. Optional.`)
	flags.StringVar(&o.Features, "features", o.Features, "comma separated list of backup-driver feature flags. Optional.")
}

func NewInstallOptions() *InstallOptions {
	return &InstallOptions{
		Namespace:      "velero",
		Image:          pkgInstall.DefaultBackupDriverImage,
		PodAnnotations: flag.NewMap(),
		PodCPURequest:  pkgInstall.DefaultBackupDriverPodCPURequest,
		PodMemRequest:  pkgInstall.DefaultBackupDriverPodMemRequest,
		PodCPULimit:    pkgInstall.DefaultBackupDriverPodCPULimit,
		PodMemLimit:    pkgInstall.DefaultBackupDriverPodMemLimit,
	}
}

// AsPodOptions translates the values provided at the command line into values used to instantiate Kubernetes resources
func (o *InstallOptions) AsBackupDriverOptions() (*pkgInstall.PodOptions, error) {
	podResources, err := kubeutil.ParseResourceRequirements(o.PodCPURequest, o.PodMemRequest, o.PodCPULimit, o.PodMemLimit)
	if err != nil {
		return nil, err
	}

	return &pkgInstall.PodOptions{
		Namespace:      o.Namespace,
		Image:          o.Image,
		PodAnnotations: o.PodAnnotations.Data(),
		PodResources:   podResources,
		SecretAdd:      o.PVSecret,
		MasterAffinity: o.MasterAffinity,
		HostNetwork:    o.HostNetwork,
		LocalMode:      o.LocalMode,
	}, nil
}

func NewCommand(f client.Factory) *cobra.Command {
	o := NewInstallOptions()

	c := &cobra.Command{
		Use:   "install",
		Short: "Install backup driver",
		Long:  "Install backup driver",
		Run: func(c *cobra.Command, args []string) {
			cmd.CheckError(o.Complete(args, f))
			cmd.CheckError(o.Run(c, f))
		},
	}

	o.BindFlags(c.Flags())
	output.BindFlags(c.Flags())
	output.ClearOutputFlagDefault(c)

	return c
}

func (o *InstallOptions) Run(c *cobra.Command, f client.Factory) error {
	// Check if ItemActionPlugin feature is enabled.
	featureFlags := strings.Split(o.Features, ",")
	features.Enable(featureFlags...)
	if !features.IsEnabled(utils.VSphereItemActionPluginFlag) {
		fmt.Printf("Feature %s is not enabled. Skipping backup-driver installation", utils.VSphereItemActionPluginFlag)
		return nil
	}

	// Check local mode
	o.LocalMode = utils.GetBool(install.DefaultBackupDriverImageLocalMode, false)

	var resources *unstructured.UnstructuredList

	// Check vSphere CSI driver version
	isCSIInstalled, isVersionOk, err := cmd.CheckCSIInstalled(f)
	if err != nil {
		fmt.Println("CSI driver check failed")
		isCSIInstalled = false
		isVersionOk = false
	}
	if !isCSIInstalled {
		fmt.Println("Velero Plug-in for vSphere requires vSphere CSI/CNS and vSphere 6.7U3 to function. Please install the vSphere CSI/CNS driver")
	}
	if !isVersionOk {
		fmt.Printf("vSphere CSI driver version is prior to %s. Velero Plug-in for vSphere requires CSI driver version to be %s or above\n", utils.CsiMinVersion, utils.CsiMinVersion)
	}

	// Check velero version
	veleroVersion, err := cmd.GetVeleroVersion(f)
	if err != nil || veleroVersion == "" {
		fmt.Println("Failed to get velero version.")
	} else {
		if cmd.CompareVersion(veleroVersion, utils.VeleroMinVersion) == -1 {
			fmt.Printf("WARNING: Velero version %s is prior to %s. Velero Plug-in for vSphere requires velero version to be %s or above.\n", veleroVersion, utils.VeleroMinVersion, utils.VeleroMinVersion)
		}
	}

	// Check velero vsphere plugin image repo
	err = o.CheckPluginImageRepo(f)
	if err != nil {
		fmt.Printf("Failed to check plugin image repo, error msg: %s. Using default image %s\n", err.Error(), o.Image)
	} else {
		fmt.Printf("Using image %s.\n", o.Image)
	}

	// Check cluster flavor. Add the PV secret to pod in Guest Cluster
	clusterFlavor, err := utils.GetClusterFlavor(nil)
	if clusterFlavor == utils.TkgGuest {
		fmt.Printf("Guest Cluster. Deploy pod with secret.")
		o.PVSecret = true
	}

	// Assign master node affinity and host network to Supervisor deployment
	if clusterFlavor == utils.Supervisor {
		fmt.Printf("Supervisor Cluster. Assign master node affinity and enable host network.")
		o.MasterAffinity = true
		o.HostNetwork = true
	}

	vo, err := o.AsBackupDriverOptions()
	if err != nil {
		return err
	}

	resources, err = install.AllBackupDriverResources(vo, true)
	if err != nil {
		return err
	}

	if _, err := output.PrintWithFormat(c, resources); err != nil {
		return err
	}

	dynamicClient, err := f.DynamicClient()
	if err != nil {
		return err
	}
	factory := client.NewDynamicFactory(dynamicClient)

	errorMsg := fmt.Sprintf("\n\nError installing backup-driver. Use `kubectl logs deploy/%s -n %s` to check the logs",
		utils.BackupDriverForPlugin, o.Namespace)

	err = install.Install(factory, resources, os.Stdout)
	if err != nil {
		return errors.Wrap(err, errorMsg)
	}

	fmt.Printf("Waiting for %s deployment to be ready.\n", utils.BackupDriverForPlugin)

	if _, err = install.DeploymentIsReady(factory, o.Namespace); err != nil {
		return errors.Wrap(err, errorMsg)
	}

	fmt.Printf("backup-driver is installed! ⛵ Use 'kubectl logs deployment/%s -n %s' to view the status.\n",
		utils.BackupDriverForPlugin, o.Namespace)

	return nil
}

func (o *InstallOptions) CheckPluginImageRepo(f client.Factory) error {
	clientset, err := f.KubeClient()
	if err != nil {
		errMsg := fmt.Sprint("Failed to get clientset.")
		return errors.New(errMsg)
	}
	deployment, err := clientset.AppsV1().Deployments(o.Namespace).Get(context.TODO(), utils.VeleroDeployment, metav1.GetOptions{})
	if err != nil {
		errMsg := fmt.Sprintf("Failed to get velero deployment in namespace %s", o.Namespace)
		return errors.New(errMsg)
	}

	repo := ""
	tag := ""
	for _, container := range deployment.Spec.Template.Spec.InitContainers {
		if strings.Contains(container.Image, utils.VeleroPluginForVsphere) {
			repo = strings.Split(container.Image, "/")[0]
			tag = strings.Split(container.Image, ":")[1]
			break
		}
	}

	if repo != "" && tag != "" {
		o.Image = repo + "/" + utils.BackupDriverForPlugin + ":" + tag
		return nil
	} else {
		errMsg := fmt.Sprint("Failed to get repo and tag from velero plugin image.")
		return errors.New(errMsg)
	}
}

//Complete completes options for a command.
func (o *InstallOptions) Complete(args []string, f client.Factory) error {
	fileName := "/var/run/secrets/kubernetes.io/serviceaccount/namespace"

	_, err := os.Stat(fileName)
	if os.IsNotExist(err) {
		// If the file isn't there, just return an empty map
		fmt.Printf("No namespace specified in the namespace file, %v\n", fileName)
		return nil
	}
	if err != nil {
		// For any other Stat() error, return it
		return errors.WithStack(err)
	}

	content, err := ioutil.ReadFile(fileName)
	if err != nil {
		return errors.WithStack(err)
	}
	namespace := string(content)

	config, err := rest.InClusterConfig()
	if err != nil {
		fmt.Println("Failed to get k8s inClusterConfig")
		return errors.WithStack(err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Println("Failed to get k8s clientset with the given config")
		return errors.WithStack(err)
	}

	_, err = clientset.CoreV1().Namespaces().Get(context.TODO(), namespace, metav1.GetOptions{})
	if err != nil {
		fmt.Printf("Failed to get the specified namespace, %v, for velero in the current k8s cluster\n", namespace)
		return errors.WithStack(err)
	}
	o.Namespace = namespace
	fmt.Printf("velero is running in the namespace, %v\n", namespace)

	return nil
}
