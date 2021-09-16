package vsphere

import (
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/vmware/govmomi/vim25/soap"
	vim "github.com/vmware/govmomi/vim25/types"
	"strconv"
)

func GetStringFromParamsMap(params map[string]interface{}, key string) (string, error) {
	valueIF, ok := params[key]
	if ok {
		value, ok := valueIF.(string)
		if !ok {
			return "", errors.New("Value for params key " + key + " is not a string")
		}
		return value, nil
	} else {
		return "", errors.New("No such key " + key + " in params map")
	}
}

func GetVirtualCenterFromParamsMap(params map[string]interface{}) (string, error) {
	return GetStringFromParamsMap(params, HostVcParamKey)
}

func GetUserFromParamsMap(params map[string]interface{}) (string, error) {
	return GetStringFromParamsMap(params, UserVcParamKey)
}

func GetPasswordFromParamsMap(params map[string]interface{}) (string, error) {
	return GetStringFromParamsMap(params, PasswordVcParamKey)
}

func GetPortFromParamsMap(params map[string]interface{}) (string, error) {
	return GetStringFromParamsMap(params, PortVcParamKey)
}

func GetDatacenterFromParamsMap(params map[string]interface{}) (string, error) {
	return GetStringFromParamsMap(params, DatacenterVcParamKey)
}

func GetClusterFromParamsMap(params map[string]interface{}) (string, error) {
	return GetStringFromParamsMap(params, ClusterVcParamKey)
}

func GetInsecureFlagFromParamsMap(params map[string]interface{}) (bool, error) {
	insecureStr, err := GetStringFromParamsMap(params, InsecureFlagVcParamKey)
	if err == nil {
		return strconv.ParseBool(insecureStr)
	}
	return false, err
}

func GetVirtualCenterConfigFromParams(params map[string]interface{}, logger logrus.FieldLogger) (*VirtualCenterConfig, error) {
	vcHost, err := GetVirtualCenterFromParamsMap(params)
	if err != nil {
		return nil, err
	}
	vcHostPortStr, err := GetPortFromParamsMap(params)
	if err != nil {
		return nil, err
	}
	vcHostPort, err := strconv.Atoi(vcHostPortStr)
	if err != nil {
		return nil, err
	}
	vcUser, err := GetUserFromParamsMap(params)
	if err != nil {
		return nil, err
	}
	vcPassword, err := GetPasswordFromParamsMap(params)
	if err != nil {
		return nil, err
	}
	insecure, err := GetInsecureFlagFromParamsMap(params)
	if err != nil {
		return nil, err
	}
	clusterId, err := GetClusterFromParamsMap(params)
	if err != nil {
		return nil, err
	}
	logger.Debug("Successfully retrieved VirtualCenterConfig from parameters.")
	// Translate the params into VirtualCenterConfig.
	vcConfig := &VirtualCenterConfig{
		Scheme:          "https",
		Host:            vcHost,
		Port:            vcHostPort,
		Username:        vcUser,
		Password:        vcPassword,
		ClusterId:       clusterId,
		Insecure:        insecure,
		VCClientTimeout: DefaultVCClientTimeoutInMinutes,
	}
	return vcConfig, nil
}

// Compare the vc configs, returns true if they are same else false
func CheckIfVirtualCenterConfigChanged(oldVcConfig *VirtualCenterConfig, newVcConfig *VirtualCenterConfig) bool {
	if oldVcConfig == nil {
		return true
	}
	if oldVcConfig.Host != newVcConfig.Host || oldVcConfig.Username != newVcConfig.Username ||
		oldVcConfig.Password != newVcConfig.Password {
		return true
	}
	return false
}

func CheckForVcAuthFaults(err error, log logrus.FieldLogger) bool {
	if soap.IsSoapFault(err) {
		soapFault := soap.ToSoapFault(err)
		receivedFault := soapFault.Detail.Fault
		_, ok := receivedFault.(vim.NotAuthenticated)
		if ok {
			log.Infof("NotAuthenticated fault received during VC API invocation, err: %v", err)
			return true
		}
		_, ok = receivedFault.(vim.InvalidLogin)
		if ok {
			log.Infof("InvalidLogin fault received during VC API invocation, err: %v", err)
			return true
		}
	}
	return false
}
