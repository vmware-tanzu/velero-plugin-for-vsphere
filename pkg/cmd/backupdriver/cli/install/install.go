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
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/constants"

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
	MasterAffinity bool
	HostNetwork    bool
}

func (o *InstallOptions) BindFlags(flags *pflag.FlagSet) {
	// TODO: remove flags that are not required by the backup-driver
	flags.StringVar(&o.Image, "image", o.Image, "image to use for the Velero and backupDriver server pods. Optional.")
	flags.StringVar(&o.PodCPURequest, "pod-cpu-request", o.PodCPURequest, `CPU request for backup-driver pod. A value of "0" is treated as unbounded. Optional.`)
	flags.StringVar(&o.PodMemRequest, "pod-mem-request", o.PodMemRequest, `memory request for backup-driver pod. A value of "0" is treated as unbounded. Optional.`)
	flags.StringVar(&o.PodCPULimit, "pod-cpu-limit", o.PodCPULimit, `CPU limit for backup-driver pod. A value of "0" is treated as unbounded. Optional.`)
	flags.StringVar(&o.PodMemLimit, "pod-mem-limit", o.PodMemLimit, `memory limit for backup-driver pod. A value of "0" is treated as unbounded. Optional.`)
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
		MasterAffinity: o.MasterAffinity,
		HostNetwork:    o.HostNetwork,
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
	kubeClient, err := f.KubeClient()
	if err != nil {
		return errors.Wrap(err, "Failed to get kubeClient")
	}

	// Start with a few of prerequisite checks before installing backup-driver
	fmt.Println("The prerequisite checks for backup-driver started")

	// Check vSphere CSI driver version
	_ = cmd.CheckVSphereCSIDriverVersion(kubeClient)

	// Check velero version
	_ = cmd.CheckVeleroVersion(kubeClient, o.Namespace)

	// Check velero-plugin-for-vsphere image repo and parse the corresponding image for backup-driver
	o.Image, _ = cmd.CheckPluginImageRepo(kubeClient, o.Namespace, o.Image, constants.BackupDriverForPlugin)

	// Check cluster flavor for backup-driver
	if err := o.CheckClusterFlavorForBackupDriver(); err != nil {
		return err
	}

	// Check feature flags for backup-driver
	if err := o.CheckFeatureFlagsForBackupDriver(kubeClient); err != nil {
		return err
	}

	fmt.Println("The prerequisite checks for backup-driver completed")

	vo, err := o.AsBackupDriverOptions()
	if err != nil {
		return err
	}

	var resources *unstructured.UnstructuredList
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
	dynamicFactory := client.NewDynamicFactory(dynamicClient)

	errorMsg := fmt.Sprintf("\n\nError installing backup-driver. Use `kubectl logs deploy/%s -n %s` to check the logs",
		constants.BackupDriverForPlugin, o.Namespace)

	err = install.Install(dynamicFactory, resources, os.Stdout)
	if err != nil {
		return errors.Wrap(err, errorMsg)
	}

	fmt.Printf("Waiting for %s deployment to be ready.\n", constants.BackupDriverForPlugin)

	if _, err = install.DeploymentIsReady(dynamicFactory, o.Namespace); err != nil {
		return errors.Wrap(err, errorMsg)
	}

	fmt.Printf("backup-driver is installed! ⛵ Use 'kubectl logs deployment/%s -n %s' to view the status.\n",
		constants.BackupDriverForPlugin, o.Namespace)

	return nil
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

func (o *InstallOptions) CheckClusterFlavorForBackupDriver() error {
	clusterFlavor, err := utils.GetClusterFlavor(nil)
	if err != nil {
		return errors.Wrap(err, "Failed to get cluster flavor for backup-driver")
	}

	// Assign master node affinity and host network to Supervisor deployment
	if clusterFlavor == constants.Supervisor {
		fmt.Printf("Supervisor Cluster. Assign master node affinity and enable host network.")
		o.MasterAffinity = true
		o.HostNetwork = true
	}

	return nil
}

func (o *InstallOptions) CheckFeatureFlagsForBackupDriver(kubeClient kubernetes.Interface) error {
	featureFlags, err := cmd.GetVeleroFeatureFlags(kubeClient, o.Namespace)
	if err != nil {
		fmt.Printf("Failed to decipher velero feature flags: %v, assuming none.\n", err)
	}

	// Create Config Map with the feature states.
	if err := cmd.CreateFeatureStateConfigMap(kubeClient, featureFlags, o.Namespace); err != nil {
		fmt.Printf("Failed to create feature state config map from velero feature flags: %v.\n", err)
		return err
	}

	return nil
}
