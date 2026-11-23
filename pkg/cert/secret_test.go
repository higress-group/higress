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

package cert

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

const (
	testSecretNamespace = "higress-system"
	testSecretName      = "example-tls"
	testSecretDomain    = "example.com"
)

func TestSecretMgrUpdateRejectsUnmanagedSecret(t *testing.T) {
	userData := []byte("user-managed-tls-data")
	existing := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        testSecretName,
			Namespace:   testSecretNamespace,
			Labels:      map[string]string{"app": "user"},
			Annotations: map[string]string{"user/annotation": "keep"},
		},
		Type: corev1.SecretTypeTLS,
		Data: map[string][]byte{
			"tls.crt": userData,
			"tls.key": userData,
		},
	}
	client := fake.NewSimpleClientset(existing)
	secretMgr, err := NewSecretMgr(testSecretNamespace, client)
	require.NoError(t, err)

	err = secretMgr.Update(testSecretDomain, testSecretName, []byte("new-key"), []byte("new-cert"), time.Now(), time.Now().Add(time.Hour), false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not automatic https secret name")

	got, err := client.CoreV1().Secrets(testSecretNamespace).Get(context.Background(), testSecretName, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, existing.Data, got.Data)
	assert.Equal(t, existing.Annotations, got.Annotations)
	assert.Equal(t, existing.Labels, got.Labels)
}

func TestSecretMgrUpdateRejectsSecretForAnotherDomain(t *testing.T) {
	existing := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testSecretName,
			Namespace: testSecretNamespace,
			Annotations: map[string]string{
				certDomainAnnotation: "other.example.com",
			},
		},
		Type: corev1.SecretTypeTLS,
		Data: map[string][]byte{
			"tls.crt": []byte("old-cert"),
			"tls.key": []byte("old-key"),
		},
	}
	client := fake.NewSimpleClientset(existing)
	secretMgr, err := NewSecretMgr(testSecretNamespace, client)
	require.NoError(t, err)

	err = secretMgr.Update(testSecretDomain, testSecretName, []byte("new-key"), []byte("new-cert"), time.Now(), time.Now().Add(time.Hour), false)
	require.Error(t, err)

	got, err := client.CoreV1().Secrets(testSecretNamespace).Get(context.Background(), testSecretName, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, existing.Data, got.Data)
	assert.Equal(t, existing.Annotations, got.Annotations)
}

func TestSecretMgrUpdatePreservesManagedSecretMetadata(t *testing.T) {
	notBefore := time.Date(2026, time.October, 25, 8, 0, 0, 0, time.UTC)
	notAfter := notBefore.Add(90 * 24 * time.Hour)
	owner := metav1.OwnerReference{
		APIVersion: "apps/v1",
		Kind:       "Deployment",
		Name:       "owner",
		UID:        "owner-uid",
	}
	existing := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testSecretName,
			Namespace: testSecretNamespace,
			Labels:    map[string]string{"app": "higress"},
			Annotations: map[string]string{
				certDomainAnnotation:    testSecretDomain,
				certRenewTimeAnnotation: "2026-01-01 00:00:00",
				"user/annotation":       "keep",
			},
			OwnerReferences: []metav1.OwnerReference{owner},
		},
		Type: corev1.SecretTypeTLS,
		Data: map[string][]byte{
			"tls.crt": []byte("old-cert"),
			"tls.key": []byte("old-key"),
			"extra":   []byte("keep"),
		},
	}
	client := fake.NewSimpleClientset(existing)
	secretMgr, err := NewSecretMgr(testSecretNamespace, client)
	require.NoError(t, err)

	err = secretMgr.Update(testSecretDomain, testSecretName, []byte("new-key"), []byte("new-cert"), notBefore, notAfter, false)
	require.NoError(t, err)

	got, err := client.CoreV1().Secrets(testSecretNamespace).Get(context.Background(), testSecretName, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, []byte("new-key"), got.Data["tls.key"])
	assert.Equal(t, []byte("new-cert"), got.Data["tls.crt"])
	assert.Equal(t, []byte("keep"), got.Data["extra"])
	assert.Equal(t, existing.Labels, got.Labels)
	assert.Equal(t, []metav1.OwnerReference{owner}, got.OwnerReferences)
	assert.Equal(t, "keep", got.Annotations["user/annotation"])
	assert.Equal(t, testSecretDomain, got.Annotations[certDomainAnnotation])
	assert.Equal(t, notBefore.Format("2006-01-02 15:04:05"), got.Annotations[certNotBeforeAnnotation])
	assert.Equal(t, notAfter.Format("2006-01-02 15:04:05"), got.Annotations[certNotAfterAnnotation])
	assert.Equal(t, "false", got.Annotations[certRenewAnnotation])
	assert.NotContains(t, got.Annotations, certRenewTimeAnnotation)
	assert.Equal(t, corev1.SecretTypeTLS, got.Type)
}
