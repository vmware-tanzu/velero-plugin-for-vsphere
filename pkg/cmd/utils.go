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

package cmd

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/pkg/errors"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/utils"
	"github.com/vmware-tanzu/velero/pkg/client"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CheckError prints err to stderr and exits with code 1 if err is not nil. Otherwise, it is a
// no-op.
func CheckError(err error) {
	if err != nil {
		if err != context.Canceled {
			fmt.Fprintf(os.Stderr, "An error occurred: %v\n", err)
		}
		os.Exit(1)
	}
}

// Exit prints msg (with optional args), plus a newline, to stderr and exits with code 1.
func Exit(msg string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, msg+"\n", args...)
	os.Exit(1)
}

// Return version in the format: vX.Y.Z
func GetVersionFromImage(containers []v1.Container, imageName string) string {
	var tag = ""
	for _, container := range containers {
		if strings.Contains(container.Image, imageName) {
			tag = strings.Split(container.Image, ":")[1]
			break
		}
	}
	if tag == "" {
		fmt.Printf("Failed to get tag from image %s\n", imageName)
		return ""
	}
	if strings.Contains(tag, "-") {
		version := strings.Split(tag, "-")[0]
		return version
	} else {
		return tag
	}
}

func GetVeleroVersion(f client.Factory) (string, error) {
	clientset, err := f.KubeClient()
	if err != nil {
		fmt.Println("Failed to get kubeclient.")
		return "", err
	}
	deploymentList, err := clientset.AppsV1().Deployments("velero").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		fmt.Println("Failed to get deployment for velero namespace.")
		return "", err
	}
	for _, item := range deploymentList.Items {
		if item.GetName() == "velero" {
			version := GetVersionFromImage(item.Spec.Template.Spec.Containers, "velero/velero")
			return version, nil
		}
	}
	return "", nil
}

// Go doesn't have max function for integers?  Oh really, that's just so convenient
func max(x, y int) int {
	if x > y {
		return x
	}
	return y
}

// If currentVersion == minVersion, return 0
// If currentVersion > minVersion, return 1
// If currentVersion < minVersion, return -1
// Assume input string format is vX.Y.Z
func CompareVersion(currentVersion string, minVersion string) int {
	if currentVersion == "" {
		currentVersion = "v0.0.0"
	}
	if minVersion == "" {
		minVersion = "v0.0.0"
	}
	curVerArr := strings.Split(currentVersion[1:], ".")
	minVerArr := strings.Split(minVersion[1:], ".")

	for index := 0; index < max(len(curVerArr), len(minVerArr)); index++ {
		var curVerDigit, minVerDigit int
		if index < len(curVerArr) {
			curVerDigit, _ = strconv.Atoi(curVerArr[index])
		} else {
			curVerDigit = 0 // Substitute 0 if a position is missing
		}
		if index < len(minVerArr) {
			minVerDigit, _ = strconv.Atoi(minVerArr[index])
		} else {
			minVerDigit = 0
		}
		if curVerDigit < minVerDigit {
			return -1
		} else if curVerDigit > minVerDigit {
			return 1
		}
	}
	return 0
}

func CheckCSIVersion(containers []v1.Container) (bool, bool, error) {
	isVersionOK := false
	csi_driver_version := GetVersionFromImage(containers, "gcr.io/cloud-provider-vsphere/csi/release/driver")
	if csi_driver_version == "" {
		csi_driver_version = GetVersionFromImage(containers, "cloudnativestorage/vsphere-csi")
		if csi_driver_version != "" {
			fmt.Printf("Got pre-relase version %s from container cloudnativestorage/vsphere-csi, setting version to min version %s\n",
				csi_driver_version, utils.CsiMinVersion)
			csi_driver_version = utils.CsiMinVersion
		}
	}
	csi_syncer_version := GetVersionFromImage(containers, "gcr.io/cloud-provider-vsphere/csi/release/syncer")
	if csi_syncer_version == "" {
		csi_syncer_version = GetVersionFromImage(containers, "cloudnativestorage/syncer")
		if csi_syncer_version != "" {
			fmt.Printf("Got pre-relase version %s from container cloudnativestorage/syncer, setting version to min version %s\n",
				csi_syncer_version, utils.CsiMinVersion)
			csi_syncer_version = utils.CsiMinVersion
		}
	}
	if CompareVersion(csi_driver_version, utils.CsiMinVersion) >= 0 && CompareVersion(csi_syncer_version, utils.CsiMinVersion) >= 0 {
		isVersionOK = true
	}
	return true, isVersionOK, nil
}

func CheckCSIInstalled(f client.Factory) (bool, bool, error) {
	clientset, err := f.KubeClient()
	if err != nil {
		return false, false, err
	}
	statefulsetList, err := clientset.AppsV1().StatefulSets("kube-system").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return false, false, err
	}
	for _, item := range statefulsetList.Items {
		if item.GetName() == "vsphere-csi-controller" {
			return CheckCSIVersion(item.Spec.Template.Spec.Containers)
		}
	}
	deploymentList, err := clientset.AppsV1().Deployments("kube-system").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return false, false, err
	}
	for _, item := range deploymentList.Items {
		if item.Name == "vsphere-csi-controller" {
			return CheckCSIVersion(item.Spec.Template.Spec.Containers)
		}
	}
	return false, false, nil
}

func BuildConfig(master, kubeConfig string, f client.Factory) (*rest.Config, error) {
	var config *rest.Config
	var err error
	if master != "" || kubeConfig != "" {
		config, err = clientcmd.BuildConfigFromFlags(master, kubeConfig)
	} else {
		config, err = f.ClientConfig()
	}
	if err != nil {
		return nil, errors.Errorf("failed to create config: %v", err)
	}
	return config, nil
}
