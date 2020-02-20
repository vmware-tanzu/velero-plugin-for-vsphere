package install

import (
	"fmt"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/cmd"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/install"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/utils"
	"github.com/vmware-tanzu/velero/pkg/client"
	"github.com/vmware-tanzu/velero/pkg/cmd/util/flag"
	"github.com/vmware-tanzu/velero/pkg/cmd/util/output"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	kubeutil "github.com/vmware-tanzu/velero/pkg/util/kube"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"io/ioutil"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"os"
	"path/filepath"
)

type InstallOptions struct {
	Namespace                         string
	Image                             string
	BucketName                        string
	Prefix                            string
	ProviderName                      string
	PodAnnotations                    flag.Map
	DatamgrPodCPURequest               string
	DatamgrPodMemRequest               string
	DatamgrPodCPULimit                 string
	DatamgrPodMemLimit                 string
	SecretFile                        string
	NoSecret                          bool
	DryRun                            bool
}

func (o *InstallOptions) BindFlags(flags *pflag.FlagSet) {
	flags.StringVar(&o.ProviderName, "provider", o.ProviderName, "provider name for backup and volume storage")
	flags.StringVar(&o.BucketName, "bucket", o.BucketName, "name of the object storage bucket where backups should be stored")
	flags.StringVar(&o.SecretFile, "secret-file", o.SecretFile, "file containing credentials for backup and volume provider. If not specified, --no-secret must be used for confirmation. Optional.")
	flags.BoolVar(&o.NoSecret, "no-secret", o.NoSecret, "flag indicating if a secret should be created. Must be used as confirmation if --secret-file is not provided. Optional.")
	flags.StringVar(&o.Image, "image", o.Image, "image to use for the Velero and datamgr server pods. Optional.")
	flags.StringVar(&o.Prefix, "prefix", o.Prefix, "prefix under which all Velero data should be stored within the bucket. Optional.")
	flags.Var(&o.PodAnnotations, "pod-annotations", "annotations to add to the Velero and restic pods. Optional. Format is key1=value1,key2=value2")
	flags.StringVar(&o.DatamgrPodCPURequest, "datamgr-pod-cpu-request", o.DatamgrPodCPURequest, `CPU request for Datamgr pod. A value of "0" is treated as unbounded. Optional.`)
	flags.StringVar(&o.DatamgrPodMemRequest, "datamgr-pod-mem-request", o.DatamgrPodMemRequest, `memory request for Datamgr pod. A value of "0" is treated as unbounded. Optional.`)
	flags.StringVar(&o.DatamgrPodCPULimit, "datamgr-pod-cpu-limit", o.DatamgrPodCPULimit, `CPU limit for Datamgr pod. A value of "0" is treated as unbounded. Optional.`)
	flags.StringVar(&o.DatamgrPodMemLimit, "datamgr-pod-mem-limit", o.DatamgrPodMemLimit, `memory limit for Datamgr pod. A value of "0" is treated as unbounded. Optional.`)
	flags.BoolVar(&o.DryRun, "dry-run", o.DryRun, "generate resources, but don't send them to the cluster. Use with -o. Optional.")
}

func NewInstallOptions() *InstallOptions {
	return &InstallOptions{
		Namespace:                 "velero",
		Image:                     install.DefaultImage,
		PodAnnotations:            flag.NewMap(),
		DatamgrPodCPURequest:      install.DefaultDatamgrPodCPURequest,
		DatamgrPodMemRequest:      install.DefaultDatamgrPodMemRequest,
		DatamgrPodCPULimit:        install.DefaultDatamgrPodCPULimit,
		DatamgrPodMemLimit:        install.DefaultDatamgrPodMemLimit,
	}
}

// AsVeleroOptions translates the values provided at the command line into values used to instantiate Kubernetes resources
func (o *InstallOptions) AsDatamgrOptions() (*install.DatamgrOptions, error) {
	var secretData []byte
	if o.SecretFile != "" && !o.NoSecret {
		realPath, err := filepath.Abs(o.SecretFile)
		if err != nil {
			return nil, err
		}
		secretData, err = ioutil.ReadFile(realPath)
		if err != nil {
			return nil, err
		}
	}
	datamgrPodResources, err := kubeutil.ParseResourceRequirements(o.DatamgrPodCPURequest, o.DatamgrPodMemRequest, o.DatamgrPodCPULimit, o.DatamgrPodMemLimit)
	if err != nil {
		return nil, err
	}

	return &install.DatamgrOptions{
		Namespace:                         o.Namespace,
		Image:                             o.Image,
		ProviderName:                      o.ProviderName,
		Bucket:                            o.BucketName,
		Prefix:                            o.Prefix,
		PodAnnotations:                    o.PodAnnotations.Data(),
		DatamgrPodResources:			   datamgrPodResources,
		SecretData:                        secretData,
	}, nil
}

func NewCommand(f client.Factory) *cobra.Command {
	o := NewInstallOptions()
	c := &cobra.Command{
		Use:   "install",
		Short: "Install data manager",
		Long: "Install data manager",
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
	var resources *unstructured.UnstructuredList

	isLocalMode := utils.GetBool(install.DefaultImageLocalMode, false)
	fmt.Printf("The Image LocalMode: %v\n", isLocalMode)
	if isLocalMode {
		fmt.Println("Skip the data manager installation")
		return nil
	}

	vo, err := o.AsDatamgrOptions()
	if err != nil {
		return err
	}

	resources, err = install.AllResources(vo, true)
	if err != nil {
		return err
	}

	if _, err := output.PrintWithFormat(c, resources); err != nil {
		return err
	}

	if o.DryRun {
		return nil
	}
	dynamicClient, err := f.DynamicClient()
	if err != nil {
		return err
	}
	factory := client.NewDynamicFactory(dynamicClient)

	nNodes, err := o.getNumberOfNodes(f)
	if err != nil {
		return errors.Wrap(err, "Error while getting number of nodes in kubernetes cluster")
	}

	errorMsg := fmt.Sprintf("\n\nError installing data manager. Use `kubectl logs daemonset/datamgr-for-vsphere-plugin -n %s` to check the deploy logs", o.Namespace)

	err = install.Install(factory, resources, os.Stdout)
	if err != nil {
		return errors.Wrap(err, errorMsg)
	}

	fmt.Println("Waiting for data manager daemonset to be ready.")
	if _, err = install.DaemonSetIsReady(factory, o.Namespace, nNodes); err != nil {
		return errors.Wrap(err, errorMsg)
	}

	fmt.Printf("Data manager is installed! ⛵ Use 'kubectl logs daemonset/datamgr-for-vsphere-plugin -n %s' to view the status.\n", o.Namespace)
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

	_, err = clientset.CoreV1().Namespaces().Get(namespace, metav1.GetOptions{})
	if err != nil {
		fmt.Printf("Failed to get the specified namespace, %v, for velero in the current k8s cluster\n", namespace)
		return errors.WithStack(err)
	}
	o.Namespace = namespace
	fmt.Printf("velero is running in the namespace, %v\n", namespace)

	return nil
}

func (o *InstallOptions) getNumberOfNodes(f client.Factory) (int, error) {
	clientset, err := f.KubeClient()
	if err != nil {
		return 0, err
	}

	nodeList, err := clientset.CoreV1().Nodes().List(metav1.ListOptions{})
	if err != nil {
		return 0, err
	}

	return len(nodeList.Items), nil
}
