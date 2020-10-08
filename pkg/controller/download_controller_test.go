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

package controller

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/agiledragon/gomonkey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmware-tanzu/astrolabe/pkg/astrolabe"
	v1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/datamover/v1alpha1"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/builder"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/constants"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/dataMover"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned/fake"
	informers "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/informers/externalversions"
	veleroplugintest "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/test"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"k8s.io/utils/clock"
	"reflect"
	"strconv"
	"testing"
	"time"
)

func defaultDownload() *builder.DownloadBuilder {
	return builder.ForDownload(constants.DefaultNamespace, "download-1")
}

func TestProcessDownloadSkipItems(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		download *v1.Download
	}{
		{
			name: "missing download returns error",
			key:  "velero/download-1",
		},
		{
			name:     "Failed upload is not processed",
			key:      "velero/download-1",
			download: defaultDownload().Phase(v1.DownloadPhaseFailed).Result(),
		},
		{
			name:     "Completed download is not processed",
			key:      "velero/download-1",
			download: defaultDownload().Phase(v1.DownloadPhaseCompleted).Result(),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var (
				sharedInformers = informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
				logger          = veleroplugintest.NewLogger()
			)

			c := &downloadController{
				genericController: newGenericController("download-test", logger),
				downloadLister:    sharedInformers.Datamover().V1alpha1().Downloads().Lister(),
			}

			if test.download != nil {
				require.NoError(t, sharedInformers.Datamover().V1alpha1().Downloads().Informer().GetStore().Add(test.download))
			}

			err := c.processDownloadItem(test.key)
			assert.Nil(t, err)
		})
	}
}

func TestPatchDownloadByStatus(t *testing.T) {
	tests := []struct {
		name          string
		key           string
		oldPhase      v1.DownloadPhase
		newPhase      v1.DownloadPhase
		expectedPhase v1.DownloadPhase
		download      *v1.Download
		msg           string
	}{
		{
			name:     "Test New to Inprogress",
			key:      "velero/download-1",
			oldPhase: v1.DownloadPhaseNew,
			newPhase: v1.DownloadPhaseInProgress,
			download: defaultDownload().Phase(v1.DownloadPhaseNew).Result(),
			msg:      "",
		},
		{
			name:     "Test InProgress to Completed",
			key:      "velero/download-1",
			oldPhase: v1.DownloadPhaseInProgress,
			newPhase: v1.DownloadPhaseCompleted,
			download: defaultDownload().Phase(v1.DownloadPhaseInProgress).Retry(constants.MIN_RETRY).Result(),
			msg:      "Download completed",
		},
		{
			name:     "Test InProgress to Retry",
			key:      "velero/download-1",
			oldPhase: v1.DownloadPhaseInProgress,
			newPhase: v1.DownLoadPhaseRetry,
			download: defaultDownload().Phase(v1.DownloadPhaseInProgress).Retry(constants.MIN_RETRY).Result(),
			msg:      "Failed to download snapshot, ivd:1234:1234, from durable object storage.",
		},
		{
			name:          "Test maximum retry count",
			key:           "velero/download-1",
			oldPhase:      v1.DownloadPhaseInProgress,
			newPhase:      v1.DownLoadPhaseRetry,
			expectedPhase: v1.DownloadPhaseFailed,
			download:      defaultDownload().Phase(v1.DownloadPhaseInProgress).Retry(constants.DOWNLOAD_MAX_RETRY + 1).Result(),
			msg:           "Failed to download snapshot, ivd:1234:1234, from durable object storage.",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var (
				client          = fake.NewSimpleClientset(test.download)
				sharedInformers = informers.NewSharedInformerFactory(client, 0)
				logger          = veleroplugintest.NewLogger()
				kubeClient      = kubefake.NewSimpleClientset()
			)

			c := &downloadController{
				genericController: newGenericController("download-test", logger),
				kubeClient:        kubeClient,
				downloadClient:    client.DatamoverV1alpha1(),
				downloadLister:    sharedInformers.Datamover().V1alpha1().Downloads().Lister(),
				nodeName:          "download-test",
				clock:             &clock.RealClock{},
				dataMover:         &dataMover.DataMover{},
			}

			if test.download != nil {
				require.NoError(t, sharedInformers.Datamover().V1alpha1().Downloads().Informer().GetStore().Add(test.download))
				// this is necessary so the Patch() call returns the appropriate object
				client.PrependReactor("patch", "downloads", func(action core.Action) (bool, runtime.Object, error) {
					var (
						patch    = action.(core.PatchAction).GetPatch()
						patchMap = make(map[string]interface{})
						res      = test.download.DeepCopy()
					)

					if err := json.Unmarshal(patch, &patchMap); err != nil {
						t.Logf("error unmarshalling patch: %s\n", err)
						return false, nil, err
					}

					// these are the fields that we expect to be set by
					// the controller
					phase, found, err := unstructured.NestedString(patchMap, "status", "phase")
					if err != nil {
						t.Logf("error getting status.phase: %s\n", err)
						return false, nil, err
					}
					if !found {
						t.Logf("status.phase not found")
						return false, nil, errors.New("status.phase not found")
					}
					res.Status.Phase = v1.DownloadPhase(phase)

					message, found, err := unstructured.NestedString(patchMap, "status", "message")
					if err == nil && found {
						res.Status.Message = message
					}

					node, found, err := unstructured.NestedString(patchMap, "status", "processingnode")
					if err == nil && found {
						res.Status.ProcessingNode = node
					}

					completionTime, found, err := unstructured.NestedString(patchMap, "status", "completiontimestamp")
					if err == nil && found {
						parsed, err := time.Parse(time.RFC3339, completionTime)
						if err != nil {
							t.Logf("error parsing status.completiontimestamp: %s\n", err)
							return false, nil, err
						}
						res.Status.CompletionTimestamp = &metav1.Time{Time: parsed}
					}

					startTime, found, err := unstructured.NestedString(patchMap, "status", "starttimestamp")
					if err == nil && found {
						parsed, err := time.Parse(time.RFC3339, startTime)
						if err != nil {
							t.Logf("error parsing status.starttimestamp: %s\n", err)
							return false, nil, err
						}
						res.Status.StartTimestamp = &metav1.Time{Time: parsed}
					}

					nextTime, found, err := unstructured.NestedString(patchMap, "status", "nextretrytimestamp")
					if err == nil && found {
						parsed, err := time.Parse(time.RFC3339, nextTime)
						if err != nil {
							t.Logf("error parsing status.nextretrytimestamp: %s\n", err)
							return false, nil, err
						}
						res.Status.NextRetryTimestamp = &metav1.Time{Time: parsed}
					}

					retry, found, err := unstructured.NestedString(patchMap, "status", "retrycount")
					if err == nil && found {
						retryCnt, err := strconv.ParseInt(retry, 10, 32)
						if err != nil {
							t.Logf("error converting status.retryCount: %v\n", err)
							return false, nil, err
						}
						res.Status.RetryCount = int32(retryCnt)
					}

					return true, res, nil
				})
			}

			var oldRetry int32
			if test.download.Status.Phase != v1.DownloadPhaseNew {
				oldRetry = test.download.Status.RetryCount
			}
			res, err := c.patchDownloadByStatus(test.download, test.newPhase, test.msg)
			require.NoError(t, err)
			require.Equal(t, test.msg, res.Status.Message)
			if test.oldPhase == v1.DownloadPhaseNew {
				require.Equal(t, int32(constants.MIN_RETRY), res.Status.RetryCount)
			}
			if test.newPhase == v1.DownLoadPhaseRetry {
				newRetry := res.Status.RetryCount
				if newRetry > constants.DOWNLOAD_MAX_RETRY {
					test.newPhase = test.expectedPhase
				} else {
					require.Equal(t, oldRetry+1, newRetry)
				}
			}
			require.Equal(t, test.newPhase, res.Status.Phase)
		})
	}
}

func TestProcessedDownloadItem(t *testing.T) {
	tests := []struct {
		name          string
		key           string
		download      *v1.Download
		expectedPhase v1.DownloadPhase
		expectedErr   error
	}{
		{
			name:          "New download proccessed to be completed",
			key:           "velero/download-1",
			download:      defaultDownload().Phase(v1.DownloadPhaseNew).SnapshotID("ivd:1234:1234").Result(),
			expectedPhase: v1.DownloadPhaseCompleted,
			expectedErr:   nil,
		},
		{
			name:          "Invalid peID should fail download ",
			key:           "velero/download-1",
			download:      defaultDownload().Phase(v1.DownloadPhaseNew).SnapshotID("ivd:invalid").Retry(constants.MIN_RETRY).Result(),
			expectedPhase: v1.DownLoadPhaseRetry,
			expectedErr:   errors.New("Failed to get PEID from SnapshotID, ivd:invalid"),
		},
		{
			name:          "Download fail when copying from remote repository",
			key:           "velero/download-1",
			download:      defaultDownload().Phase(v1.DownloadPhaseNew).SnapshotID("ivd:1234:1234").Result(),
			expectedPhase: v1.DownLoadPhaseRetry,
			expectedErr:   errors.New("Failed to download snapshot, ivd:1234:1234, from durable object storage."),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var (
				clientset       = fake.NewSimpleClientset(test.download)
				sharedInformers = informers.NewSharedInformerFactory(clientset, 0)
				logger          = veleroplugintest.NewLogger()
				kubeClient      = kubefake.NewSimpleClientset()
			)

			c := &downloadController{
				genericController: newGenericController("download-test", logger),
				kubeClient:        kubeClient,
				downloadClient:    clientset.DatamoverV1alpha1(),
				downloadLister:    sharedInformers.Datamover().V1alpha1().Downloads().Lister(),
				nodeName:          "download-test",
				clock:             &clock.RealClock{},
				dataMover:         &dataMover.DataMover{},
			}
			require.NoError(t, sharedInformers.Datamover().V1alpha1().Downloads().Informer().GetStore().Add(test.download))

			patches := gomonkey.ApplyMethod(reflect.TypeOf(c.dataMover), "CopyFromRepo", func(_ *dataMover.DataMover, _ astrolabe.ProtectedEntityID,
				_ astrolabe.ProtectedEntityID, _ astrolabe.CopyCreateOptions) (astrolabe.ProtectedEntityID, error) {
				return astrolabe.ProtectedEntityID{}, test.expectedErr
			})
			defer patches.Reset()

			c.processDownloadFunc = c.processDownload
			err := c.processDownloadItem(test.key)
			require.Equal(t, test.expectedErr == nil, err == nil)
			res, err := c.downloadClient.Downloads(test.download.Namespace).Get(context.TODO(), test.download.Name, metav1.GetOptions{})
			require.Nil(t, err)
			require.Equal(t, test.expectedPhase, res.Status.Phase)
		})
	}
}
