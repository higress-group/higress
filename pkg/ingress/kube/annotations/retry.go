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

import (
	"strings"

	"github.com/golang/protobuf/ptypes/duration"
	networking "istio.io/api/networking/v1alpha3"
)

const (
	retryCount      = "proxy-next-upstream-tries"
	perRetryTimeout = "proxy-next-upstream-timeout"
	retryOn         = "proxy-next-upstream"

	defaultRetryCount = 3
	defaultRetryOn    = "5xx"
	retryStatusCode   = "retriable-status-codes"
)

var (
	_ Parser       = retry{}
	_ RouteHandler = retry{}
)

type RetryConfig struct {
	retryCount      int32
	perRetryTimeout *duration.Duration
	retryOn         string
}

type retry struct{}

func (r retry) Parse(annotations Annotations, config *Ingress, _ *GlobalContext) error {
	if !needRetryConfig(annotations) {
		return nil
	}

	retryConfig := &RetryConfig{
		retryCount:      defaultRetryCount,
		perRetryTimeout: &duration.Duration{},
		retryOn:         defaultRetryOn,
	}
	defer func() {
		config.Retry = retryConfig
	}()

	if count, err := annotations.ParseInt32ASAP(retryCount); err == nil {
		retryConfig.retryCount = count
	}

	if timeout, err := annotations.ParseIntASAP(perRetryTimeout); err == nil {
		retryConfig.perRetryTimeout = &duration.Duration{
			Seconds: int64(timeout),
		}
	}

	if retryOn, err := annotations.ParseStringASAP(retryOn); err == nil {
		var retryOnConditions []string
		if strings.Contains(retryOn, ",") {
			retryOnConditions = splitBySeparator(retryOn, ",")
		} else {
			retryOnConditions = strings.Fields(retryOn)
		}
		conditions := toSet(retryOnConditions)
		if len(conditions) > 0 {
			if conditions.Contains("off") {
				retryConfig.retryCount = 0
			} else {
				var stringBuilder strings.Builder
				// Convert error, timeout, invalid_header to 5xx
				if conditions.Contains("error") ||
					conditions.Contains("timeout") ||
					conditions.Contains("invalid_header") {
					stringBuilder.WriteString(defaultRetryOn + ",")
				}
				// Just use the raw.
				if conditions.Contains("non_idempotent") {
					stringBuilder.WriteString("non_idempotent,")
				}
				// Append the status codes.
				statusCodes := convertStatusCodes(retryOnConditions)
				if len(statusCodes) > 0 {
					stringBuilder.WriteString(retryStatusCode + ",")
					for _, code := range statusCodes {
						stringBuilder.WriteString(code + ",")
					}
				}

				// A value that contains none of the recognised conditions used to
				// leave retryOn empty, which silently disables retries while
				// Attempts keeps its default of 3.
				if built := strings.TrimSuffix(stringBuilder.String(), ","); built != "" {
					retryConfig.retryOn = built
				}
			}
		}
	}

	return nil
}

func (r retry) ApplyRoute(route *networking.HTTPRoute, config *Ingress) {
	retryConfig := config.Retry
	if retryConfig == nil {
		return
	}

	route.Retries = &networking.HTTPRetry{
		Attempts:      retryConfig.retryCount,
		PerTryTimeout: retryConfig.perRetryTimeout,
		RetryOn:       retryConfig.retryOn,
	}
}

func needRetryConfig(annotations Annotations) bool {
	return annotations.HasASAP(retryCount) ||
		annotations.HasASAP(perRetryTimeout) ||
		annotations.HasASAP(retryOn)
}

func convertStatusCodes(statusCodes []string) []string {
	var result []string
	for _, statusCode := range statusCodes {
		if strings.HasPrefix(statusCode, "http_") {
			result = append(result, strings.TrimPrefix(statusCode, "http_"))
		}
	}
	return result
}
