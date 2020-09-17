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
	v1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/veleroplugin/v1"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/builder"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/constants"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/dataMover"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned/fake"
	informers "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/informers/externalversions"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/snapshotmgr"
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

func defaultUpload() *builder.UploadBuilder {
	return builder.ForUpload(constants.DefaultNamespace, "upload-1")
}

func TestProcessUploadNonProcessedItems(t *testing.T) {
	tests := []struct {
		name   string
		key    string
		upload *v1.Upload
	}{
		{
			name: "missing upload returns error",
			key:  "foo/bar",
		},
		{
			name:   "CleanupFailed upload is not processed",
			key:    "velero/upload-1",
			upload: defaultUpload().Phase(v1.UploadPhaseCleanupFailed).Result(),
		},
		{
			name:   "Completed upload is not processed",
			key:    "velero/upload-1",
			upload: defaultUpload().Phase(v1.UploadPhaseCompleted).Result(),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var (
				sharedInformers = informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
				logger          = veleroplugintest.NewLogger()
			)

			c := &uploadController{
				genericController: newGenericController("upload-test", logger),
				uploadLister:      sharedInformers.Veleroplugin().V1().Uploads().Lister(),
			}

			if test.upload != nil {
				require.NoError(t, sharedInformers.Veleroplugin().V1().Uploads().Informer().GetStore().Add(test.upload))
			}

			err := c.processUploadItem(test.key)
			assert.Nil(t, err)
		})
	}
}

func TestProcessedUploadItem(t *testing.T) {
	tests := []struct {
		name          string
		key           string
		upload        *v1.Upload
		expectedPhase v1.UploadPhase
		expectedErr   error
		cleanupFail   bool
	}{
		{
			name:          "New upload proccessed to be completed",
			key:           "velero/upload-1",
			upload:        defaultUpload().Phase(v1.UploadPhaseNew).SnapshotID("ivd:1234:1234").Result(),
			expectedPhase: v1.UploadPhaseCompleted,
			expectedErr:   nil,
			cleanupFail:   false,
		},
		{
			name:          "Invalid peID should fail upload ",
			key:           "velero/upload-1",
			upload:        defaultUpload().Phase(v1.UploadPhaseNew).SnapshotID("ivd:invalid").Result(),
			expectedPhase: v1.UploadPhaseUploadError,
			expectedErr:   errors.New("Failed to get PEID from SnapshotID, ivd:invalid"),
			cleanupFail:   false,
		},
		{
			name:          "Upload fail when copying to remote repository",
			key:           "velero/upload-1",
			upload:        defaultUpload().Phase(v1.UploadPhaseNew).SnapshotID("ivd:1234:1234").Result(),
			expectedPhase: v1.UploadPhaseUploadError,
			expectedErr:   errors.New("Failed at copying to remote repository"),
			cleanupFail:   false,
		},
		{
			name:          "Clean up local snapshot fail",
			key:           "velero/upload-1",
			upload:        defaultUpload().Phase(v1.UploadPhaseNew).SnapshotID("ivd:1234:1234").Result(),
			expectedPhase: v1.UploadPhaseCleanupFailed,
			expectedErr:   errors.New("Failed to delete the snapshot ivd:1234:1234"),
			cleanupFail:   true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var (
				clientset       = fake.NewSimpleClientset(test.upload)
				sharedInformers = informers.NewSharedInformerFactory(clientset, 0)
				logger          = veleroplugintest.NewLogger()
				kubeClient      = kubefake.NewSimpleClientset()
			)

			c := &uploadController{
				genericController: newGenericController("upload-test", logger),
				kubeClient:        kubeClient,
				uploadClient:      clientset.VeleropluginV1(),
				uploadLister:      sharedInformers.Veleroplugin().V1().Uploads().Lister(),
				nodeName:          "upload-test",
				clock:             &clock.RealClock{},
				dataMover:         &dataMover.DataMover{},
				snapMgr:           &snapshotmgr.SnapshotManager{},
			}
			require.NoError(t, sharedInformers.Veleroplugin().V1().Uploads().Informer().GetStore().Add(test.upload))
			if test.cleanupFail {
				patches := gomonkey.ApplyMethod(reflect.TypeOf(c.dataMover), "CopyToRepo", func(_ *dataMover.DataMover, _ astrolabe.ProtectedEntityID) (astrolabe.ProtectedEntityID, error) {
					return astrolabe.ProtectedEntityID{}, nil
				})
				defer patches.Reset()
				patches.ApplyMethod(reflect.TypeOf(c.dataMover), "UnregisterOngoingUpload", func(_ *dataMover.DataMover, _ astrolabe.ProtectedEntityID) {
				})
				defer patches.Reset()
				patches.ApplyMethod(reflect.TypeOf(c.snapMgr), "DeleteLocalSnapshot", func(_ *snapshotmgr.SnapshotManager, _ astrolabe.ProtectedEntityID) error {
					return test.expectedErr
				})
			} else {
				patches := gomonkey.ApplyMethod(reflect.TypeOf(c.dataMover), "CopyToRepo", func(_ *dataMover.DataMover, _ astrolabe.ProtectedEntityID) (astrolabe.ProtectedEntityID, error) {
					return astrolabe.ProtectedEntityID{}, test.expectedErr
				})
				patches.ApplyMethod(reflect.TypeOf(c.dataMover), "UnregisterOngoingUpload", func(_ *dataMover.DataMover, _ astrolabe.ProtectedEntityID) {
				})
				defer patches.Reset()
				// UploadPhaseCompleted case
				if test.expectedErr == nil {
					patches.ApplyMethod(reflect.TypeOf(c.snapMgr), "DeleteLocalSnapshot", func(_ *snapshotmgr.SnapshotManager, _ astrolabe.ProtectedEntityID) error {
						return nil
					})
				}
			}
			c.processUploadFunc = c.processUpload
			err := c.processUploadItem(test.key)
			assert.Equal(t, test.expectedErr == nil, err == nil)
			res, err := c.uploadClient.Uploads(test.upload.Namespace).Get(context.TODO(), test.upload.Name, metav1.GetOptions{})
			assert.Nil(t, err)
			assert.Equal(t, test.expectedPhase, res.Status.Phase)
		})
	}
}

func TestPatchUploadByStatus(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		oldPhase v1.UploadPhase
		newPhase v1.UploadPhase
		upload   *v1.Upload
		msg      string
	}{
		{
			name:     "Test New to Inprogress",
			key:      "velero/upload-1",
			oldPhase: v1.UploadPhaseNew,
			newPhase: v1.UploadPhaseInProgress,
			upload:   defaultUpload().Phase(v1.UploadPhaseNew).Result(),
			msg:      "",
		},
		{
			name:     "Test InProgress to Completed",
			key:      "velero/upload-1",
			oldPhase: v1.UploadPhaseInProgress,
			newPhase: v1.UploadPhaseCompleted,
			upload:   defaultUpload().Phase(v1.UploadPhaseInProgress).Retry(constants.MIN_RETRY).Result(),
			msg:      "Upload completed",
		},
		{
			name:     "Test InProgress to UploadError",
			key:      "velero/upload-1",
			oldPhase: v1.UploadPhaseInProgress,
			newPhase: v1.UploadPhaseUploadError,
			upload:   defaultUpload().Phase(v1.UploadPhaseInProgress).Retry(constants.MIN_RETRY).Result(),
			msg:      "Failed to upload snapshot, ivd:1234:1234, to durable object storage.",
		},
		{
			name:     "Test InProgress to CleanupFailed",
			key:      "velero/upload-1",
			oldPhase: v1.UploadPhaseInProgress,
			newPhase: v1.UploadPhaseCleanupFailed,
			upload:   defaultUpload().Phase(v1.UploadPhaseInProgress).Retry(constants.MIN_RETRY).Result(),
			msg:      "Failed to clean up local snapshot after uploading snapshot, ivd:1234:1234",
		},
		{
			name:     "Test max backoff for retry",
			key:      "velero/upload-1",
			oldPhase: v1.UploadPhaseInProgress,
			newPhase: v1.UploadPhaseUploadError,
			upload:   defaultUpload().Phase(v1.UploadPhaseInProgress).Retry(constants.RETRY_WARNING_COUNT).Result(),
			msg:      "Failed to upload snapshot, ivd:1234:1234, to durable object storage.",
		},
		{
			name:     "Test UploadError to Inprogress",
			key:      "velero/upload-1",
			oldPhase: v1.UploadPhaseUploadError,
			newPhase: v1.UploadPhaseInProgress,
			upload:   defaultUpload().Phase(v1.UploadPhaseUploadError).Result(),
			msg:      "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var (
				client          = fake.NewSimpleClientset(test.upload)
				sharedInformers = informers.NewSharedInformerFactory(client, 0)
				logger          = veleroplugintest.NewLogger()
				kubeClient      = kubefake.NewSimpleClientset()
			)

			c := &uploadController{
				genericController: newGenericController("upload-test", logger),
				kubeClient:        kubeClient,
				uploadClient:      client.VeleropluginV1(),
				uploadLister:      sharedInformers.Veleroplugin().V1().Uploads().Lister(),
				nodeName:          "upload-test",
				clock:             &clock.RealClock{},
				dataMover:         &dataMover.DataMover{},
				snapMgr:           &snapshotmgr.SnapshotManager{},
			}

			if test.upload != nil {
				require.NoError(t, sharedInformers.Veleroplugin().V1().Uploads().Informer().GetStore().Add(test.upload))
				// this is necessary so the Patch() call returns the appropriate object
				client.PrependReactor("patch", "uploads", func(action core.Action) (bool, runtime.Object, error) {
					var (
						patch    = action.(core.PatchAction).GetPatch()
						patchMap = make(map[string]interface{})
						res      = test.upload.DeepCopy()
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
					res.Status.Phase = v1.UploadPhase(phase)

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

					backoff, found, err := unstructured.NestedString(patchMap, "status", "currentbackoff")
					if err == nil && found {
						curBackoff, err := strconv.ParseInt(backoff, 10, 32)
						if err != nil {
							t.Logf("error converting status.currentbackoff: %v\n", err)
							return false, nil, err
						}
						res.Status.CurrentBackOff = int32(curBackoff)
					}

					return true, res, nil
				})
			}

			var oldRetry int32
			if test.upload.Status.Phase != v1.UploadPhaseNew {
				oldRetry = test.upload.Status.RetryCount
			}
			res, err := c.patchUploadByStatus(test.upload, test.newPhase, test.msg)
			require.NoError(t, err)
			require.Equal(t, test.newPhase, res.Status.Phase)
			require.Equal(t, test.msg, res.Status.Message)
			if test.oldPhase == v1.UploadPhaseNew {
				require.Equal(t, int32(constants.MIN_RETRY), res.Status.RetryCount)
			}
			if test.newPhase == v1.UploadPhaseUploadError {
				newRetry := res.Status.RetryCount
				require.Equal(t, oldRetry+1, newRetry)
				require.LessOrEqual(t, res.Status.CurrentBackOff, int32(constants.UPLOAD_MAX_BACKOFF))
			}
		})
	}
}

func TestUploadErrorRetry(t *testing.T) {
	tests := []struct {
		name          string
		key           string
		retry         int32
		upload        *v1.Upload
		expectedPhase v1.UploadPhase
		expectedErr   error
	}{
		{
			name:          "Test retry for upload with uploaderror",
			key:           "velero/upload-1",
			retry:         constants.MIN_RETRY,
			upload:        defaultUpload().Phase(v1.UploadPhaseInProgress).SnapshotID("ivd:1234:1234").Retry(0).Result(),
			expectedPhase: v1.UploadPhaseCompleted,
			expectedErr:   errors.New("Failed to upload snapshot, ivd:1234:1234, to durable object storage. Failed at copying to remote repository"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var (
				client          = fake.NewSimpleClientset(test.upload)
				sharedInformers = informers.NewSharedInformerFactory(client, 0)
				logger          = veleroplugintest.NewLogger()
				kubeClient      = kubefake.NewSimpleClientset()
			)

			c := &uploadController{
				genericController: newGenericController("upload-test", logger),
				kubeClient:        kubeClient,
				uploadClient:      client.VeleropluginV1(),
				uploadLister:      sharedInformers.Veleroplugin().V1().Uploads().Lister(),
				nodeName:          "upload-test",
				clock:             &clock.RealClock{},
				dataMover:         &dataMover.DataMover{},
				snapMgr:           &snapshotmgr.SnapshotManager{},
			}

			// First time set Inprogress to UploadError
			require.NoError(t, sharedInformers.Veleroplugin().V1().Uploads().Informer().GetStore().Add(test.upload))
			patches := gomonkey.ApplyMethod(reflect.TypeOf(c.dataMover), "CopyToRepo", func(_ *dataMover.DataMover, _ astrolabe.ProtectedEntityID) (astrolabe.ProtectedEntityID, error) {
				return astrolabe.ProtectedEntityID{}, errors.New("Failed at copying to remote repository")
			})
			defer patches.Reset()
			c.processUploadFunc = c.processUpload
			err := c.processUploadItem(test.key)
			require.Equal(t, err.Error(), test.expectedErr.Error())

			// Retry for second time, set to completed at this time
			require.NoError(t, sharedInformers.Veleroplugin().V1().Uploads().Informer().GetStore().Add(test.upload))
			patches.ApplyMethod(reflect.TypeOf(c.dataMover), "CopyToRepo", func(_ *dataMover.DataMover, _ astrolabe.ProtectedEntityID) (astrolabe.ProtectedEntityID, error) {
				return astrolabe.ProtectedEntityID{}, nil
			})

			patches.ApplyMethod(reflect.TypeOf(c.dataMover), "UnregisterOngoingUpload", func(_ *dataMover.DataMover, _ astrolabe.ProtectedEntityID) {
			})

			patches.ApplyMethod(reflect.TypeOf(c.snapMgr), "DeleteLocalSnapshot", func(_ *snapshotmgr.SnapshotManager, _ astrolabe.ProtectedEntityID) error {
				return nil
			})

			err = c.processUploadItem(test.key)
			require.Nil(t, err)
			res, err := c.uploadClient.Uploads(test.upload.Namespace).Get(context.TODO(), test.upload.Name, metav1.GetOptions{})
			require.Nil(t, err)
			require.Equal(t, test.expectedPhase, res.Status.Phase)
			require.Equal(t, test.retry+1, res.Status.RetryCount)
		})
	}
}
