package server

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/cmd"
	"github.com/vmware-tanzu/velero/pkg/buildinfo"
	"github.com/vmware-tanzu/velero/pkg/client"
	"github.com/vmware-tanzu/velero/pkg/cmd/util/flag"
	"github.com/vmware-tanzu/velero/pkg/cmd/util/signals"
	velerodiscovery "github.com/vmware-tanzu/velero/pkg/discovery"
	"github.com/vmware-tanzu/velero/pkg/metrics"
	"github.com/vmware-tanzu/velero/pkg/util/logging"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"log"
	"os"
	"strings"
	"time"
	clientset "github.com/vmware-tanzu/velero/pkg/generated/clientset/versioned"
	informers "github.com/vmware-tanzu/velero/pkg/generated/informers/externalversions"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	kubeerrs "k8s.io/apimachinery/pkg/util/errors"
)

type serverConfig struct {
	pluginDir, metricsAddress, defaultBackupLocation                        string
	backupSyncPeriod, podVolumeOperationTimeout, resourceTerminatingTimeout time.Duration
	defaultBackupTTL                                                        time.Duration
	restoreResourcePriorities                                               []string
	defaultVolumeSnapshotLocations                                          map[string]string
	restoreOnly                                                             bool
	disabledControllers                                                     []string
	clientQPS                                                               float32
	clientBurst                                                             int
	profilerAddress                                                         string
	formatFlag                                                              *logging.FormatFlag
	defaultResticMaintenanceFrequency                                       time.Duration
}

func NewCommand(f client.Factory) *cobra.Command {
	var (
		volumeSnapshotLocations = flag.NewMap().WithKeyValueDelimiter(":")
		logLevelFlag            = logging.LogLevelFlag(logrus.InfoLevel)
		config                  = serverConfig{
			pluginDir:                         "/plugins",
			//metricsAddress:                    defaultMetricsAddress,
			defaultBackupLocation:             "default",
			defaultVolumeSnapshotLocations:    make(map[string]string),
			//backupSyncPeriod:                  defaultBackupSyncPeriod,
			//defaultBackupTTL:                  defaultBackupTTL,
			//podVolumeOperationTimeout:         defaultPodVolumeOperationTimeout,
			//restoreResourcePriorities:         defaultRestorePriorities,
			//clientQPS:                         defaultClientQPS,
			//clientBurst:                       defaultClientBurst,
			//profilerAddress:                   defaultProfilerAddress,
			//resourceTerminatingTimeout:        defaultResourceTerminatingTimeout,
			formatFlag:                        logging.NewFormatFlag(),
			//defaultResticMaintenanceFrequency: restic.DefaultMaintenanceFrequency,
		}
	)

	var command = &cobra.Command{
		Use:    "server",
		Short:  "Run the data manager server",
		Long:   "Run the data manager server",
		Hidden: true,
		Run: func(c *cobra.Command, args []string) {
			// go-plugin uses log.Println to log when it's waiting for all plugin processes to complete so we need to
			// set its output to stdout.
			log.SetOutput(os.Stdout)

			logLevel := logLevelFlag.Parse()
			format := config.formatFlag.Parse()

			// Make sure we log to stdout so cloud log dashboards don't show this as an error.
			logrus.SetOutput(os.Stdout)

			// Velero's DefaultLogger logs to stdout, so all is good there.
			logger := logging.DefaultLogger(logLevel, format)

			logger.Infof("setting log-level to %s", strings.ToUpper(logLevel.String()))

			logger.Infof("Starting data manager server %s (%s)", buildinfo.Version, buildinfo.FormattedGitSHA())

			if volumeSnapshotLocations.Data() != nil {
				config.defaultVolumeSnapshotLocations = volumeSnapshotLocations.Data()
			}

			f.SetBasename(fmt.Sprintf("%s-%s", c.Parent().Name(), c.Name()))

			s, err := newServer(f, config, logger)
			cmd.CheckError(err)

			cmd.CheckError(s.run())
		},
	}

	command.Flags().Var(logLevelFlag, "log-level", fmt.Sprintf("the level at which to log. Valid values are %s.", strings.Join(logLevelFlag.AllowedValues(), ", ")))
	command.Flags().Var(config.formatFlag, "log-format", fmt.Sprintf("the format for log output. Valid values are %s.", strings.Join(config.formatFlag.AllowedValues(), ", ")))
	command.Flags().StringVar(&config.pluginDir, "plugin-dir", config.pluginDir, "directory containing Velero plugins")
	command.Flags().StringVar(&config.metricsAddress, "metrics-address", config.metricsAddress, "the address to expose prometheus metrics")
	command.Flags().DurationVar(&config.backupSyncPeriod, "backup-sync-period", config.backupSyncPeriod, "how often to ensure all Velero backups in object storage exist as Backup API objects in the cluster. This is the default sync period if none is explicitly specified for a backup storage location.")
	command.Flags().DurationVar(&config.podVolumeOperationTimeout, "restic-timeout", config.podVolumeOperationTimeout, "how long backups/restores of pod volumes should be allowed to run before timing out")
	command.Flags().BoolVar(&config.restoreOnly, "restore-only", config.restoreOnly, "run in a mode where only restores are allowed; backups, schedules, and garbage-collection are all disabled. DEPRECATED: this flag will be removed in v2.0. Use read-only backup storage locations instead.")
	//command.Flags().StringSliceVar(&config.disabledControllers, "disable-controllers", config.disabledControllers, fmt.Sprintf("list of controllers to disable on startup. Valid values are %s", strings.Join(disableControllerList, ",")))
	command.Flags().StringSliceVar(&config.restoreResourcePriorities, "restore-resource-priorities", config.restoreResourcePriorities, "desired order of resource restores; any resource not in the list will be restored alphabetically after the prioritized resources")
	command.Flags().StringVar(&config.defaultBackupLocation, "default-backup-storage-location", config.defaultBackupLocation, "name of the default backup storage location")
	command.Flags().Var(&volumeSnapshotLocations, "default-volume-snapshot-locations", "list of unique volume providers and default volume snapshot location (provider1:location-01,provider2:location-02,...)")
	command.Flags().Float32Var(&config.clientQPS, "client-qps", config.clientQPS, "maximum number of requests per second by the server to the Kubernetes API once the burst limit has been reached")
	command.Flags().IntVar(&config.clientBurst, "client-burst", config.clientBurst, "maximum number of requests by the server to the Kubernetes API in a short period of time")
	command.Flags().StringVar(&config.profilerAddress, "profiler-address", config.profilerAddress, "the address to expose the pprof profiler")
	command.Flags().DurationVar(&config.resourceTerminatingTimeout, "terminating-resource-timeout", config.resourceTerminatingTimeout, "how long to wait on persistent volumes and namespaces to terminate during a restore before timing out")
	command.Flags().DurationVar(&config.defaultBackupTTL, "default-backup-ttl", config.defaultBackupTTL, "how long to wait by default before backups can be garbage collected")

	return command
}

type server struct {
	namespace             string
	metricsAddress        string
	kubeClientConfig      *rest.Config
	kubeClient            kubernetes.Interface
	veleroClient          clientset.Interface
	discoveryClient       discovery.DiscoveryInterface
	discoveryHelper       velerodiscovery.Helper
	dynamicClient         dynamic.Interface
	sharedInformerFactory informers.SharedInformerFactory
	ctx                   context.Context
	cancelFunc            context.CancelFunc
	logger                logrus.FieldLogger
	logLevel              logrus.Level
	//pluginRegistry        clientmgmt.Registry
	metrics               *metrics.ServerMetrics
	config                serverConfig
}

func (s *server) run() error {
	s.logger.Infof("data manager server is up and running")
	signals.CancelOnShutdown(s.cancelFunc, s.logger)

	//if s.config.profilerAddress != "" {
	//	go s.runProfiler()
	//}

	// Since s.namespace, which specifies where backups/restores/schedules/etc. should live,
	// *could* be different from the namespace where the Velero server pod runs, check to make
	// sure it exists, and fail fast if it doesn't.
	if err := s.namespaceExists(s.namespace); err != nil {
		return err
	}

	if err := s.initDiscoveryHelper(); err != nil {
		return err
	}

	if err := s.veleroResourcesExist(); err != nil {
		return err
	}

	if err := s.validateBackupStorageLocations(); err != nil {
		return err
	}

	if _, err := s.veleroClient.VeleroV1().BackupStorageLocations(s.namespace).Get(s.config.defaultBackupLocation, metav1.GetOptions{}); err != nil {
		s.logger.WithError(errors.WithStack(err)).
			Warnf("A backup storage location named %s has been specified for the server to use by default, but no corresponding backup storage location exists. Backups with a location not matching the default will need to explicitly specify an existing location", s.config.defaultBackupLocation)
	}

	if err := s.runControllers(s.config.defaultVolumeSnapshotLocations); err != nil {
		return err
	}

	return nil
}


func newServer(f client.Factory, config serverConfig, logger *logrus.Logger) (*server, error) {
	logger.Infof("data manager server is started")
	if config.clientQPS < 0.0 {
		return nil, errors.New("client-qps must be positive")
	}
	f.SetClientQPS(config.clientQPS)

	if config.clientBurst <= 0 {
		return nil, errors.New("client-burst must be positive")
	}
	f.SetClientBurst(config.clientBurst)

	kubeClient, err := f.KubeClient()
	if err != nil {
		return nil, err
	}

	veleroClient, err := f.Client()
	if err != nil {
		return nil, err
	}

	dynamicClient, err := f.DynamicClient()
	if err != nil {
		return nil, err
	}

	//pluginRegistry := clientmgmt.NewRegistry(config.pluginDir, logger, logger.Level)
	//if err := pluginRegistry.DiscoverPlugins(); err != nil {
	//	return nil, err
	//}

	ctx, cancelFunc := context.WithCancel(context.Background())

	clientConfig, err := f.ClientConfig()
	if err != nil {
		return nil, err
	}

	s := &server{
		namespace:             f.Namespace(),
		metricsAddress:        config.metricsAddress,
		kubeClientConfig:      clientConfig,
		kubeClient:            kubeClient,
		veleroClient:          veleroClient,
		discoveryClient:       veleroClient.Discovery(),
		dynamicClient:         dynamicClient,
		sharedInformerFactory: informers.NewSharedInformerFactoryWithOptions(veleroClient, 0, informers.WithNamespace(f.Namespace())),
		ctx:                   ctx,
		cancelFunc:            cancelFunc,
		logger:                logger,
		logLevel:              logger.Level,
		config:                config,
	}

	return s, nil
}

// namespaceExists returns nil if namespace can be successfully
// gotten from the kubernetes API, or an error otherwise.
func (s *server) namespaceExists(namespace string) error {
	s.logger.WithField("namespace", namespace).Info("Checking existence of namespace")

	if _, err := s.kubeClient.CoreV1().Namespaces().Get(namespace, metav1.GetOptions{}); err != nil {
		return errors.WithStack(err)
	}

	s.logger.WithField("namespace", namespace).Info("Namespace exists")
	return nil
}

// veleroResourcesExist checks for the existence of each Velero CRD via discovery
// and returns an error if any of them don't exist.
func (s *server) veleroResourcesExist() error {
	s.logger.Info("Checking existence of Velero custom resource definitions")

	var veleroGroupVersion *metav1.APIResourceList
	for _, gv := range s.discoveryHelper.Resources() {
		if gv.GroupVersion == api.SchemeGroupVersion.String() {
			veleroGroupVersion = gv
			break
		}
	}

	if veleroGroupVersion == nil {
		return errors.Errorf("Velero API group %s not found. Apply examples/common/00-prereqs.yaml to create it.", api.SchemeGroupVersion)
	}

	foundResources := sets.NewString()
	for _, resource := range veleroGroupVersion.APIResources {
		foundResources.Insert(resource.Kind)
	}

	var errs []error
	for kind := range api.CustomResources() {
		if foundResources.Has(kind) {
			s.logger.WithField("kind", kind).Debug("Found custom resource")
			continue
		}

		errs = append(errs, errors.Errorf("custom resource %s not found in Velero API group %s", kind, api.SchemeGroupVersion))
	}

	if len(errs) > 0 {
		errs = append(errs, errors.New("Velero custom resources not found - apply examples/common/00-prereqs.yaml to update the custom resource definitions"))
		return kubeerrs.NewAggregate(errs)
	}

	s.logger.Info("All Velero custom resource definitions exist")
	return nil
}

// validateBackupStorageLocations checks to ensure all backup storage locations exist
// and have a compatible layout, and returns an error if not.
func (s *server) validateBackupStorageLocations() error {
	s.logger.Info("Checking that all backup storage locations are valid")

	//pluginManager := clientmgmt.NewManager(s.logger, s.logLevel, s.pluginRegistry)
	//defer pluginManager.CleanupClients()

	_, err := s.veleroClient.VeleroV1().BackupStorageLocations(s.namespace).List(metav1.ListOptions{})
	if err != nil {
		return errors.WithStack(err)
	}

	//var invalid []string
	//for _, location := range locations.Items {
	//	backupStore, err := persistence.NewObjectBackupStore(&location, pluginManager, s.logger)
	//	if err != nil {
	//		invalid = append(invalid, errors.Wrapf(err, "error getting backup store for location %q", location.Name).Error())
	//		continue
	//	}
	//
	//	if err := backupStore.IsValid(); err != nil {
	//		invalid = append(invalid, errors.Wrapf(err, "backup store for location %q is invalid", location.Name).Error())
	//	}
	//}
	//
	//if len(invalid) > 0 {
	//	return errors.Errorf("some backup storage locations are invalid: %s", strings.Join(invalid, "; "))
	//}

	return nil
}

// initDiscoveryHelper instantiates the server's discovery helper and spawns a
// goroutine to call Refresh() every 5 minutes.
func (s *server) initDiscoveryHelper() error {
	discoveryHelper, err := velerodiscovery.NewHelper(s.discoveryClient, s.logger)
	if err != nil {
		return err
	}
	s.discoveryHelper = discoveryHelper

	go wait.Until(
		func() {
			if err := discoveryHelper.Refresh(); err != nil {
				s.logger.WithError(err).Error("Error refreshing discovery")
			}
		},
		5*time.Minute,
		s.ctx.Done(),
	)

	return nil
}

func (s *server) runControllers(defaultVolumeSnapshotLocations map[string]string) error {
	s.logger.Info("Starting controllers")
	return nil
}