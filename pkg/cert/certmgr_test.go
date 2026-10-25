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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/caddyserver/certmagic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestCertMgrOnEventReturnsSecretUpdateError(t *testing.T) {
	const (
		domain       = "example.com"
		privateKey   = "test-private-key"
		privateKeyID = "certificates/acme-v02.api.letsencrypt.org-directory/example.com/example.com.key"
		certID       = "certificates/acme-v02.api.letsencrypt.org-directory/example.com/example.com.crt"
		updateError  = "forced secret update failure"
	)
	certificate := newTestCertificate(t, domain)
	existing := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testSecretName,
			Namespace: testSecretNamespace,
			Annotations: map[string]string{
				certDomainAnnotation: domain,
			},
		},
		Type: corev1.SecretTypeTLS,
		Data: map[string][]byte{
			"tls.crt": []byte("old-cert"),
			"tls.key": []byte("old-key"),
		},
	}
	client := fake.NewSimpleClientset(existing)
	storage, err := NewConfigmapStorage(testSecretNamespace, client)
	require.NoError(t, err)
	require.NoError(t, storage.Store(context.Background(), privateKeyID, []byte(privateKey)))
	require.NoError(t, storage.Store(context.Background(), certID, certificate))

	client.PrependReactor("update", "secrets", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New(updateError)
	})

	configMgr, err := NewConfigMgr(testSecretNamespace, client)
	require.NoError(t, err)
	configMgr.SetConfig(&Config{
		CredentialConfig: []CredentialEntry{
			{
				Domains:   []string{domain},
				TLSIssuer: IssuerTypeLetsencrypt,
				TLSSecret: testSecretName,
			},
		},
	})
	secretMgr, err := NewSecretMgr(testSecretNamespace, client)
	require.NoError(t, err)
	certMgr := &CertMgr{
		cfg:       &certmagic.Config{Storage: storage},
		secretMgr: secretMgr,
		configMgr: configMgr,
	}

	err = certMgr.OnEvent(context.Background(), EventCertObtained, map[string]any{
		"identifier":       domain,
		"renewal":          false,
		"private_key_path": privateKeyID,
		"certificate_path": certID,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), updateError)

	got, err := client.CoreV1().Secrets(testSecretNamespace).Get(context.Background(), testSecretName, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, existing.Data, got.Data)
}

func newTestCertificate(t *testing.T, domain string) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: domain},
		DNSNames:     []string{domain},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}
