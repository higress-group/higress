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

package client

import (
	"errors"
	"io"
	"net/http"
	"testing"
	"time"
)

type trackingBody struct {
	closed bool
}

func (b *trackingBody) Read(p []byte) (int, error) { return 0, io.EOF }
func (b *trackingBody) Close() error              { b.closed = true; return nil }

type bodyTransport struct{ body *trackingBody }

func (t *bodyTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Body: t.body}, nil
}

type failingTransport struct{}

func (failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("connection refused")
}

func TestRequestClosesResponseBody(t *testing.T) {
	tracked := &trackingBody{}
	previous := httpClient
	httpClient = &http.Client{Transport: &bodyTransport{body: tracked}}
	defer func() { httpClient = previous }()

	c := &eurekaHttpClient{EurekaHttpConfig{BaseUrl: "localhost:8761", Retries: 1}}
	if _, _, err := c.request("/apps"); err != nil {
		t.Fatalf("request returned an unexpected error: %v", err)
	}
	if !tracked.closed {
		t.Fatal("the response body was never closed")
	}
}

func TestRequestHonoursRetryDelayInMilliseconds(t *testing.T) {
	previous := httpClient
	httpClient = &http.Client{Transport: failingTransport{}}
	defer func() { httpClient = previous }()

	c := &eurekaHttpClient{EurekaHttpConfig{
		BaseUrl:        "localhost:8761",
		Retries:        2,
		RetryDelayTime: 200,
	}}
	start := time.Now()
	if _, _, err := c.request("/apps"); err == nil {
		t.Fatal("expected the request to fail")
	}
	elapsed := time.Since(start)
	if elapsed < 150*time.Millisecond {
		t.Fatalf("RetryDelayTime is documented in milliseconds but only %v elapsed for a 200ms delay", elapsed)
	}
}
