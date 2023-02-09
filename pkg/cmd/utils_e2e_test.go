/*
Copyright 2023 the Velero contributors.

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
	"k8s.io/client-go/kubernetes"
	"os"
	"reflect"
	"strconv"
	"testing"
	"time"
	"k8s.io/client-go/tools/clientcmd"
	"github.com/sirupsen/logrus"
)

func TestCreateBlockListConfigMap(t *testing.T) {
	path := os.Getenv("KUBECONFIG")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("The KubeConfig file, %v, is not exist", path)
	}

	config, err := clientcmd.BuildConfigFromFlags("", path)
	if err != nil {
		t.Fatalf("Failed to build k8s config from kubeconfig file: %+v ", err)
	}
	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		t.Fatalf("Failed to retrieve kubeclient from config: %+v ", err)
	}

	// Setup Logger
	logger := logrus.New()
	formatter := new(logrus.TextFormatter)
	formatter.TimestampFormat = time.RFC3339Nano
	formatter.FullTimestamp = true
	logger.SetFormatter(formatter)
	logger.SetLevel(logrus.DebugLevel)

	// using velero ns for testing.
	veleroNs := "velero"

	resourcesToBlockForTest := map[string]bool{
		"agentinstalls.installers.tmc.cloud.vmware.com": true,
		"availabilityzones.topology.tanzu.vmware.com":   true,
		"aviloadbalancerconfigs.netoperator.vmware.com": true,
		"blockaffinities.crd.projectcalico.org":         true,
	}

	// Case 1: if there is not blocklist configmap created before

    err = CreateBlockListConfigMap(kubeClient, veleroNs, resourcesToBlockForTest)
    if err != nil {
		t.Fatalf("Failed to create blocklist configmap: %+v ", err)
	}

    blockListMap, err := RetrieveBlockListConfigMap()
    if err != nil {
    	t.Fatal("Failed to retrieve blocklist configmap.")
	}
    if len(blockListMap) != len(resourcesToBlockForTest) {
    	t.Fatal("The length of blocklist configmap created does not equal to resourcesToBlockForTest input length.")
	}

    for crdName, isBlocked := range resourcesToBlockForTest{
    	val, ok := blockListMap[crdName]
    	if !ok {
			t.Fatal("Failed to create blocklist configmap from given resourcesToBlockForTest input.")
		}
    	isBlockedConfigMap, err := strconv.ParseBool(val)
    	if err != nil {
    		t.Fatal("Failed to create blocklist configmap from given resourcesToBlockForTest input.")
		}
    	if isBlocked != isBlockedConfigMap {
			t.Fatal("Failed to create blocklist configmap from given resourcesToBlockForTest input.")
		}
	}

    t.Log("Blocklist configmap created successfully.")

	// Case 2: if there is already blocklist configmap created before, it should be unchanged.
	resourcesToBlockForTest_2 := map[string]bool{
		"agentinstalls.installers.tmc.cloud.vmware.com": true,
		"availabilityzones.topology.tanzu.vmware.com":   true,
		"aviloadbalancerconfigs.netoperator.vmware.com": true,
		"blockaffinities.crd.projectcalico.org":         true,
		"ccsplugins.appplatform.wcp.vmware.com":         true,
	}

	err = CreateBlockListConfigMap(kubeClient, veleroNs, resourcesToBlockForTest_2)
	if err != nil {
		t.Fatalf("Failed to create blocklist configmap: %+v ", err)
	}
	blockListMap_2, err := RetrieveBlockListConfigMap()
	if err != nil {
		t.Fatal("Failed to retrieve blocklist configmap.")
	}
	//blockListMap_2 should be equal to blockListMap
	eq := reflect.DeepEqual(blockListMap_2, blockListMap)
	if !eq {
		t.Fatal("ConfigMap for blocking list should keeps unchanged.")
	}

	t.Log("Blocklist configmap updated successfully.")
}
