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

package annotations

const sslPassthroughAnnotation = "ssl-passthrough"

var _ Parser = &sslPassthrough{}

type SSLPassthroughConfig struct {
	Enabled bool
}

type sslPassthrough struct{}

func (s sslPassthrough) Parse(annotations Annotations, config *Ingress, _ *GlobalContext) error {
	enabled, err := annotations.ParseBoolASAP(sslPassthroughAnnotation)
	if err != nil {
		return nil
	}
	config.SSLPassthrough = &SSLPassthroughConfig{Enabled: enabled}
	return nil
}
