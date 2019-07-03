/*
 * Copyright 2019 VMware, Inc..
 * SPDX-License-Identifier: Apache-2.0
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package snapshotmgr

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/vmware-tanzu/velero/pkg/generated/clientset/versioned"

	//"encoding/base64"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"strings"
	"testing"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)


func TestGetVcConfigSecret(t *testing.T) {
	path := os.Getenv("HOME") + "/.kube/config"
	config, err := clientcmd.BuildConfigFromFlags("", path)
	if err != nil {
		t.Fatal("Got error " + err.Error())
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		t.Fatal("Got error " + err.Error())
	}

	ns := "kube-system"
	secretApis := clientset.CoreV1().Secrets(ns)
	vsphere_secret := "vsphere-config-secret"
	secret, err := secretApis.Get(vsphere_secret, metav1.GetOptions{})
	//t.Logf("encoded secret: %s", secret.Data["csi-vsphere.conf"])
	sEnc := string(secret.Data["csi-vsphere.conf"])
	//sDec, _ := base64.StdEncoding.DecodeString(sEnc)
	t.Logf("Encoded secret: %s", sEnc)
	lines := strings.Split(sEnc, "\n")

	vcMap := make(map[string]string)

	for _, line := range lines {
		if strings.Contains(line, "VirtualCenter") {

			parts := strings.Split(line, "\"")
			vcMap["VirtualCenter"] = parts[1]
		} else if strings.Contains(line, "=") {
			parts := strings.Split(line, "=")
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			vcMap[key] = value[1: len(value) - 1]
		}
	}
	//t.Logf("vcMap: %v", vcMap)
	t.Logf("vcHosts: %s", vcMap["VirtualCenter"])
	t.Logf("insecureVC: %s", vcMap["insecure-flag"])
	t.Logf("user: %s", vcMap["user"])
	t.Logf("password: %s", vcMap["password"])
}

func TestS3OperationsUnderEnvVars(t *testing.T) {
	_, exist := os.LookupEnv("AWS_SHARED_CREDENTIALS_FILE")
	if !exist {
		t.Fatal("Expected Environmental Variable, AWS_SHARED_CREDENTIALS_FILE, is not set")
	}
	region := "us-west-1"
	sess := session.Must(session.NewSession(&aws.Config{
		Region: aws.String(region),
	}))

	svc := s3.New(sess)

	result, err := svc.ListBuckets(nil)
	if err != nil {
		t.Fatal("Unable to list buckets " + err.Error())
	}
	// list all buckets
	t.Logf("Buckets:")

	for _, b := range result.Buckets {
		t.Logf("* %s created on %s\n",
			aws.StringValue(b.Name), aws.TimeValue(b.CreationDate))
	}

	bucket := "velero-backups-lintongj"
	t.Logf("S3 base URL: s3-%s.amazonaws.com/%s", region, bucket)
	resp, err := svc.ListObjectsV2(&s3.ListObjectsV2Input{Bucket: aws.String(bucket)})
	if err != nil {
		t.Fatalf("Unable to list items in bucket %q, %v", bucket, err)
	}

	for i, item := range resp.Contents {
		if i > 10 {
			break
		}
		t.Log("Name:         ", *item.Key)
		t.Log("Last modified:", *item.LastModified)
		t.Log("Size:         ", *item.Size)
		t.Log("Storage class:", *item.StorageClass)
		t.Log("")
	}
}


func TestUseVeleroV1API(t *testing.T) {
	path := os.Getenv("HOME") + "/.kube/config"
	config, err := clientcmd.BuildConfigFromFlags("", path)
	if err != nil {
		t.Fatal("Got error " + err.Error())
	}

	veleroClient, err := versioned.NewForConfig(config)
	if err != nil {
		t.Fatal("Got error " + err.Error())
	}
	backupLocationList, err := veleroClient.VeleroV1().BackupStorageLocations("velero").List(metav1.ListOptions{})

	for _, item := range backupLocationList.Items {
		t.Logf("List name: %s", item.Name)
		t.Logf("List spec.config.region: %v", item.Spec.Config["region"])
		t.Logf("List spec.objectstorage: %s", item.Spec.ObjectStorage.Bucket)
	}

	backupStorageLocation ,err := veleroClient.VeleroV1().BackupStorageLocations("velero").Get("default", metav1.GetOptions{})
	t.Logf("Get spec.config.region: %v", backupStorageLocation.Spec.Config["region"])
	t.Logf("Get spec.objectstorage: %s", backupStorageLocation.Spec.ObjectStorage.Bucket)
}