// Copyright (c) 2022 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gateway

import (
	"os"
	"testing"

	gatewaycommon "github.com/alibaba/higress/v2/test/gateway/common"
	"sigs.k8s.io/gateway-api/conformance"
	"sigs.k8s.io/gateway-api/conformance/utils/roundtripper"
)

func TestGatewayAPIConformance(t *testing.T) {
	opts := conformance.DefaultOptions(t)
	if os.Getenv(gatewaycommon.DialLocalhostEnv) == "true" {
		dialer := gatewaycommon.NewLocalGatewayDialer()
		t.Cleanup(dialer.Close)
		opts.RoundTripper = &roundtripper.DefaultRoundTripper{
			Debug:             opts.Debug,
			TimeoutConfig:     opts.TimeoutConfig,
			CustomDialContext: dialer.DialContext,
		}
	}
	conformance.RunConformanceWithOptions(t, opts)
}
