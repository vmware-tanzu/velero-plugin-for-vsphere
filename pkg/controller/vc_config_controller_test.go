/*
Copyright 2026 the Velero contributors.

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
	"testing"

	"github.com/stretchr/testify/assert"
	veleroplugintest "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/test"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

func newTestVcConfigController() *vcConfigController {
	return &vcConfigController{
		genericController: newGenericController("vc-config", veleroplugintest.NewLogger()),
	}
}

func TestEnqueueVcConfigSecret(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "velero",
			Name:      "vc-config",
		},
	}
	secretKey := "velero/vc-config"

	tests := []struct {
		name           string
		obj            interface{}
		expectEnqueued bool
		expectedKey    string
	}{
		{
			name:           "valid secret is enqueued",
			obj:            secret,
			expectEnqueued: true,
			expectedKey:    secretKey,
		},
		{
			name:           "secret wrapped in DeletedFinalStateUnknown is unwrapped and enqueued",
			obj:            cache.DeletedFinalStateUnknown{Key: secretKey, Obj: secret},
			expectEnqueued: true,
			expectedKey:    secretKey,
		},
		{
			name: "typed nil secret does not panic and is not enqueued",
			obj: func() interface{} {
				var nilSecret *corev1.Secret
				return nilSecret
			}(),
			expectEnqueued: false,
		},
		{
			name:           "non-secret object is ignored",
			obj:            "not-a-secret",
			expectEnqueued: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			v := newTestVcConfigController()

			assert.NotPanics(t, func() {
				v.enqueueVcConfigSecret(test.obj)
			})

			if test.expectEnqueued {
				assert.Equal(t, 1, v.queue.Len())
				key, _ := v.queue.Get()
				assert.Equal(t, test.expectedKey, key)
			} else {
				assert.Equal(t, 0, v.queue.Len())
			}
		})
	}
}
