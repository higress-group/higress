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

package roundtripper

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/net/http2"
)

func TestTransport(t *testing.T) {
	req := Request{
		Protocol: "HTTP/2.0",
	}
	tests := []struct {
		name          string
		req           Request
		prevTransport http.RoundTripper
		tlsConfig     *TLSConfig
		transport     http.RoundTripper
	}{
		{
			name: "default",
			req:  Request{},
		},
		{
			name:      "http2",
			req:       req,
			transport: &http2.Transport{},
		},
		{
			name: "http1",
			req: Request{
				Protocol: "HTTP/1.1",
			},
		},
		{
			name: "https",
			req:  req,
			tlsConfig: &TLSConfig{
				SNI: "www.example.com",
			},
			transport: &http2.Transport{
				TLSClientConfig: &tls.Config{
					RootCAs:    x509.NewCertPool(),
					ServerName: "www.example.com",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := DefaultRoundTripper{}
			c := http.Client{}
			d.initTransport(&c, tt.req.Protocol, tt.tlsConfig)
			assert.Equal(t, tt.transport, c.Transport)
		})
	}
}
