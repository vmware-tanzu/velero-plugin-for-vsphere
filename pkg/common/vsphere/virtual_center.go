package vsphere

import (
	"context"
	"github.com/sirupsen/logrus"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/cns"
	"github.com/vmware/govmomi/session"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vslm"
	"net"
	neturl "net/url"
	"strconv"
	"sync"
	"time"
)

const (
	// DefaultScheme is the default connection scheme.
	DefaultScheme = "https"
	// DefaultRoundTripperCount is the default SOAP round tripper count.
	DefaultRoundTripperCount = 3
	// DefaultVCClientTimeoutInMinutes is the default timeout value.
	DefaultVCClientTimeoutInMinutes = 30
	// DefaultAuthErrorRetryCount is the number of retries
	DefaultAuthErrorRetryCount = 1
)

// Keys for VCenter parameters
const (
	HostVcParamKey         = "VirtualCenter"
	UserVcParamKey         = "user"
	PasswordVcParamKey     = "password"
	PortVcParamKey         = "port"
	DatacenterVcParamKey   = "datacenters"
	InsecureFlagVcParamKey = "insecure-flag"
	ClusterVcParamKey      = "cluster-id"
)

// VirtualCenter holds details of a virtual center instance.
type VirtualCenter struct {
	// Config represents the virtual center configuration.
	Config *VirtualCenterConfig
	// Client represents the govmomi client instance for the connection.
	Client *govmomi.Client
	// CnsClient represents the CNS client instance.
	CnsClient *cns.Client
	// VslmClient  represents the VSLM client instance.
	VslmClient *vslm.GlobalObjectManager
	// Logger
	logger logrus.FieldLogger
	// Mutex to handle concurrent connect/disconnect
	connectMutex sync.Mutex
}

// VirtualCenterConfig represents virtual center configuration.
type VirtualCenterConfig struct {
	// Scheme represents the connection scheme. (Ex: https)
	Scheme string
	// Host represents the virtual center host address.
	Host string
	// Port represents the virtual center host port.
	Port int
	// Username represents the virtual center username.
	Username string
	// Password represents the virtual center password in clear text.
	Password string
	// Cluster-id
	ClusterId string
	// Specifies whether to verify the server's certificate chain. Set to true to
	// skip verification.
	Insecure bool
	// RoundTripperCount is the SOAP round tripper count. (retries = RoundTripperCount - 1)
	RoundTripperCount int
	// VCClientTimeout is the time limit in minutes for requests made by vCenter client
	VCClientTimeout int
}

func GetVirtualCenter(ctx context.Context, config *VirtualCenterConfig, log logrus.FieldLogger) (*VirtualCenter, *vslm.GlobalObjectManager, *cns.Client, error) {
	vc := &VirtualCenter{Config: config, logger: log}
	vslmClient, cnsClient, err := vc.Connect(ctx)
	if err != nil {
		log.Errorf("Failed to connect to virtual center while retrieving a instance of it.")
		return nil, nil, nil, err
	}
	return vc, vslmClient, cnsClient, nil
}

// newVslmClient creates a new VSLM client
func newVslmClient(ctx context.Context, c *vim25.Client, logger logrus.FieldLogger) (*vslm.Client, error) {
	vslmClient, err := vslm.NewClient(ctx, c)
	if err != nil {
		logger.Errorf("failed to create a new client for VSLM. err: %v", err)
		return nil, err
	}
	return vslmClient, nil
}

// NewCnsClient creates a new CNS client
func newCnsClient(ctx context.Context, c *vim25.Client, logger logrus.FieldLogger) (*cns.Client, error) {
	cnsClient, err := cns.NewClient(ctx, c)
	if err != nil {
		logger.Errorf("failed to create a new client for CNS. err: %v", err)
		return nil, err
	}
	return cnsClient, nil
}

// newClient creates a new govmomi Client instance.
func (this *VirtualCenter) newClient(ctx context.Context) (*govmomi.Client, error) {
	log := this.logger
	if this.Config.Scheme == "" {
		this.Config.Scheme = DefaultScheme
	}

	url, err := soap.ParseURL(net.JoinHostPort(this.Config.Host, strconv.Itoa(this.Config.Port)))
	if err != nil {
		log.Errorf("failed to parse URL %s with err: %v", url, err)
		return nil, err
	}
	if this.Config.Insecure == false {
		log.Warnf("The vCenter Configuration states secure connection, overriding to use insecure connection..")
		this.Config.Insecure = true
		// TODO: support vCenter connection using certs.
	}
	soapClient := soap.NewClient(url, this.Config.Insecure)
	soapClient.Timeout = time.Duration(this.Config.VCClientTimeout) * time.Minute
	log.Debugf("Setting vCenter soap client timeout to %v", soapClient.Timeout)
	vimClient, err := vim25.NewClient(ctx, soapClient)
	if err != nil {
		log.Errorf("failed to create new client with err: %v", err)
		return nil, err
	}
	vimClient.RoundTripper = session.KeepAlive(vimClient.RoundTripper, 10*time.Minute)
	err = vimClient.UseServiceVersion("vsan")
	if err != nil && this.Config.Host != "127.0.0.1" {
		// skipping error for simulator connection for unit tests
		log.Errorf("Failed to set vimClient service version to vsan. err: %v", err)
		return nil, err
	}
	vimClient.UserAgent = "cns-dp-useragent"

	client := &govmomi.Client{
		Client:         vimClient,
		SessionManager: session.NewManager(vimClient),
	}

	err = this.login(ctx, client)
	if err != nil {
		return nil, err
	}

	s, err := client.SessionManager.UserSession(ctx)
	if err == nil {
		log.Infof("New session ID for '%s' = %s", s.UserName, s.Key)
	}

	if this.Config.RoundTripperCount == 0 {
		this.Config.RoundTripperCount = DefaultRoundTripperCount
	}
	client.RoundTripper = vim25.Retry(client.RoundTripper, vim25.TemporaryNetworkError(this.Config.RoundTripperCount))
	return client, nil
}

// SessionManager.Login with user and password.
func (this *VirtualCenter) login(ctx context.Context, client *govmomi.Client) error {
	log := this.logger
	log.Infof("Attempting login for user: %s", this.Config.Username)
	err := client.SessionManager.Login(ctx, neturl.UserPassword(this.Config.Username, this.Config.Password))
	if err != nil {
		log.Errorf("Failed for login for username : %s error: %v", this.Config.Username, err)
	}
	return err
}

// Connect establishes a new connection with vSphere with updated credentials
// If credentials are invalid then it fails the connection.
func (this *VirtualCenter) Connect(ctx context.Context) (*vslm.GlobalObjectManager, *cns.Client, error) {
	this.connectMutex.Lock()
	defer this.connectMutex.Unlock()
	log := this.logger
	// If client was never initialized, initialize one.
	var err error
	if this.Client == nil {
		this.Client, err = this.newClient(ctx)
		if err != nil {
			log.Errorf("failed to create govmomi client with err: %v", err)
			return nil, nil, err
		}
	}
	if this.VslmClient == nil {
		vslmVcClient, err := newVslmClient(ctx, this.Client.Client, log)
		if err != nil {
			log.Errorf("failed to create VSLM client on vCenter host %q with err: %v", this.Config.Host, err)
			return nil, nil, err
		}
		this.VslmClient = vslm.NewGlobalObjectManager(vslmVcClient)
	}
	if this.CnsClient == nil {
		this.CnsClient, err = newCnsClient(ctx, this.Client.Client, log)
		if err != nil {
			log.Errorf("failed to create CNS client on vCenter host %q with err: %v", this.Config.Host, err)
			return nil, nil, err
		}
	}

	// If session hasn't expired, nothing to do.
	sessionMgr := session.NewManager(this.Client.Client)
	// SessionMgr.UserSession(ctx) retrieves and returns the SessionManager's CurrentSession field
	// Nil is returned if the session is not authenticated or timed out.
	userSession, err := sessionMgr.UserSession(ctx)
	if err != nil {
		log.Errorf("failed to obtain user session with err: %v", err)
		return nil, nil, err
	}
	// Got a valid session if userSession is non-nil
	if userSession != nil {
		return this.VslmClient, this.CnsClient, nil
	}
	// Re-create all clients since the old session is no longer valid.
	// If session has expired, create a new instance.
	log.Warnf("Creating a new client session as the existing session isn't valid or not authenticated")
	this.Client, err = this.newClient(ctx)
	if err != nil {
		log.Errorf("failed to create govmomi client with err: %v", err)
		return nil, nil, err
	}

	// Recreate VslmClient If created using timed out VC Client
	vslmVcClient, err := newVslmClient(ctx, this.Client.Client, this.logger)
	if err != nil {
		log.Errorf("failed to create VSLM client with err: %v", err)
		return nil, nil, err
	}
	this.VslmClient = vslm.NewGlobalObjectManager(vslmVcClient)

	// Recreate CNSClient If created using timed out VC Client
	this.CnsClient, err = newCnsClient(ctx, this.Client.Client, this.logger)
	if err != nil {
		log.Errorf("failed to create CNS client on vCenter host %v with err: %v", this.Config.Host, err)
		return nil, nil, err
	}

	return this.VslmClient, this.CnsClient, nil
}

// Disconnect disconnects the virtual center host connection if connected.
func (this *VirtualCenter) Disconnect(ctx context.Context) error {
	this.connectMutex.Lock()
	defer this.connectMutex.Unlock()
	log := this.logger
	if this.VslmClient == nil {
		log.Info("VslmClient wasn't connected, ignoring")
	} else {
		this.VslmClient = nil
		log.Info("VslmClient was successfully disconnected.")
	}
	if this.CnsClient == nil {
		log.Info("CnsClient wasn't connected, ignoring")
	} else {
		this.CnsClient = nil
		log.Info("CnsClient was successfully disconnected.")
	}
	if this.Client == nil {
		log.Info("Client wasn't connected, ignoring")
		return nil
	}
	if err := this.Client.Logout(ctx); err != nil {
		log.Errorf("failed to logout with err: %v", err)
		return err
	} else {
		log.Info("VC Client was logged out.")
	}
	this.Client = nil
	log.Infof("VC client disconnected.")
	log.Infof("Disconnected all clients for the virtual center %s", this.Config.Host)
	return nil
}
