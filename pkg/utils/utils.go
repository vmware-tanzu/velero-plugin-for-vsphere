package utils

import (
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"strings"
	"github.com/vmware-tanzu/velero/pkg/generated/clientset/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func RetrieveBackupStorageLocation(params map[string]interface{}, logger logrus.FieldLogger) error {
	config, err := rest.InClusterConfig()
	if err != nil {
		return err
	}

	veleroClient, err := versioned.NewForConfig(config)
	if err != nil {
		return err
	}
	defaultBackupLocation := "default"
	backupStorageLocation, err := veleroClient.VeleroV1().BackupStorageLocations("velero").
		Get(defaultBackupLocation, metav1.GetOptions{})
	params["region"] = backupStorageLocation.Spec.Config["region"]
	params["bucket"] = backupStorageLocation.Spec.ObjectStorage.Bucket

	return nil
}

/*
 * In the CSI setup, VC credential is stored as a secret
 * under the kube-system namespace.
 */
func RetrieveVcConfigSecret(params map[string]interface{}, logger logrus.FieldLogger) error {
	config, err := rest.InClusterConfig()
	if err != nil {
		logger.Errorf("Failed to get k8s inClusterConfig")
		return err
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		logger.Errorf("Failed to get k8s clientset with the given config")
		return err
	}

	ns := "kube-system"
	secretApis := clientset.CoreV1().Secrets(ns)
	vsphere_secret := "vsphere-config-secret"
	secret, err := secretApis.Get(vsphere_secret, metav1.GetOptions{})
	if err != nil {
		logger.Errorf("Failed to get k8s secret, %s", vsphere_secret)
		return err
	}
	sEnc := string(secret.Data["csi-vsphere.conf"])
	lines := strings.Split(sEnc, "\n")

	for _, line := range lines {
		if strings.Contains(line, "VirtualCenter") {
			parts := strings.Split(line, "\"")
			params["VirtualCenter"] = parts[1]
		} else if strings.Contains(line, "=") {
			parts := strings.Split(line, "=")
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			params[key] = value[1 : len(value)-1]
		}
	}
	return nil
}


func RetrieveVcConfigSecretByHardCoding(params map[string]interface{}, logger logrus.FieldLogger) error {
	params["VirtualCenter"] = "10.193.45.3"
	params["port"] = "443"
	params["user"] = "Administrator@vsphere.local"
	params["password"] = "Admin!23"
	params["insecure-flag"] = "true"
	return nil
}