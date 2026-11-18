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

package provider

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"testing"

	"github.com/stretchr/testify/require"
)

// A service-account key whose private_key is a syntactically valid PKCS#8 key of
// a non-RSA algorithm (here ECDSA P-256) must be rejected with an error, just
// like other unusable keys (undecodable PEM, malformed PKCS#8). createJWT used
// to assert the parsed key to *rsa.PrivateKey unconditionally, which panicked
// the Wasm plugin instead of returning a configuration error.
func TestCreateJWTNonRSAKeyDoesNotPanic(t *testing.T) {
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(ecKey)
	require.NoError(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})

	key := &ServiceAccountKey{
		ClientEmail:  "svc@example.iam.gserviceaccount.com",
		PrivateKeyID: "test-key-id",
		PrivateKey:   string(keyPEM),
		TokenURI:     "https://oauth2.googleapis.com/token",
	}

	token, err := createJWT(key)
	require.Error(t, err)
	require.Empty(t, token)
}
