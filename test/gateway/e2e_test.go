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

package gateway

import (
	"context"
	"net"
	"os"
	"testing"

	"sigs.k8s.io/gateway-api/conformance"
	"sigs.k8s.io/gateway-api/conformance/utils/roundtripper"
)

const dialLocalhostEnv = "HIGRESS_GATEWAY_API_TEST_DIAL_LOCALHOST"
const localHTTPPortEnv = "HIGRESS_GATEWAY_API_TEST_LOCAL_HTTP_PORT"
const localHTTPSPortEnv = "HIGRESS_GATEWAY_API_TEST_LOCAL_HTTPS_PORT"

func TestGatewayAPIConformance(t *testing.T) {
	opts := conformance.DefaultOptions(t)
	if os.Getenv(dialLocalhostEnv) == "true" {
		opts.RoundTripper = &roundtripper.DefaultRoundTripper{
			Debug:         opts.Debug,
			TimeoutConfig: opts.TimeoutConfig,
			CustomDialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
				_, port, err := net.SplitHostPort(address)
				if err != nil {
					return nil, err
				}
				switch port {
				case "80":
					if localPort := os.Getenv(localHTTPPortEnv); localPort != "" {
						port = localPort
					}
				case "443":
					if localPort := os.Getenv(localHTTPSPortEnv); localPort != "" {
						port = localPort
					}
				}
				var dialer net.Dialer
				return dialer.DialContext(ctx, network, net.JoinHostPort("127.0.0.1", port))
			},
		}
	}
	conformance.RunConformanceWithOptions(t, opts)
}
