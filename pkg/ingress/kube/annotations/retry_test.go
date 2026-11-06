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
	"reflect"
	"testing"

	"github.com/golang/protobuf/ptypes/duration"
	"github.com/stretchr/testify/assert"
	networking "istio.io/api/networking/v1alpha3"
)

func TestRetryParse(t *testing.T) {
	retry := retry{}
	inputCases := []struct {
		input  map[string]string
		expect *RetryConfig
	}{
		{},
		{
			input: map[string]string{
				buildNginxAnnotationKey(retryCount): "1",
			},
			expect: &RetryConfig{
				retryCount:      1,
				retryOn:         "5xx",
				perRetryTimeout: &duration.Duration{},
			},
		},
		{
			input: map[string]string{
				buildNginxAnnotationKey(perRetryTimeout): "10",
			},
			expect: &RetryConfig{
				retryCount: 3,
				perRetryTimeout: &duration.Duration{
					Seconds: 10,
				},
				retryOn: "5xx",
			},
		},
		{
			input: map[string]string{
				buildNginxAnnotationKey(retryCount): "2",
				buildNginxAnnotationKey(retryOn):    "off",
			},
			expect: &RetryConfig{
				retryCount:      0,
				retryOn:         "5xx",
				perRetryTimeout: &duration.Duration{},
			},
		},
		{
			input: map[string]string{
				buildNginxAnnotationKey(retryCount): "2",
				buildNginxAnnotationKey(retryOn):    "error,timeout",
			},
			expect: &RetryConfig{
				retryCount:      2,
				retryOn:         "5xx",
				perRetryTimeout: &duration.Duration{},
			},
		},
		{
			input: map[string]string{
				buildNginxAnnotationKey(retryCount): "2",
				buildNginxAnnotationKey(retryOn):    "error  timeout",
			},
			expect: &RetryConfig{
				retryCount:      2,
				retryOn:         "5xx",
				perRetryTimeout: &duration.Duration{},
			},
		},
		{
			input: map[string]string{
				buildNginxAnnotationKey(retryOn): "timeout,non_idempotent",
			},
			expect: &RetryConfig{
				retryCount:      3,
				retryOn:         "5xx,non_idempotent",
				perRetryTimeout: &duration.Duration{},
			},
		},
		{
			input: map[string]string{
				buildNginxAnnotationKey(retryOn): "timeout non_idempotent",
			},
			expect: &RetryConfig{
				retryCount:      3,
				retryOn:         "5xx,non_idempotent",
				perRetryTimeout: &duration.Duration{},
			},
		},
		{
			input: map[string]string{
				buildNginxAnnotationKey(retryOn): "timeout,http_503,http_502,http_404",
			},
			expect: &RetryConfig{
				retryCount:      3,
				retryOn:         "5xx,retriable-status-codes,503,502,404",
				perRetryTimeout: &duration.Duration{},
			},
		},
		{
			input: map[string]string{
				buildNginxAnnotationKey(retryOn): "timeout http_503  http_502 http_404",
			},
			expect: &RetryConfig{
				retryCount:      3,
				retryOn:         "5xx,retriable-status-codes,503,502,404",
				perRetryTimeout: &duration.Duration{},
			},
		},
	}

	for _, inputCase := range inputCases {
		t.Run("", func(t *testing.T) {
			config := &Ingress{}
			_ = retry.Parse(inputCase.input, config, nil)
			assert.Equal(t, inputCase.expect, config.Retry)
		})
	}
}

func TestRetryApplyRoute(t *testing.T) {
	retry := retry{}
	inputCases := []struct {
		config *Ingress
		input  *networking.HTTPRoute
		expect *networking.HTTPRoute
	}{
		{
			config: &Ingress{},
			input:  &networking.HTTPRoute{},
			expect: &networking.HTTPRoute{},
		},
		{
			config: &Ingress{
				Retry: &RetryConfig{
					retryCount: 3,
					retryOn:    "test",
				},
			},
			input: &networking.HTTPRoute{},
			expect: &networking.HTTPRoute{
				Retries: &networking.HTTPRetry{
					Attempts: 3,
					RetryOn:  "test",
				},
			},
		},
	}

	for _, inputCase := range inputCases {
		t.Run("", func(t *testing.T) {
			retry.ApplyRoute(inputCase.input, inputCase.config)
			if !reflect.DeepEqual(inputCase.input, inputCase.expect) {
				t.Fatalf("Should be equal")
			}
		})
	}
}

func TestRetryParseUnknownCondition(t *testing.T) {
	config := &Ingress{}
	annotations := Annotations{
		buildNginxAnnotationKey(retryOn): "bogus",
	}
	assert.NoError(t, retry{}.Parse(annotations, config, nil))
	if assert.NotNil(t, config.Retry) {
		assert.Equal(t, defaultRetryOn, config.Retry.retryOn, "an unrecognised condition must keep the default retry policy")
		assert.Equal(t, int32(defaultRetryCount), config.Retry.retryCount)
	}
}
