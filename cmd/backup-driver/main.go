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

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/backupdriver"
	backupdriverClientset "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned"
	backupdriverInformers "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/informers/externalversions"
	"github.com/vmware-tanzu/velero/pkg/util/logging"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/workqueue"
)

var (
	master             = flag.String("master", "", "Master URL to build a client config from. Either this or kubeconfig needs to be set if the backupdriver is being run out of cluster.")
	kubeConfig         = flag.String("kubeconfig", "", "Absolute path to the kubeconfig")
	resyncPeriod       = flag.Duration("resync-period", time.Minute*10, "Resync period for cache")
	workers            = flag.Int("workers", 1, "Concurrency to process multiple backup requests")
	retryIntervalStart = flag.Duration("retry-interval-start", time.Second, "Initial retry interval of failed backup request. It exponentially increases with each failure, up to retry-interval-max.")
	retryIntervalMax   = flag.Duration("retry-interval-max", 5*time.Minute, "Maximum retry interval of failed backup request.")

	showVersion = flag.Bool("version", false, "Show version")

	version = "unknown"
)

func main() {
	flag.Set("logtostderr", "true")
	flag.Parse()

	if *showVersion {
		fmt.Println(os.Args[0], version)
		os.Exit(0)
	}

	logLevelFlag := logging.LogLevelFlag(logrus.InfoLevel)
	formatFlag := logging.NewFormatFlag()
	// go-plugin uses log.Println to log when it's waiting for all plugin processes to complete so we need to
	// set its output to stdout.
	log.SetOutput(os.Stdout)

	logLevel := logLevelFlag.Parse()
	format := formatFlag.Parse()

	// Make sure we log to stdout so cloud log dashboards don't show this as an error.
	logrus.SetOutput(os.Stdout)

	// Velero's DefaultLogger logs to stdout, so all is good there.
	logger := logging.DefaultLogger(logLevel, format)

	formatter := new(logrus.TextFormatter)
	formatter.TimestampFormat = time.RFC3339
	formatter.FullTimestamp = true
	logger.SetFormatter(formatter)

	logger.Debugf("setting log-level to %s", strings.ToUpper(logLevel.String()))

	config, err := buildConfig(*master, *kubeConfig)
	if err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}

	// kubeClient is the client to the current cluster
	// (vanilla, guest, or supervisor)
	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		logger.Errorf("Error building kube client: %s", err.Error())
		os.Exit(1)
	}

	backupdriverClient, err := backupdriverClientset.NewForConfig(config)
	if err != nil {
		logger.Errorf("Error building backupdriver clientset: %s", err.Error())
		os.Exit(1)
	}

	informerFactory := informers.NewSharedInformerFactory(kubeClient, *resyncPeriod)
	backupdriverInformerFactory := backupdriverInformers.NewSharedInformerFactory(backupdriverClient, *resyncPeriod)

	// TODO: If CLUSTER_FLAVOR is GUEST_CLUSTER, set up svcKubeClient to communicate with the Supervisor Cluster
	// If CLUSTER_FLAVOR is WORKLOAD, it is a Supervisor Cluster.
	// By default we are in the Vanilla Cluster
	rc := backupdriver.NewBackupDriverController("BackupDriverController", logger, kubeClient, *resyncPeriod, informerFactory, backupdriverInformerFactory, workqueue.NewItemExponentialFailureRateLimiter(*retryIntervalStart, *retryIntervalMax))
	run := func(ctx context.Context) {
		stopCh := make(chan struct{})
		informerFactory.Start(stopCh)
		backupdriverInformerFactory.Start(stopCh)
		go rc.Run(ctx, *workers)

		// ...until SIGINT
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		<-c
		close(stopCh)
	}

	run(context.TODO())
}

func buildConfig(master, kubeConfig string) (*rest.Config, error) {
	var config *rest.Config
	var err error
	if master != "" || kubeConfig != "" {
		config, err = clientcmd.BuildConfigFromFlags(master, kubeConfig)
	} else {
		config, err = rest.InClusterConfig()
	}
	if err != nil {
		return nil, errors.Errorf("failed to create config: %v", err)
	}
	return config, nil
}
