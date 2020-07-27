package paravirt

import (
	"context"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/vmware-tanzu/astrolabe/pkg/astrolabe"
	"github.com/vmware-tanzu/astrolabe/pkg/server"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	CACRTKEY     = "ca.crt"
	NAMESPACEKEY = "namespace"
	TOKENKEY     = "token"
	CSICONFKEY   = "cns-csi.conf"
	ENDPOINTKEY  = "endpoint"
	PORTKEY      = "port"
)

// Testbed specific setup

const (
	CSINAMESPACE     = "vmware-system-csi"
	CSICONFIGMAPNAME = "pvcsi-config"
	TARGETNAMESPACE  = "velero"
	TARGETSECRETNAME = "pvbackupdriver-provider-creds"
)

func getSupervisorConfig(gcClientSet *kubernetes.Clientset, secretDataMap map[string][]byte, svcMasterIP string, logger logrus.FieldLogger) (*rest.Config, error) {
	token := string(secretDataMap[TOKENKEY])

	tlsClientConfig := rest.TLSClientConfig{
		CAData: secretDataMap[CACRTKEY],
	}

	// retrieve api endpoint and port from pvCSI configmap in vsphere CSI namespace
	csiConfigmapParams := make(map[string]string)
	err := parseCSIConfigmapData(gcClientSet, CSINAMESPACE, CSICONFIGMAPNAME, csiConfigmapParams)
	if err != nil {
		logger.Errorf("Failed to parse CSI configmap data: %v", err)
		return nil, errors.WithStack(err)
	}

	_, endpointExists := csiConfigmapParams[ENDPOINTKEY]
	pvPort, portExists := csiConfigmapParams[PORTKEY]
	if !endpointExists || !portExists {
		errMsg := "Either endpoint or port is not available in pvCSI configmap"
		logger.Error(errMsg)
		return nil, errors.New(errMsg)
	}

	return &rest.Config{
		// TODO: change the hardcoded IP to hostname from cluster DNS
		Host:            "https://" + net.JoinHostPort(svcMasterIP, pvPort),
		TLSClientConfig: tlsClientConfig,
		BearerToken:     string(token),
	}, nil
}

func parseCSIConfigmapData(gcClientSet *kubernetes.Clientset, csiNamespace, csiConfigmapName string, params map[string]string) error {
	csiConfigmap, err := gcClientSet.CoreV1().ConfigMaps(csiNamespace).Get(csiConfigmapName, metav1.GetOptions{})
	if err != nil {
		return errors.Wrap(err, "Failed to get the pvCSI config map as expected")
	}

	csiConfData, ok := csiConfigmap.Data[CSICONFKEY]
	if !ok {
		return errors.Errorf("Failed to get the expected key %v from CSI configmap %v in the namespace %v", CSICONFKEY, csiConfigmapName, csiNamespace)
	}

	lines := strings.Split(csiConfData, "\n")
	for _, line := range lines {
		if strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			unquotedValue, err := strconv.Unquote(string(value))
			if err != nil {
				continue
			}
			params[key] = unquotedValue
		}
	}

	return nil
}

func TestGetProtectedEntity(t *testing.T) {
	svcMasterIP, ok := os.LookupEnv("SVCMASTERIP")
	if !ok {
		t.Skip("The ENV, SVCMASTERIP, is not set")
	}

	// In this case, KUBECONFIG is required to point to a kubeconfig file of a guest cluster in vSphere with kubernetes
	path := os.Getenv("KUBECONFIG")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("The KubeConfig file, %v, is not exist", path)
	}

	gcRestConfig, err := clientcmd.BuildConfigFromFlags("", path)
	if err != nil {
		t.Skipf("Failed to build k8s gcRestConfig from kubeconfig file: %+v ", err)
	}

	gcClientSet, err := kubernetes.NewForConfig(gcRestConfig)
	if err != nil {
		t.Skipf("Failed to get kubernetes clientSet from the given gcRestConfig: %v from guest cluster", gcRestConfig)
	}

	// Prerequisite checks on the availability of GCM credential via the providerserviceaccount CRD
	gcmSecret, err := gcClientSet.CoreV1().Secrets(TARGETNAMESPACE).Get(TARGETSECRETNAME, metav1.GetOptions{})
	if err != nil {
		t.Skipf("Failed to get the required gcm credential: %v", err)
	}

	logger := logrus.New()
	formatter := new(logrus.TextFormatter)
	formatter.TimestampFormat = time.RFC3339Nano
	formatter.FullTimestamp = true
	logger.SetFormatter(formatter)
	logger.SetLevel(logrus.DebugLevel)

	svcRestConfig, err := getSupervisorConfig(gcClientSet, gcmSecret.Data, svcMasterIP, logger)
	if err != nil {
		t.Skipf("Failed to get the rest gcRestConfig for supervisor cluster by parsing GCM credential in guest cluster: %v", err)
	}
	svcNamespace := string(gcmSecret.Data[NAMESPACEKEY])

	// Initialize the Astrolabe Paravirt ProtectedEntity Manager
	paravirtParams := make(map[string]interface{})
	paravirtParams["restConfig"] = gcRestConfig
	paravirtParams["entityType"] = ParaVirtEntityTypePersistentVolume
	paravirtParams["svcConfig"] = svcRestConfig
	paravirtParams["svcNamespace"] = svcNamespace
	paraVirtPETM, err := NewParaVirtProtectedEntityTypeManagerFromConfig(paravirtParams, astrolabe.S3Config{
		URLBase: "VOID_URL",
	}, logger)
	if err != nil {
		t.Fatalf("Failed to init para virt PETM: %v", err)
	}

	ctx := context.Background()
	peids, err := paraVirtPETM.GetProtectedEntities(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, curPEID := range peids {
		logger.Infof("curPEID = %v", curPEID.String())
		curPE, err := paraVirtPETM.GetProtectedEntity(ctx, curPEID)
		if err != nil {
			logger.Errorf("Failed to get Paravirt PE with PEID = %v", curPEID.String())
			t.Fatal(err)
		}
		curIPE, err := curPE.GetInfo(ctx)
		if err != nil {
			logger.Errorf("Failed to get PE Info of Paravirt PE with PEID = %v", curPEID.String())
			t.Fatal(err)
		}
		logger.Infof("curPEName = %v", curIPE.GetName())
	}
}

func TestSnapshotOps(t *testing.T) {
	svcMasterIP, ok := os.LookupEnv("SVCMASTERIP")
	if !ok {
		t.Skip("The ENV, SVCMASTERIP, is not set")
	}

	// In this case, KUBECONFIG is required to point to a kubeconfig file of a guest cluster in vSphere with kubernetes
	path := os.Getenv("KUBECONFIG")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("The KubeConfig file, %v, is not exist", path)
	}

	gcRestConfig, err := clientcmd.BuildConfigFromFlags("", path)
	if err != nil {
		t.Skipf("Failed to build k8s gcRestConfig from kubeconfig file: %+v ", err)
	}

	gcClientSet, err := kubernetes.NewForConfig(gcRestConfig)
	if err != nil {
		t.Skipf("Failed to get kubernetes clientSet from the given gcRestConfig, %v, from guest cluster: %v", gcRestConfig, err)
	}

	// Prerequisite checks on the availability of GCM credential via the providerserviceaccount CRD
	gcmSecret, err := gcClientSet.CoreV1().Secrets(TARGETNAMESPACE).Get(TARGETSECRETNAME, metav1.GetOptions{})
	if err != nil {
		t.Skipf("Failed to get the required gcm credential: %v", err)
	}

	logger := logrus.New()
	formatter := new(logrus.TextFormatter)
	formatter.TimestampFormat = time.RFC3339Nano
	formatter.FullTimestamp = true
	logger.SetFormatter(formatter)
	logger.SetLevel(logrus.DebugLevel)

	svcRestConfig, err := getSupervisorConfig(gcClientSet, gcmSecret.Data, svcMasterIP, logger)
	if err != nil {
		t.Skipf("Failed to get the rest gcRestConfig for supervisor cluster by parsing GCM credential in guest cluster: %v", err)
	}
	svcNamespace := string(gcmSecret.Data[NAMESPACEKEY])

	// Initialize the Astrolabe Paravirt ProtectedEntity Manager
	paravirtParams := make(map[string]interface{})
	paravirtParams["restConfig"] = gcRestConfig
	paravirtParams["entityType"] = ParaVirtEntityTypePersistentVolume
	paravirtParams["svcConfig"] = svcRestConfig
	paravirtParams["svcNamespace"] = svcNamespace
	paraVirtPETM, err := NewParaVirtProtectedEntityTypeManagerFromConfig(paravirtParams, astrolabe.S3Config{
		URLBase: "VOID_URL",
	}, logger)
	if err != nil {
		t.Fatalf("Failed to init para virt PETM: %v", err)
	}

	ctx := context.Background()
	peids, err := paraVirtPETM.GetProtectedEntities(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(peids) <= 0 {
		t.Skip("No Paravirt PEs can be found in the cluster")
	}

	selectedPEID := peids[0]
	logger.Infof("Picked up the first available PVC PE, %v", selectedPEID.String())

	paravirtPE, err := paraVirtPETM.GetProtectedEntity(ctx, selectedPEID)
	if err != nil {
		logger.Errorf("Failed to get PVC PE with PEID = %v", selectedPEID.String())
		t.Fatal(err)
	}

	logger.Infof("Snapshotting the PVC PE, %v", selectedPEID.String())
	snapshotID, err := paravirtPE.Snapshot(ctx, make(map[string]map[string]interface{}))
	if err != nil {
		logger.Errorf("Failed to snapshot PVC PE with PEID = %v: %v", paravirtPE.GetID().String(), err)
		t.Fatal(err)
	}
	logger.Infof("snapshotID = %v", snapshotID.String())

	logger.Info("Test completed")
}

func TestPVCSnapshotOps(t *testing.T) {
	svcMasterIP, ok := os.LookupEnv("SVCMASTERIP")
	if !ok {
		t.Skip("The ENV, SVCMASTERIP, is not set")
	}

	// In this case, KUBECONFIG is required to point to a kubeconfig file of a guest cluster in vSphere with kubernetes
	path := os.Getenv("KUBECONFIG")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("The KubeConfig file, %v, is not exist", path)
	}

	gcRestConfig, err := clientcmd.BuildConfigFromFlags("", path)
	if err != nil {
		t.Skipf("Failed to build k8s gcRestConfig from kubeconfig file: %+v ", err)
	}

	gcClientSet, err := kubernetes.NewForConfig(gcRestConfig)
	if err != nil {
		t.Skipf("Failed to get kubernetes clientSet from the given gcRestConfig, %v, from guest cluster: %v", gcRestConfig, err)
	}

	// Prerequisite checks on the availability of GCM credential via the providerserviceaccount CRD
	gcmSecret, err := gcClientSet.CoreV1().Secrets(TARGETNAMESPACE).Get(TARGETSECRETNAME, metav1.GetOptions{})
	if err != nil {
		t.Skipf("Failed to get the required gcm credential: %v", err)
	}

	logger := logrus.New()
	formatter := new(logrus.TextFormatter)
	formatter.TimestampFormat = time.RFC3339Nano
	formatter.FullTimestamp = true
	logger.SetFormatter(formatter)
	logger.SetLevel(logrus.DebugLevel)

	svcRestConfig, err := getSupervisorConfig(gcClientSet, gcmSecret.Data, svcMasterIP, logger)
	if err != nil {
		t.Skipf("Failed to get the rest gcRestConfig for supervisor cluster by parsing GCM credential in guest cluster: %v", err)
	}
	svcNamespace := string(gcmSecret.Data[NAMESPACEKEY])

	// Initialize the Astrolabe Direct ProtectedEntity Manager
	pvcParams := make(map[string]interface{})
	pvcParams["restConfig"] = gcRestConfig
	pvcParams["svcNamespace"] = svcNamespace

	configParams := make(map[string]map[string]interface{})
	configParams["pvc"] = pvcParams

	configInfo := server.NewConfigInfo(configParams, astrolabe.S3Config{
		URLBase: "VOID_URL",
	})

	pem := server.NewDirectProtectedEntityManagerFromParamMap(configInfo, logger)

	// Initialize the External Astrolabe ProtectedEntity Type Manager for paravirtualized entities
	paravirtParams := make(map[string]interface{})
	paravirtParams["restConfig"] = gcRestConfig
	paravirtParams["entityType"] = ParaVirtEntityTypePersistentVolume
	paravirtParams["svcConfig"] = svcRestConfig
	paravirtParams["svcNamespace"] = svcNamespace
	paraVirtPETM, err := NewParaVirtProtectedEntityTypeManagerFromConfig(paravirtParams, astrolabe.S3Config{
		URLBase: "VOID_URL",
	}, logger)
	if err != nil {
		t.Fatalf("Failed to init para virt PETM: %v", err)
	}

	pem.RegisterExternalProtectedEntityTypeManagers([]astrolabe.ProtectedEntityTypeManager{paraVirtPETM})

	ctx := context.Background()

	pvc_petm := pem.GetProtectedEntityTypeManager("pvc")
	if pvc_petm == nil {
		t.Fatal("Failed to get PVC ProtectedEntityTypeManager")
	}

	peids, err := pvc_petm.GetProtectedEntities(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(peids) <= 0 {
		t.Skip("No PVC PEs can be found in the cluster")
	}

	selectedPEID := peids[0]
	logger.Infof("Picked up the first available PVC PE, %v", selectedPEID.String())

	pvcPE, err := pvc_petm.GetProtectedEntity(ctx, selectedPEID)
	if err != nil {
		logger.Errorf("Failed to get PVC PE with PEID = %v", selectedPEID.String())
		t.Fatal(err)
	}
	logger.Infof("PVC PE: %v", pvcPE.GetID().String())

	logger.Infof("Snapshotting the PVC PE, %v", selectedPEID.String())
	_, err = pvcPE.Snapshot(ctx, make(map[string]map[string]interface{}))
	if err != nil {
		logger.Errorf("Failed to snapshot PVC PE with PEID = %v: %v", pvcPE.GetID().String(), err)
		t.Fatal(err)
	}
}
