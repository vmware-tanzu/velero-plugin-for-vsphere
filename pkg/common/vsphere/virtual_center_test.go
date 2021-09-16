package vsphere

import (
	"context"
	"github.com/sirupsen/logrus"
	"github.com/vmware-tanzu/astrolabe/pkg/util"
	"github.com/vmware/govmomi/session"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"testing"
	"time"
)

/*
	Steps
	0. Set KUBECONFIG to the kubeconfig of vanilla or supervisor cluster.
	1. Retrieve the vcenter connection parameters from the secret and create VcenterConfig.
	2. Retrieve the vcenter center instance for the config.
	3. Verify the clients are initialized.

	Expected Output:
	1. No errors during the workflow.

*/
func TestVirtualCenter_Connect(t *testing.T) {
	log := GetLogger()
	ctx := context.Background()
	virtualCenterConfig := RetrieveVirtualCenterConfig(t, log)
	log.Infof("Successfully retrieved VirtualCenterConfig : %+v", virtualCenterConfig)

	virtualCenter, _, _, err := GetVirtualCenter(ctx, virtualCenterConfig, log)
	if err != nil {
		t.Fatalf("Failed to register virtual center err: %+v", err)
	}
	defer func() {
		err = virtualCenter.Disconnect(ctx)
		if err != nil {
			log.Warnf("Was unable to disconnect vc clients as part of cleanup.")
		}
	}()
	if virtualCenter.Client == nil || virtualCenter.CnsClient == nil || virtualCenter.VslmClient == nil {
		t.Fatalf("Failed to initialize vc client")
	}
	log.Infof("PASS: Successfully initialized vcenter client.")
}

/*
	Steps

	0. Set KUBECONFIG to the kubeconfig of vanilla or supervisor cluster.
	1. Retrieve the vcenter connection parameters from the secret and create VcenterConfig.
	2. Retrieve the vcenter center instance for the config.
	3. Verify the clients are initialized.
	4. Invoke Disconnect and verify that clients are uninitialized.

	Expected Output:
	1. No errors during the workflow.

*/
func TestVirtualCenter_Disconnect(t *testing.T) {
	log := GetLogger()
	ctx := context.Background()
	virtualCenterConfig := RetrieveVirtualCenterConfig(t, log)
	log.Infof("Successfully retrieved VirtualCenterConfig : %+v", virtualCenterConfig)

	virtualCenter, _, _, err := GetVirtualCenter(ctx, virtualCenterConfig, log)
	if err != nil {
		t.Fatalf("Failed to register virtual center err: %+v", err)
	}
	if virtualCenter.Client == nil || virtualCenter.CnsClient == nil || virtualCenter.VslmClient == nil {
		t.Fatalf("Failed to initialize vc client")
	}

	err = virtualCenter.Disconnect(ctx)
	if err != nil {
		t.Fatalf("Failed to disconnect virtual center.")
	}
	if virtualCenter.VslmClient != nil || virtualCenter.CnsClient != nil || virtualCenter.Client != nil {
		t.Fatalf("Clients are still initialized after disconnect.")
	}
	log.Infof("PASS: Successfully disconnected vcenter client.")
}

/*
	Steps

	0. Set KUBECONFIG to the kubeconfig of vanilla or supervisor cluster.
	1. Retrieve the vcenter connection parameters from the secret and create VcenterConfig.
	2. Retrieve the vcenter center instance for the config.
	3. Verify the clients are initialized.
	4. Invoke Connect and Disconnect concurrently.

	Expected Output:
	1. No errors during the workflow.

*/
func TestVirtualCenter_ConcurrentConnectDisconnect(t *testing.T) {
	log := GetLogger()
	ctx := context.Background()
	virtualCenterConfig := RetrieveVirtualCenterConfig(t, log)
	log.Infof("Successfully retrieved VirtualCenterConfig : %+v", virtualCenterConfig)

	virtualCenter, _, _, err := GetVirtualCenter(ctx, virtualCenterConfig, log)
	if err != nil {
		t.Fatalf("Failed to register virtual center err: %+v", err)
	}
	out1 := make(chan error)
	out2 := make(chan error)
	go func() {
		_, _, err1 := virtualCenter.Connect(ctx)
		out1 <- err1
	}()
	go func() {
		out2 <- virtualCenter.Disconnect(ctx)
	}()
	connectErr := <-out1
	if connectErr != nil {
		log.Errorf("Received exception on concurrent connect, err : %+v", connectErr)
	}
	disconnectErr := <-out2
	if disconnectErr != nil {
		log.Errorf("Received exception on concurrent disconnect, err : %+v", disconnectErr)
	}
	if connectErr != nil || disconnectErr != nil {
		t.Fatalf("Received exception on concurrent connect disconnect.")
	}
	log.Infof("PASS: Concurrent connect disconnect did not throw any errors.")
}

/*
	Steps

	0. Set KUBECONFIG to the kubeconfig of vanilla or supervisor cluster.
	1. Retrieve the vcenter connection parameters from the secret and create VcenterConfig.
	2. Retrieve the vcenter center instance for the config.
	3. Verify the clients are initialized.
	4. Retrieve the session-id.
	5. Invoke Connect on the vcenter again.
	6. Verify that new session-id is returned.

	Expected Output:
	1. No errors during the workflow.

*/
func TestVirtualCenter_NewSessionCreationOnNewConnect(t *testing.T) {
	log := GetLogger()
	ctx := context.Background()
	virtualCenterConfig := RetrieveVirtualCenterConfig(t, log)
	log.Infof("Successfully retrieved VirtualCenterConfig : %+v", virtualCenterConfig)

	virtualCenter, _, _, err := GetVirtualCenter(ctx, virtualCenterConfig, log)
	if err != nil {
		t.Fatalf("Failed to register virtual center err: %+v", err)
	}

	// Retrieve session id for initial connect.
	sessionMgr := session.NewManager(virtualCenter.Client.Client)
	userSession, err := sessionMgr.UserSession(ctx)
	if err != nil {
		t.Fatalf("Failed to retrieve the user session from current vc, err: %+v", err)
	}
	if userSession == nil {
		t.Fatalf("Session was not initiated.")
	}
	initialSessionId := userSession.Key

	//Disconnect vcenter.
	err = virtualCenter.Disconnect(ctx)
	if err != nil {
		t.Fatalf("Failed to disconnect the initial vcenter session.")
	}
	_, _, err = virtualCenter.Connect(ctx)
	if err != nil {
		t.Fatalf("Failed to connect to vcenter, err: %+v", err)
	}
	if virtualCenter.Client == nil {
		t.Fatalf("Failed to initialize vc client second time")
	}

	// Retrieve session id for initial connect.
	sessionMgr = session.NewManager(virtualCenter.Client.Client)
	userSession, err = sessionMgr.UserSession(ctx)
	if err != nil {
		t.Fatalf("Failed to retrieve the user session from current vc after disconnect and connect, err: %+v", err)
	}
	if userSession == nil {
		t.Fatalf("Session after disconnect and connect was  not initiated.")
	}
	newSessionId := userSession.Key
	if initialSessionId == newSessionId {
		t.Fatalf("Failed to generate new session-id on connect-disconnect-connect")
	}
	log.Infof("PASS: new session-id was generated, old-session: %s, new-session: %s", initialSessionId, newSessionId)
}

func GetLogger() logrus.FieldLogger {
	logger := logrus.New()
	formatter := new(logrus.TextFormatter)
	formatter.TimestampFormat = time.RFC3339Nano
	formatter.FullTimestamp = true
	logger.SetFormatter(formatter)
	logger.SetLevel(logrus.DebugLevel)
	return logger
}

func RetrieveVirtualCenterConfig(t *testing.T, log logrus.FieldLogger) *VirtualCenterConfig {
	path := os.Getenv("KUBECONFIG")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("The KubeConfig file, %v, is not exist", path)
	}
	config, err := clientcmd.BuildConfigFromFlags("", path)
	if err != nil {
		t.Fatalf("Failed to build k8s config from kubeconfig file: %+v ", err)
	}
	params := make(map[string]interface{})
	err = util.RetrievePlatformInfoFromConfig(config, params)
	if err != nil {
		t.Fatalf("Failed to get VC params from k8s: %+v", err)
	}
	virtualCenterConfig, err := GetVirtualCenterConfigFromParams(params, log)
	if err != nil {
		t.Fatalf("Failed to build VirtualCenterConfig from params: %+v", err)
	}
	return virtualCenterConfig
}