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
	"fmt"
	"strconv"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	certDomainAnnotation = "higress.io/cert-domain"

	certNotAfterAnnotation  = "higress.io/cert-notAfter"
	certNotBeforeAnnotation = "higress.io/cert-notBefore"
	certRenewAnnotation     = "higress.io/cert-renew"
	certRenewTimeAnnotation = "higress.io/cert-renew-time"
	certSourceAnnotation    = "higress.io/cert-source"
)

type SecretMgr struct {
	client    kubernetes.Interface
	namespace string
}

func NewSecretMgr(namespace string, client kubernetes.Interface) (*SecretMgr, error) {
	secretMgr := &SecretMgr{
		namespace: namespace,
		client:    client,
	}

	return secretMgr, nil
}

func (s *SecretMgr) Update(domain string, secretName string, privateKey []byte, certificate []byte, notBefore time.Time, notAfter time.Time, isRenew bool) error {
	CertLog.Infof("update secret, domain:%s, secretName:%s, notBefore:%v, notAfter:%v, isRenew:%t", domain, secretName, notBefore, notAfter, isRenew)
	name := secretName
	namespace := s.namespace
	namespaceP, secretP := ParseTLSSecret(secretName)
	if namespaceP != "" {
		namespace = namespaceP
		name = secretP
	}

	existingSecret, err := s.client.CoreV1().Secrets(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			// create secret
			_, createErr := s.client.CoreV1().Secrets(namespace).Create(context.Background(), s.constructSecret(domain, name, namespace, privateKey, certificate, notBefore, notAfter, isRenew), metav1.CreateOptions{})
			return createErr
		}
		return err
	}

	// Only update Secrets that Higress already owns for this exact domain. Checking the
	// fetched object (rather than the freshly built one) prevents overwriting a
	// user-managed Secret whose name happens to appear in the credential config.
	if existingSecret.Annotations[certDomainAnnotation] != domain {
		return fmt.Errorf("the secret name %s is not automatic https secret name for the domain:%s, please rename it in config", secretName, domain)
	}

	// Mutate the existing object instead of replacing it so labels, owner references
	// and user annotations survive certificate updates.
	if existingSecret.Annotations == nil {
		existingSecret.Annotations = make(map[string]string)
	}
	for key, value := range certAnnotations(domain, notBefore, notAfter, isRenew) {
		existingSecret.Annotations[key] = value
	}
	if !isRenew {
		delete(existingSecret.Annotations, certRenewTimeAnnotation)
	}
	if existingSecret.Data == nil {
		existingSecret.Data = make(map[string][]byte)
	}
	existingSecret.Data["tls.key"] = privateKey
	existingSecret.Data["tls.crt"] = certificate

	_, err = s.client.CoreV1().Secrets(namespace).Update(context.Background(), existingSecret, metav1.UpdateOptions{})
	return err
}

func (s *SecretMgr) constructSecret(domain string, name string, namespace string, privateKey []byte, certificate []byte, notBefore time.Time, notAfter time.Time, isRenew bool) *v1.Secret {
	// Required fields:
	// - Secret.Data["tls.key"] - TLS private key.
	//   Secret.Data["tls.crt"] - TLS certificate.
	dataMap := make(map[string][]byte, 0)
	dataMap["tls.key"] = privateKey
	dataMap["tls.crt"] = certificate
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Annotations: certAnnotations(domain, notBefore, notAfter, isRenew),
		},
		Type: v1.SecretTypeTLS,
		Data: dataMap,
	}
	return secret
}

func certAnnotations(domain string, notBefore time.Time, notAfter time.Time, isRenew bool) map[string]string {
	annotations := map[string]string{
		certDomainAnnotation:    domain,
		certNotAfterAnnotation:  notAfter.Format("2006-01-02 15:04:05"),
		certNotBeforeAnnotation: notBefore.Format("2006-01-02 15:04:05"),
		certRenewAnnotation:     strconv.FormatBool(isRenew),
		certSourceAnnotation:    string(IssuerTypeLetsencrypt),
	}
	if isRenew {
		annotations[certRenewTimeAnnotation] = time.Now().Format("2006-01-02 15:04:05")
	}
	return annotations
}
