// Copyright (c) 2022 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package secret

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/alibaba/higress/v2/pkg/ingress/kube/common"

	kubeclient "istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/test/util/retry"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"

	"github.com/alibaba/higress/v2/pkg/ingress/kube/util"
)

const (
	secretFakeName     = "fake-secret"
	secretFakeKey      = "fake-key"
	secretInitValue    = "init-value"
	secretUpdatedValue = "updated-value"
)

var period = time.Second

func TestController(t *testing.T) {
	client := kubeclient.NewFakeClient()
	ctrl := NewController(client, common.Options{ClusterId: "fake-cluster"})

	stop := make(chan struct{})
	t.Cleanup(func() {
		close(stop)
	})

	client.RunAndWait(stop)

	// store secret
	store := sync.Map{}

	// add event handler
	ctrl.AddEventHandler(func(name util.ClusterNamespacedName) {
		t.Logf("event received, clusterId: %s, namespacedName: %s", name.ClusterId, name.NamespacedName.String())

		retry.UntilSuccessOrFail(t, func() error {
			secret, err := ctrl.Lister().Secrets(name.NamespacedName.Namespace).Get(name.NamespacedName.Name)
			if err != nil && !kerrors.IsNotFound(err) {
				t.Logf("get secret %s error: %v", name.NamespacedName.String(), err)
				return err
			}
			store.Store(name.NamespacedName.String(), secret.Data)
			return nil
		})
	})

	// start controller
	go ctrl.Run(stop)

	// wait for cache sync
	cache.WaitForCacheSync(stop, ctrl.Informer().HasSynced)

	// init secret
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: secretFakeName,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			secretFakeKey: []byte(secretInitValue),
		},
	}

	testCases := []struct {
		name   string
		do     func() error
		expect string
	}{
		{
			name: "create secret",
			do: func() error {
				_, err := client.Kube().CoreV1().Secrets(metav1.NamespaceDefault).Create(context.Background(),
					secret, metav1.CreateOptions{})
				return err
			},
			expect: secretInitValue,
		},
		{
			name: "update secret",
			do: func() error {
				var getSecret *corev1.Secret
				// get or create secret
				getSecret, err := ctrl.Lister().Secrets(metav1.NamespaceDefault).Get(secretFakeName)
				if err != nil {
					if !kerrors.IsNotFound(err) {
						return err
					}
					getSecret, err = client.Kube().CoreV1().Secrets(metav1.NamespaceDefault).Create(context.Background(),
						secret, metav1.CreateOptions{})
					if err != nil {
						return err
					}
				}
				// update secret
				getSecret.Data[secretFakeKey] = []byte(secretUpdatedValue)
				_, err = client.Kube().CoreV1().Secrets(metav1.NamespaceDefault).Update(context.Background(),
					getSecret, metav1.UpdateOptions{})
				return err
			},
			expect: secretUpdatedValue,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if err := testCase.do(); err != nil {
				t.Fatalf("do %s error: %v", testCase.name, err)
			}

			// controller Run() with setting period time to 1s.
			time.Sleep(period)

			secretFullName := types.NamespacedName{
				Namespace: metav1.NamespaceDefault,
				Name:      secretFakeName,
			}.String()

			valAny, ok := store.Load(secretFullName)
			if !ok {
				t.Fatalf("secret %s not found", secretFullName)
			}

			val, ok := valAny.(map[string][]byte)
			if !ok {
				t.Fatalf("assert secret %s data type error", secretFullName)
			}

			if !reflect.DeepEqual(val[secretFakeKey], []byte(testCase.expect)) {
				t.Fatalf("secret %s data error, expect: %s, got: %s",
					secretFullName, testCase.expect, string(val[secretFakeKey]))
			}
		})
	}
}
