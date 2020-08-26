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
	"github.com/vmware-tanzu/astrolabe/pkg/ivd"
	astrolabeServer "github.com/vmware-tanzu/astrolabe/pkg/server"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/cmd"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/controller"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/dataMover"
	plugin_clientset "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned"
	pluginInformers "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/informers/externalversions"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/snapshotmgr"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/utils"
	"github.com/vmware-tanzu/velero/pkg/buildinfo"
	"github.com/vmware-tanzu/velero/pkg/client"
	"github.com/vmware-tanzu/velero/pkg/cmd/util/signals"
	velero_clientset "github.com/vmware-tanzu/velero/pkg/generated/clientset/versioned"
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
	vCenter            string
	port               string
	user               string
	clusterId          string
	insecureFlag       bool
	vcConfigFromSecret bool
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
			port:               utils.DefaultVCenterPort,
			insecureFlag:       cmd.DefaultInsecureFlag,
			vcConfigFromSecret: cmd.DefaultVCConfigFromSecret,
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

			formatter := new(logrus.TextFormatter)
			formatter.TimestampFormat = time.RFC3339
			formatter.FullTimestamp = true
			logger.SetFormatter(formatter)

			logger.Debugf("setting log-level to %s", strings.ToUpper(logLevel.String()))

			logger.Infof("Starting data manager server %s (%s)", buildinfo.Version, buildinfo.FormattedGitSHA())

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
	command.Flags().StringVar(&config.vCenter, "vcenter-address", config.vCenter, "VirtualCenter address. If specified, --use-secret should be set to False.")
	command.Flags().StringVar(&config.port, "vcenter-port", config.port, "VirtualCenter port. If specified, --use-secret should be set to False.")
	command.Flags().StringVar(&config.user, "vcenter-user", config.user, "VirtualCenter user. If specified, --use-secret should be set to False.")
	command.Flags().StringVar(&config.clusterId, "cluster-id", config.clusterId, "kubernetes cluster id. If specified, --use-secret should be set to False.")
	command.Flags().BoolVar(&config.insecureFlag, "insecure-Flag", config.insecureFlag, "insecure flag. If specified, --use-secret should be set to False.")
	command.Flags().BoolVar(&config.vcConfigFromSecret, "use-secret", config.vcConfigFromSecret, "retrieve VirtualCenter configuration from secret")

	return command
}

type server struct {
	namespace             string
	metricsAddress        string
	kubeClientConfig      *rest.Config
	kubeClient            kubernetes.Interface
	veleroClient          velero_clientset.Interface
	pluginClient          plugin_clientset.Interface
	pluginInformerFactory pluginInformers.SharedInformerFactory
	kubeInformerFactory   kubeinformers.SharedInformerFactory
	ctx                   context.Context
	cancelFunc            context.CancelFunc
	logger                logrus.FieldLogger
	logLevel              logrus.Level
	metrics               *metrics.ServerMetrics
	config                serverConfig
	dataMover             *dataMover.DataMover
	snapManager           *snapshotmgr.SnapshotManager
	externalDataMgr       bool
}

func (s *server) run() error {
	s.ctx, s.cancelFunc = context.WithCancel(context.Background())
	defer s.cancelFunc()	// We shouldn't exit until everything is shutdown anyhow, but this ensures there are no leaks
	s.logger.Infof("data manager server is up and running")
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

// Assign vSphere VC credentials and configuration parameters
func getVCConfigParams(config serverConfig, params map[string]interface{}, logger logrus.FieldLogger) error {

	if config.vCenter == "" {
		return errors.New("getVCConfigParams: parameter vcenter-address not provided")
	}
	params[ivd.HostVcParamKey] = config.vCenter

	if config.user == "" {
		return errors.New("getVCConfigParams: parameter vcenter-user not provided")
	}
	params[ivd.UserVcParamKey] = config.user

	passwd := os.Getenv("VC_PASSWORD")
	if passwd == "" {
		logger.Warnf("getVCConfigParams: Environment variable VC_PASSWORD not set or empty")
	}
	params[ivd.PasswordVcParamKey] = passwd

	if config.clusterId == "" {
		return errors.New("getVCConfigParams: parameter vcenter-user not provided")
	}
	params[ivd.ClusterVcParamKey] = config.clusterId

	// Below vc configuration params are optional
	params[ivd.PortVcParamKey] = config.port
	params[ivd.InsecureFlagVcParamKey] = strconv.FormatBool(config.insecureFlag)

	return nil
}

func newServer(f client.Factory, config serverConfig, logger *logrus.Logger) (*server, error) {
	logger.Infof("data manager server is started")
	kubeClient, err := f.KubeClient()
	if err != nil {
		return nil, err
	}

	veleroClient, err := f.Client()
	if err != nil {
		return nil, err
	}

	clientConfig, err := f.ClientConfig()
	if err != nil {
		return nil, err
	}

	pluginClient, err := plugin_clientset.NewForConfig(clientConfig)
	if err != nil {
		return nil, err
	}
	
	ivdParams := make(map[string]interface{})
	if config.vcConfigFromSecret == true {
		logger.Infof("VC Configuration will be retrieved from cluster secret")
	} else {
		err := getVCConfigParams(config, ivdParams, logger)
		if err != nil {
			return nil, err
		}

		logger.Infof("VC configuration provided by user for :%s", ivdParams[ivd.HostVcParamKey])
	}

	snapshotMgrConfig := make(map[string]string)
	snapshotMgrConfig[utils.VolumeSnapshotterManagerLocation] = utils.VolumeSnapshotterDataServer

	peConfigs := make(map[string]map[string]interface{})
	peConfigs["ivd"] = ivdParams // Even an empty map here will force NewSnapshotManagerFromConfig to use the default VC config

	// Initialize dummy s3 config.
	s3Config := astrolabe.S3Config{
		URLBase: "VOID_URL",
	}
	s3RepoParams := make(map[string]interface{})

	configInfo := astrolabeServer.NewConfigInfo(peConfigs, s3Config)
	snapshotmgr, err := snapshotmgr.NewSnapshotManagerFromConfig(configInfo, s3RepoParams, snapshotMgrConfig,
		clientConfig, logger)
	if err != nil {
		return nil, err
	}

	clusterFlavor, err := utils.GetClusterFlavor(clientConfig)
	if err != nil {
		logger.WithError(err).Error("Failed tp identify cluster flavor")
		return nil, err
	}
	// In supervisor/guest cluster, data manager is remote
	externalDataMgr := false
	if clusterFlavor != utils.VSphere {
		externalDataMgr = true
	}

	dataMover, err := dataMover.NewDataMoverFromCluster(ivdParams, externalDataMgr, logger)
	if err != nil {
		return nil, err
	}

	s := &server{
		namespace:             f.Namespace(),
		metricsAddress:        config.metricsAddress,
		kubeClient:            kubeClient,
		veleroClient:          veleroClient,
		pluginClient:          pluginClient,
		pluginInformerFactory: pluginInformers.NewSharedInformerFactoryWithOptions(pluginClient, utils.ResyncPeriod, pluginInformers.WithNamespace(f.Namespace())),
		kubeInformerFactory:   kubeinformers.NewSharedInformerFactory(kubeClient, 0),
		logger:                logger,
		logLevel:              logger.Level,
		config:                config,
		dataMover:             dataMover,
		snapManager:           snapshotmgr,
		externalDataMgr:       externalDataMgr,
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
	s.logger.Info("Starting data manager controllers")

	ctx := s.ctx
	var wg sync.WaitGroup
	// Register controllers
	s.logger.Info("Registering controllers")

	uploadController := controller.NewUploadController(
		s.logger,
		s.pluginInformerFactory.Veleroplugin().V1().Uploads(),
		s.pluginClient.VeleropluginV1(),
		s.kubeClient,
		s.dataMover,
		s.snapManager,
		os.Getenv("NODE_NAME"),
		s.externalDataMgr,
	)

	downloadController := controller.NewDownloadController(
		s.logger,
		s.pluginInformerFactory.Veleroplugin().V1().Downloads(),
		s.pluginClient.VeleropluginV1(),
		s.kubeClient,
		s.dataMover,
		os.Getenv("NODE_NAME"),
	)

	wg.Add(1)
	go func() {
		defer wg.Done()
		uploadController.Run(s.ctx, 1)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		downloadController.Run(s.ctx, 1)
	}()

	// SHARED INFORMERS HAVE TO BE STARTED AFTER ALL CONTROLLERS
	go s.pluginInformerFactory.Start(ctx.Done())
	go s.kubeInformerFactory.Start(ctx.Done())

	s.logger.Info("Server started successfully")

	<-ctx.Done()

	s.logger.Info("Waiting for all controllers to shut down gracefully")
	wg.Wait()

	return nil
}
