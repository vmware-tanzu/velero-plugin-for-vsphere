package server

import (
	"context"
	"fmt"
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
	"log"
	"net/http"
	"net/http/pprof"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	// the port where prometheus metrics are exposed
	defaultMetricsAddress = ":8085"
	// server's client default qps and burst
	defaultClientQPS   float32 = 20.0
	defaultClientBurst int     = 30

	defaultProfilerAddress = "localhost:6060"

	defaultControllerWorkers = 1
	// the default TTL for a backup
	//defaultBackupTTL = 30 * 24 * time.Hour
)

type serverConfig struct {
	metricsAddress                        									string
	clientQPS                                                               float32
	clientBurst                                                             int
	profilerAddress                                                         string
	formatFlag                                                              *logging.FormatFlag
}

func NewCommand(f client.Factory) *cobra.Command {
	var (
		logLevelFlag            = logging.LogLevelFlag(logrus.InfoLevel)
		config                  = serverConfig{
			metricsAddress:                    defaultMetricsAddress,
			clientQPS:                         defaultClientQPS,
			clientBurst:                       defaultClientBurst,
			profilerAddress:                   defaultProfilerAddress,
			formatFlag:                        logging.NewFormatFlag(),
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

			logger.Infof("setting log-level to %s", strings.ToUpper(logLevel.String()))

			logger.Infof("Starting data manager server %s (%s)", buildinfo.Version, buildinfo.FormattedGitSHA())

			f.SetBasename(fmt.Sprintf("%s-%s", c.Parent().Name(), c.Name()))

			s, err := newServer(f, config, logger)
			cmd.CheckError(err)

			cmd.CheckError(s.run())
		},
	}

	command.Flags().Var(logLevelFlag, "log-level", fmt.Sprintf("the level at which to log. Valid values are %s.", strings.Join(logLevelFlag.AllowedValues(), ", ")))
	command.Flags().Var(config.formatFlag, "log-format", fmt.Sprintf("the format for log output. Valid values are %s.", strings.Join(config.formatFlag.AllowedValues(), ", ")))
	command.Flags().StringVar(&config.metricsAddress, "metrics-address", config.metricsAddress, "the address to expose prometheus metrics")
	command.Flags().Float32Var(&config.clientQPS, "client-qps", config.clientQPS, "maximum number of requests per second by the server to the Kubernetes API once the burst limit has been reached")
	command.Flags().IntVar(&config.clientBurst, "client-burst", config.clientBurst, "maximum number of requests by the server to the Kubernetes API in a short period of time")
	command.Flags().StringVar(&config.profilerAddress, "profiler-address", config.profilerAddress, "the address to expose the pprof profiler")

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
	snapManager               *snapshotmgr.SnapshotManager
}

func (s *server) run() error {
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

	dataMover, err := dataMover.NewDataMoverFromCluster(logger)
	if err != nil {
		return nil, err
	}

	snapshotMgrConfig := make(map[string]string)
	snapshotMgrConfig[utils.VolumeSnapshotterManagerLocation] = utils.VolumeSnapshotterDataServer
	snapshotmgr, err := snapshotmgr.NewSnapshotManagerFromCluster(snapshotMgrConfig, logger)
	if err != nil {
		return nil, err
	}

	ctx, cancelFunc := context.WithCancel(context.Background())

	s := &server{
		namespace:             f.Namespace(),
		metricsAddress:        config.metricsAddress,
		kubeClient:            kubeClient,
		veleroClient:          veleroClient,
		pluginClient:          pluginClient,
		pluginInformerFactory: pluginInformers.NewSharedInformerFactoryWithOptions(pluginClient, utils.ResyncPeriod, pluginInformers.WithNamespace(f.Namespace())),
		kubeInformerFactory:   kubeinformers.NewSharedInformerFactory(kubeClient, 0),
		ctx:                   ctx,
		cancelFunc:            cancelFunc,
		logger:                logger,
		logLevel:              logger.Level,
		config:                config,
		dataMover:             dataMover,
		snapManager:           snapshotmgr,
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
		s.kubeInformerFactory.Core().V1().PersistentVolumeClaims(),
		s.kubeInformerFactory.Core().V1().PersistentVolumes(),
		s.dataMover,
		s.snapManager,
		os.Getenv("NODE_NAME"),
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

