package install

import (
	"fmt"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/cmd"
	"github.com/vmware-tanzu/velero/pkg/client"
	"github.com/vmware-tanzu/velero/pkg/cmd/util/flag"
	"github.com/vmware-tanzu/velero/pkg/cmd/util/output"
	"github.com/vmware-tanzu/velero/pkg/install"
	"io/ioutil"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"os"
	"path/filepath"
	"strings"
	"time"
	kubeutil "github.com/vmware-tanzu/velero/pkg/util/kube"
)

type InstallOptions struct {
	Namespace                         string
	Image                             string
	BucketName                        string
	Prefix                            string
	ProviderName                      string
	PodAnnotations                    flag.Map
	ServiceAccountAnnotations         flag.Map
	VeleroPodCPURequest               string
	VeleroPodMemRequest               string
	VeleroPodCPULimit                 string
	VeleroPodMemLimit                 string
	RestoreOnly                       bool
	SecretFile                        string
	NoSecret                          bool
	DryRun                            bool
	BackupStorageConfig               flag.Map
	VolumeSnapshotConfig              flag.Map
	UseRestic                         bool
	Wait                              bool
	UseVolumeSnapshots                bool
	DefaultResticMaintenanceFrequency time.Duration
	Plugins                           flag.StringArray
	NoDefaultBackupLocation           bool
	CRDsOnly                          bool
}

func (o *InstallOptions) BindFlags(flags *pflag.FlagSet) {
	flags.StringVar(&o.ProviderName, "provider", o.ProviderName, "provider name for backup and volume storage")
	flags.StringVar(&o.BucketName, "bucket", o.BucketName, "name of the object storage bucket where backups should be stored")
	flags.StringVar(&o.SecretFile, "secret-file", o.SecretFile, "file containing credentials for backup and volume provider. If not specified, --no-secret must be used for confirmation. Optional.")
	flags.BoolVar(&o.NoSecret, "no-secret", o.NoSecret, "flag indicating if a secret should be created. Must be used as confirmation if --secret-file is not provided. Optional.")
	flags.BoolVar(&o.NoDefaultBackupLocation, "no-default-backup-location", o.NoDefaultBackupLocation, "flag indicating if a default backup location should be created. Must be used as confirmation if --bucket or --provider are not provided. Optional.")
	flags.StringVar(&o.Image, "image", o.Image, "image to use for the Velero and restic server pods. Optional.")
	flags.StringVar(&o.Prefix, "prefix", o.Prefix, "prefix under which all Velero data should be stored within the bucket. Optional.")
	flags.Var(&o.PodAnnotations, "pod-annotations", "annotations to add to the Velero and restic pods. Optional. Format is key1=value1,key2=value2")
	flags.Var(&o.ServiceAccountAnnotations, "sa-annotations", "annotations to add to the Velero ServiceAccount. Add iam.gke.io/gcp-service-account=[GSA_NAME]@[PROJECT_NAME].iam.gserviceaccount.com for workload identity. Optional. Format is key1=value1,key2=value2")
	flags.StringVar(&o.VeleroPodCPURequest, "velero-pod-cpu-request", o.VeleroPodCPURequest, `CPU request for Velero pod. A value of "0" is treated as unbounded. Optional.`)
	flags.StringVar(&o.VeleroPodMemRequest, "velero-pod-mem-request", o.VeleroPodMemRequest, `memory request for Velero pod. A value of "0" is treated as unbounded. Optional.`)
	flags.StringVar(&o.VeleroPodCPULimit, "velero-pod-cpu-limit", o.VeleroPodCPULimit, `CPU limit for Velero pod. A value of "0" is treated as unbounded. Optional.`)
	flags.StringVar(&o.VeleroPodMemLimit, "velero-pod-mem-limit", o.VeleroPodMemLimit, `memory limit for Velero pod. A value of "0" is treated as unbounded. Optional.`)
	flags.Var(&o.BackupStorageConfig, "backup-location-config", "configuration to use for the backup storage location. Format is key1=value1,key2=value2")
	flags.Var(&o.VolumeSnapshotConfig, "snapshot-location-config", "configuration to use for the volume snapshot location. Format is key1=value1,key2=value2")
	flags.BoolVar(&o.UseVolumeSnapshots, "use-volume-snapshots", o.UseVolumeSnapshots, "whether or not to create snapshot location automatically. Set to false if you do not plan to create volume snapshots via a storage provider.")
	flags.BoolVar(&o.RestoreOnly, "restore-only", o.RestoreOnly, "run the server in restore-only mode. Optional.")
	flags.BoolVar(&o.DryRun, "dry-run", o.DryRun, "generate resources, but don't send them to the cluster. Use with -o. Optional.")
	flags.BoolVar(&o.UseRestic, "use-restic", o.UseRestic, "create restic deployment. Optional.")
	flags.BoolVar(&o.Wait, "wait", o.Wait, "wait for Velero deployment to be ready. Optional.")
	flags.DurationVar(&o.DefaultResticMaintenanceFrequency, "default-restic-prune-frequency", o.DefaultResticMaintenanceFrequency, "how often 'restic prune' is run for restic repositories by default. Optional.")
	flags.Var(&o.Plugins, "plugins", "Plugin container images to install into the Velero Deployment")
	flags.BoolVar(&o.CRDsOnly, "crds-only", o.CRDsOnly, "only generate CustomResourceDefinition resources. Useful for updating CRDs for an existing Velero install.")
}

func NewInstallOptions() *InstallOptions {
	return &InstallOptions{
		Namespace:                 "velero",
		// TODO: hardcoded for now, will change it to the general expr later.
		Image:                     install.DefaultImage,
		BackupStorageConfig:       flag.NewMap(),
		VolumeSnapshotConfig:      flag.NewMap(),
		PodAnnotations:            flag.NewMap(),
		ServiceAccountAnnotations: flag.NewMap(),
		VeleroPodCPURequest:       install.DefaultVeleroPodCPURequest,
		VeleroPodMemRequest:       install.DefaultVeleroPodMemRequest,
		VeleroPodCPULimit:         install.DefaultVeleroPodCPULimit,
		VeleroPodMemLimit:         install.DefaultVeleroPodMemLimit,
		// Default to creating a VSL unless we're told otherwise
		UseVolumeSnapshots:      true,
		NoDefaultBackupLocation: false,
		CRDsOnly:                false,
	}
}

// AsVeleroOptions translates the values provided at the command line into values used to instantiate Kubernetes resources
func (o *InstallOptions) AsVeleroOptions() (*install.VeleroOptions, error) {
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
	veleroPodResources, err := kubeutil.ParseResourceRequirements(o.VeleroPodCPURequest, o.VeleroPodMemRequest, o.VeleroPodCPULimit, o.VeleroPodMemLimit)
	if err != nil {
		return nil, err
	}

	return &install.VeleroOptions{
		Namespace:                         o.Namespace,
		Image:                             o.Image,
		ProviderName:                      o.ProviderName,
		Bucket:                            o.BucketName,
		Prefix:                            o.Prefix,
		PodAnnotations:                    o.PodAnnotations.Data(),
		ServiceAccountAnnotations:         o.ServiceAccountAnnotations.Data(),
		VeleroPodResources:                veleroPodResources,
		SecretData:                        secretData,
		RestoreOnly:                       o.RestoreOnly,
		UseRestic:                         o.UseRestic,
		UseVolumeSnapshots:                o.UseVolumeSnapshots,
		BSLConfig:                         o.BackupStorageConfig.Data(),
		VSLConfig:                         o.VolumeSnapshotConfig.Data(),
		DefaultResticMaintenanceFrequency: o.DefaultResticMaintenanceFrequency,
		Plugins:                           o.Plugins,
		NoDefaultBackupLocation:           o.NoDefaultBackupLocation,
	}, nil
}

func NewCommand(f client.Factory) *cobra.Command {
	o := NewInstallOptions()
	c := &cobra.Command{
		Use:   "install",
		Short: "Install data manager",
		Long: "Install data manager",
		Run: func(c *cobra.Command, args []string) {
			//cmd.CheckError(o.Validate(c, args, f))
			//cmd.CheckError(o.Complete(args, f))
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

	vo, err := o.AsVeleroOptions()
	if err != nil {
		return err
	}

	resources, err = install.AllResources(vo)
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

	errorMsg := fmt.Sprintf("\n\nError installing data manager. Use `kubectl logs deploy/velero -n %s` to check the deploy logs", o.Namespace)

	err = install.Install(factory, resources, os.Stdout)
	if err != nil {
		return errors.Wrap(err, errorMsg)
	}

	if o.Wait {
		fmt.Println("Waiting for data manager daemonset to be ready.")
		if _, err = install.DaemonSetIsReady(factory, o.Namespace); err != nil {
			return errors.Wrap(err, errorMsg)
		}
	}

	if o.SecretFile == "" {
		fmt.Printf("\nNo secret file was specified, no Secret created.\n\n")
	}

	if o.NoDefaultBackupLocation {
		fmt.Printf("\nNo bucket and provider were specified, no default backup storage location created.\n\n")
	}

	fmt.Printf("Data manager is installed! ⛵ Use 'kubectl logs daemonset/datamgr -n %s' to view the status.\n", o.Namespace)
	return nil
}

//Complete completes options for a command.
func (o *InstallOptions) Complete(args []string, f client.Factory) error {
	o.Namespace = f.Namespace()
	return nil
}

// Validate validates options provided to a command.
func (o *InstallOptions) Validate(c *cobra.Command, args []string, f client.Factory) error {
	if err := output.ValidateFlags(c); err != nil {
		return err
	}

	// If we're only installing CRDs, we can skip the rest of the validation.
	if o.CRDsOnly {
		return nil
	}

	// Our main 3 providers don't support bucket names starting with a dash, and a bucket name starting with one
	// can indicate that an environment variable was left blank.
	// This case will help catch that error
	if strings.HasPrefix(o.BucketName, "-") {
		return errors.Errorf("Bucket names cannot begin with a dash. Bucket name was: %s", o.BucketName)
	}

	if o.NoDefaultBackupLocation {

		if o.BucketName != "" {
			return errors.New("Cannot use both --bucket and --no-default-backup-location at the same time")
		}

		if o.Prefix != "" {
			return errors.New("Cannot use both --prefix and --no-default-backup-location at the same time")
		}

		if o.BackupStorageConfig.String() != "" {
			return errors.New("Cannot use both --backup-location-config and --no-default-backup-location at the same time")
		}
	} else {
		if o.ProviderName == "" {
			return errors.New("--provider is required")
		}

		if o.BucketName == "" {
			return errors.New("--bucket is required")
		}

	}

	if o.UseVolumeSnapshots {
		if o.ProviderName == "" {
			return errors.New("--provider is required when --use-volume-snapshots is set to true")
		}
	} else {
		if o.VolumeSnapshotConfig.String() != "" {
			return errors.New("--snapshot-location-config must be empty when --use-volume-snapshots=false")
		}
	}

	if o.NoDefaultBackupLocation && !o.UseVolumeSnapshots {
		if o.ProviderName != "" {
			return errors.New("--provider must be empty when using --no-default-backup-location and --use-volume-snapshots=false")
		}
	} else {
		if len(o.Plugins) == 0 {
			return errors.New("--plugins flag is required")
		}
	}

	switch {
	case o.SecretFile == "" && !o.NoSecret:
		return errors.New("One of --secret-file or --no-secret is required")
	case o.SecretFile != "" && o.NoSecret:
		return errors.New("Cannot use both --secret-file and --no-secret")
	}

	return nil
}
