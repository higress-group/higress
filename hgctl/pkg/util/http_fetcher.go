// Copyright (c) 2022 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package util

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	defaultInitialInterval = 500 * time.Millisecond
	defaultMaxInterval     = 60 * time.Second
)

type HTTPFetcher struct {
	client          *http.Client
	initialBackoff  time.Duration
	requestMaxRetry int
	bufferSize      int64
}

// NewHTTPFetcher create a new HTTP remote fetcher.
func NewHTTPFetcher(requestTimeout time.Duration, requestMaxRetry int, bufferSize int64) *HTTPFetcher {
	if requestTimeout == 0 {
		requestTimeout = 5 * time.Second
	}
	return &HTTPFetcher{
		client: &http.Client{
			Timeout: requestTimeout,
		},
		initialBackoff:  defaultInitialInterval,
		requestMaxRetry: requestMaxRetry,
		bufferSize:      bufferSize,
	}
}

// Fetch downloads with HTTP get.
func (f *HTTPFetcher) Fetch(ctx context.Context, url string) ([]byte, error) {
	c := f.client
	delayInterval := f.initialBackoff
	attempts := 0
	var lastError error
	for attempts < f.requestMaxRetry {
		attempts++
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		resp, err := c.Do(req)
		if err != nil {
			lastError = err
			if ctx.Err() != nil {
				// If there is context timeout, exit this loop.
				return nil, fmt.Errorf("download failed after %v attempts, last error: %v", attempts, lastError)
			}
			delayInterval = delayInterval + f.initialBackoff
			if delayInterval > defaultMaxInterval {
				break
			}
			time.Sleep(delayInterval)
			continue
		}
		if resp.StatusCode == http.StatusOK {
			body, err := io.ReadAll(io.LimitReader(resp.Body, f.bufferSize))
			if err != nil {
				return nil, err
			}
			err = resp.Body.Close()
			if err != nil {
				return nil, err
			}
			return body, err
		}

		lastError = fmt.Errorf("download request failed: status code %v", resp.StatusCode)

		if retryable(resp.StatusCode) {
			_, err := io.ReadAll(io.LimitReader(resp.Body, f.bufferSize))
			if err != nil {
				return nil, err
			}
			err = resp.Body.Close()
			delayInterval = delayInterval + f.initialBackoff
			if delayInterval > defaultMaxInterval {
				break
			}
			time.Sleep(delayInterval)
			continue
		}

		err = resp.Body.Close()
		break

	}
	return nil, fmt.Errorf("download failed after %v attempts, last error: %v", attempts, lastError)
}

func retryable(code int) bool {
	return code >= 500 &&
		!(code == http.StatusNotImplemented ||
			code == http.StatusHTTPVersionNotSupported ||
			code == http.StatusNetworkAuthenticationRequired)
}
