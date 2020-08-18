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

package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/pprof"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/vmware-tanzu/astrolabe/pkg/astrolabe"
	server2 "github.com/vmware-tanzu/astrolabe/pkg/server"
	"k8s.io/client-go/util/workqueue"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/backupdriver"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/cmd"
	plugin_clientset "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned"
	backupdriver_clientset "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned/typed/backupdriver/v1"
	veleroplugin_clientset "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned/typed/veleroplugin/v1"
	pluginInformers "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/informers/externalversions"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/snapshotmgr"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/utils"
	"github.com/vmware-tanzu/velero/pkg/buildinfo"
	"github.com/vmware-tanzu/velero/pkg/client"
	"github.com/vmware-tanzu/velero/pkg/cmd/util/signals"
	"github.com/vmware-tanzu/velero/pkg/metrics"
	"github.com/vmware-tanzu/velero/pkg/util/logging"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type serverConfig struct {
	metricsAddress     string
	clientQPS          float32
	clientBurst        int
	profilerAddress    string
	formatFlag         *logging.FormatFlag
	master             string
	kubeConfig         string
	resyncPeriod       time.Duration
	workers            int
	retryIntervalStart time.Duration
	retryIntervalMax   time.Duration
	localMode          bool
}

func NewCommand(f client.Factory) *cobra.Command {
	var (
		logLevelFlag = logging.LogLevelFlag(logrus.InfoLevel)
		config       = serverConfig{
			metricsAddress:     cmd.DefaultMetricsAddress,
			clientQPS:          cmd.DefaultClientQPS,
			clientBurst:        cmd.DefaultClientBurst,
			profilerAddress:    cmd.DefaultProfilerAddress,
			formatFlag:         logging.NewFormatFlag(),
			master:             "",
			kubeConfig:         "",
			resyncPeriod:       utils.ResyncPeriod,
			workers:            cmd.DefaultBackupWorkers,
			retryIntervalStart: cmd.DefaultRetryIntervalStart,
			retryIntervalMax:   cmd.DefaultRetryIntervalMax,
			localMode:          false,
		}
	)

	var command = &cobra.Command{
		Use:    "server",
		Short:  "Run the backup-driver server",
		Long:   "Run the backup-driver server",
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

			formatter := new(logrus.TextFormatter)
			formatter.TimestampFormat = time.RFC3339
			formatter.FullTimestamp = true
			logger.SetFormatter(formatter)

			logger.Debugf("setting log-level to %s", strings.ToUpper(logLevel.String()))

			logger.Infof("Starting backup-driver server %s (%s)", buildinfo.Version, buildinfo.FormattedGitSHA())

			f.SetBasename(fmt.Sprintf("%s-%s", c.Parent().Name(), c.Name()))

			s, err := newServer(f, config, logger)
			cmd.CheckError(err)

			cmd.CheckError(s.run())
		},
	}

	// Common flags
	command.Flags().Var(logLevelFlag, "log-level", fmt.Sprintf("the level at which to log. Valid values are %s.", strings.Join(logLevelFlag.AllowedValues(), ", ")))
	command.Flags().Var(config.formatFlag, "log-format", fmt.Sprintf("the format for log output. Valid values are %s.", strings.Join(config.formatFlag.AllowedValues(), ", ")))
	command.Flags().StringVar(&config.metricsAddress, "metrics-address", config.metricsAddress, "the address to expose prometheus metrics")
	command.Flags().Float32Var(&config.clientQPS, "client-qps", config.clientQPS, "maximum number of requests per second by the server to the Kubernetes API once the burst limit has been reached")
	command.Flags().IntVar(&config.clientBurst, "client-burst", config.clientBurst, "maximum number of requests by the server to the Kubernetes API in a short period of time")
	command.Flags().StringVar(&config.profilerAddress, "profiler-address", config.profilerAddress, "the address to expose the pprof profiler")
	command.Flags().StringVar(&config.master, "master", config.master, "Master URL to build a client config from. Either this or kubeconfig needs to be set if the pod is being run out of cluster.")
	command.Flags().StringVar(&config.kubeConfig, "kubeconfig", config.kubeConfig, "Absolute path to the kubeconfig. Either this or master needs to be set if the pod is being run out of cluster.")
	command.Flags().DurationVar(&config.resyncPeriod, "resync-period", config.resyncPeriod, "Resync period for cache")
	command.Flags().IntVar(&config.workers, "backup-workers", config.workers, "Concurrency to process multiple backup requests")
	command.Flags().DurationVar(&config.retryIntervalStart, "backup-retry-int-start", config.retryIntervalStart, "Initial retry interval of failed backup request. It exponentially increases with each failure, up to retry-interval-max.")
	command.Flags().DurationVar(&config.retryIntervalMax, "backup-retry-int-max", config.retryIntervalMax, "Maximum retry interval of failed backup request.")
	command.Flags().BoolVar(&config.localMode, "local-mode", config.localMode, "Run backup driver in local mode. Optional.")

	return command
}

type server struct {
	namespace                      string
	metricsAddress                 string
	kubeClient                     kubernetes.Interface
	backupdriverClient             *backupdriver_clientset.BackupdriverV1Client
	veleropluginClient             *veleroplugin_clientset.VeleropluginV1Client
	svcBackupdriverClient          *backupdriver_clientset.BackupdriverV1Client
	svcConfig                      *rest.Config
	svcNamespace                   string
	pluginInformerFactory          pluginInformers.SharedInformerFactory
	kubeInformerFactory            kubeinformers.SharedInformerFactory
	svcKubeInformerFactory         kubeinformers.SharedInformerFactory
	svcBackupdriverInformerFactory pluginInformers.SharedInformerFactory
	ctx                            context.Context
	cancelFunc                     context.CancelFunc
	logger                         logrus.FieldLogger
	logLevel                       logrus.Level
	metrics                        *metrics.ServerMetrics
	config                         serverConfig
	snapManager                    *snapshotmgr.SnapshotManager
}

func (s *server) run() error {
	s.logger.Infof("backup-driver server is up and running")
	signals.CancelOnShutdown(s.cancelFunc, s.logger)

	if s.config.profilerAddress != "" {
		go s.runProfiler()
	}

	// Since s.namespace, which specifies where backups/restores/schedules/etc. should live,
	// *could* be different from the namespace where the Velero server pod runs, check to make
	// sure it exists, and fail fast if it doesn't.
	if err := s.namespaceExists(s.namespace); err != nil {
		return err
	}

	if err := s.runControllers(); err != nil {
		return err
	}

	return nil
}

func newServer(f client.Factory, config serverConfig, logger *logrus.Logger) (*server, error) {
	logger.Infof("backup-driver server is started")

	clientConfig, err := cmd.BuildConfig(config.master, config.kubeConfig, f)
	if err != nil {
		logger.Errorf("Failed to get client config")
		return nil, err
	}

	// kubeClient is the client to the current cluster
	// (vanilla, guest, or supervisor)
	kubeClient, err := kubernetes.NewForConfig(clientConfig)
	if err != nil {
		logger.Errorf("Failed to get client to the current kubernetes cluster")
		return nil, err
	}

	pluginClient, err := plugin_clientset.NewForConfig(clientConfig)
	if err != nil {
		logger.Errorf("Failed to get the plugin client for the current kubernetes cluster")
		return nil, err
	}

	backupdriverClient, err := backupdriver_clientset.NewForConfig(clientConfig)
	if err != nil {
		logger.Errorf("Failed to get the backupdriver client for the current kubernetes cluster")
		return nil, err
	}

	veleropluginClient, err := veleroplugin_clientset.NewForConfig(clientConfig)
	if err != nil {
		logger.Errorf("Failed to get the veleroplugin client for the current kubernetes cluster")
		return nil, err
	}

	// backup driver watches all namespaces so do not specify any one
	backupdriverInformerFactory := pluginInformers.NewSharedInformerFactoryWithOptions(pluginClient, config.resyncPeriod)
	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeClient, config.resyncPeriod)

	// Set the ProtectedEntity configuration
	pvcConfig := make(map[string]interface{})
	pvcConfig["restConfig"] = clientConfig

	// Set snapshot manager configuration information
	snapshotMgrConfig := make(map[string]string)
	snapshotMgrConfig[utils.VolumeSnapshotterManagerLocation] = utils.VolumeSnapshotterDataServer
	snapshotMgrConfig[utils.VolumeSnapshotterLocalMode] = strconv.FormatBool(config.localMode)

	ctx, cancelFunc := context.WithCancel(context.Background())
	// If CLUSTER_FLAVOR is GUEST_CLUSTER, set up svcKubeConfig to communicate with the Supervisor Cluster
	clusterFlavor, _ := utils.GetClusterFlavor(clientConfig)
	var svcConfig *rest.Config
	var svcBackupdriverClient *backupdriver_clientset.BackupdriverV1Client
	var svcKubeInformerFactory kubeinformers.SharedInformerFactory
	var svcBackupdriverInformerFactory pluginInformers.SharedInformerFactory
	var svcNamespace string
	if clusterFlavor == utils.TkgGuest {
		svcConfig, svcNamespace, err = utils.SupervisorConfig(logger)
		if err != nil {
			logger.Error("Failed to get the supervisor config for the guest kubernetes cluster")
			return nil, err
		}
		logger.Infof("Supervisor Namespace: %s", svcNamespace)

		// Create informers to watch supervisor CRs in the guest-cluster namespace
		svcBackupdriverClient, err = backupdriver_clientset.NewForConfig(svcConfig)
		if err != nil {
			logger.Error("Failed to get the supervisor backupdriver client for the guest kubernetes cluster")
			return nil, err
		}

		svcPluginClient, err := plugin_clientset.NewForConfig(svcConfig)
		if err != nil {
			logger.Errorf("Failed to get the plugin client for the supervisor cluster")
			return nil, err
		}
		svcBackupdriverInformerFactory = pluginInformers.NewSharedInformerFactoryWithOptions(svcPluginClient, config.resyncPeriod,
			pluginInformers.WithNamespace(svcNamespace))

		svcKubeClient, err := kubernetes.NewForConfig(svcConfig)
		if err != nil {
			logger.Errorf("Failed to get the kubernetes client for the supervisor cluster")
			return nil, err
		}
		svcKubeInformerFactory = kubeinformers.NewSharedInformerFactoryWithOptions(svcKubeClient, config.resyncPeriod,
			kubeinformers.WithNamespace(svcNamespace))

		// Set snapshot manager params
		pvcConfig["svcConfig"] = svcConfig
		pvcConfig["svcNamespace"] = svcNamespace

		// Set the mode to local as the data movement job is assigned to the supervisor cluster
		snapshotMgrConfig[utils.VolumeSnapshotterLocalMode] = "true"

		// Log supervisor namespace annotations
		if params, err := utils.GetSupervisorParameters(svcConfig, svcNamespace, logger); err != nil {
			logger.WithError(err).Warn("Failed to get supervisor parameters")
		} else {
			logger.Infof("Supervisor parameters: %v", params)
		}
	}

	peConfigs := make(map[string]map[string]interface{})
	peConfigs[astrolabe.PvcPEType] = pvcConfig

	// Initialize dummy s3 config.
	s3Config := astrolabe.S3Config{
		URLBase: "VOID_URL",
	}

	s3RepoParams := make(map[string]interface{})
	configInfo := server2.NewConfigInfo(peConfigs, s3Config)
	snapshotmgr, err := snapshotmgr.NewSnapshotManagerFromConfig(configInfo, s3RepoParams, snapshotMgrConfig,
		clientConfig, logger)
	if err != nil {
		return nil, err
	}

	s := &server{
		namespace:                      f.Namespace(),
		metricsAddress:                 config.metricsAddress,
		kubeClient:                     kubeClient,
		backupdriverClient:             backupdriverClient,
		veleropluginClient:             veleropluginClient,
		svcBackupdriverClient:          svcBackupdriverClient,
		svcConfig:                      svcConfig,
		svcNamespace:                   svcNamespace,
		pluginInformerFactory:          backupdriverInformerFactory,
		kubeInformerFactory:            kubeInformerFactory,
		svcKubeInformerFactory:         svcKubeInformerFactory,
		svcBackupdriverInformerFactory: svcBackupdriverInformerFactory,
		ctx:                            ctx,
		cancelFunc:                     cancelFunc,
		logger:                         logger,
		logLevel:                       logger.Level,
		config:                         config,
		snapManager:                    snapshotmgr,
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

func (s *server) runProfiler() {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	if err := http.ListenAndServe(s.config.profilerAddress, mux); err != nil {
		s.logger.WithError(errors.WithStack(err)).Error("error running profiler http server")
	}
}

func (s *server) runControllers() error {
	s.logger.Info("Starting backup-driver controllers")

	ctx := s.ctx
	var wg sync.WaitGroup
	// Register controllers
	s.logger.Info("Registering controllers")

	backupDriverController := backupdriver.NewBackupDriverController(
		"BackupDriverController",
		s.logger,
		s.backupdriverClient,
		s.veleropluginClient,
		s.svcBackupdriverClient,
		s.svcConfig,
		s.svcNamespace,
		s.config.resyncPeriod,
		s.kubeInformerFactory,
		s.pluginInformerFactory,
		s.svcKubeInformerFactory,
		s.svcBackupdriverInformerFactory,
		s.config.localMode,
		s.snapManager,
		workqueue.NewItemExponentialFailureRateLimiter(s.config.retryIntervalStart, s.config.retryIntervalMax))

	wg.Add(1)
	go func() {
		defer wg.Done()
		backupDriverController.Run(s.ctx, s.config.workers)
	}()

	// SHARED INFORMERS HAVE TO BE STARTED AFTER ALL CONTROLLERS
	go s.pluginInformerFactory.Start(ctx.Done())
	go s.kubeInformerFactory.Start(ctx.Done())
	if s.svcConfig != nil {
		go s.svcKubeInformerFactory.Start(ctx.Done())
		go s.svcBackupdriverInformerFactory.Start(ctx.Done())
	}

	s.logger.Info("Server started successfully")

	<-ctx.Done()

	s.logger.Info("Waiting for all controllers to shut down gracefully")
	wg.Wait()

	return nil
}
