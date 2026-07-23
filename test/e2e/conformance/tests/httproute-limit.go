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

package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/roundtripper"
	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/suite"
)

func init() {
	Register(HttpRouteLimiter)
}

var HttpRouteLimiter = suite.ConformanceTest{
	ShortName:   "HttpRouteLimiter",
	Description: "The Ingress in the higress-conformance-infra namespace uses rps annotation",
	Features:    []suite.SupportedFeature{suite.HTTPConformanceFeature},
	Manifests:   []string{"tests/httproute-limit.yaml"},
	Test: func(t *testing.T, suite *suite.ConformanceTestSuite) {
		t.Run("HTTPRoute limiter", func(t *testing.T) {
			// wait ingress ready
			time.Sleep(1 * time.Second)
			client := &http.Client{}
			TestRps10(t, suite.GatewayAddress, client)
			TestRps50(t, suite.GatewayAddress, client)
			TestRps10Burst3(t, suite.GatewayAddress, client)
			TestRpm10(t, suite.GatewayAddress, client)
			TestRpm10Burst3(t, suite.GatewayAddress, client)
		})
	},
}

// TestRps10 test case 1: rps10
func TestRps10(t *testing.T, gwAddr string, client *http.Client) {
	req := &roundtripper.Request{
		Method: "GET",
		Host:   "limiter.higress.io",
		URL: url.URL{
			Scheme: "http",
			Host:   gwAddr,
			Path:   "/rps10",
		},
	}

	result, err := ParallelRunner(10, 3000, req, client)
	if err != nil {
		t.Fatal(err)
	}
	AssertRps(t, result, 10, 0.5)
}

// TestRps50 test case 2: rps50
func TestRps50(t *testing.T, gwAddr string, client *http.Client) {
	req := &roundtripper.Request{
		Method: "GET",
		Host:   "limiter.higress.io",
		URL: url.URL{
			Scheme: "http",
			Host:   gwAddr,
			Path:   "/rps50",
		},
	}

	result, err := ParallelRunner(10, 5000, req, client)
	if err != nil {
		t.Fatal(err)
	}
	AssertRps(t, result, 50, 0.5)
}

// TestRps10Burst3 test case 3: rps10 burst3
func TestRps10Burst3(t *testing.T, gwAddr string, client *http.Client) {
	req := &roundtripper.Request{
		Method: "GET",
		Host:   "limiter.higress.io",
		URL: url.URL{
			Scheme: "http",
			Host:   gwAddr,
			Path:   "/rps10/burst3",
		},
	}

	result, err := ParallelRunner(30, 50, req, client)
	if err != nil {
		t.Fatal(err)
	}
	AssertRps(t, result, 30, -1)
}

// TestRpm10 test case 4: rpm10
func TestRpm10(t *testing.T, gwAddr string, client *http.Client) {
	req := &roundtripper.Request{
		Method: "GET",
		Host:   "limiter.higress.io",
		URL: url.URL{
			Scheme: "http",
			Host:   gwAddr,
			Path:   "/rpm10",
		},
	}

	result, err := ParallelRunner(10, 100, req, client)
	if err != nil {
		t.Fatal(err)
	}
	AssertRps(t, result, 10, -1)
}

// TestRpm10Burst3 test case 5: rpm10 burst3
func TestRpm10Burst3(t *testing.T, gwAddr string, client *http.Client) {
	req := &roundtripper.Request{
		Method: "GET",
		Host:   "limiter.higress.io",
		URL: url.URL{
			Scheme: "http",
			Host:   gwAddr,
			Path:   "/rpm10/burst3",
		},
	}
	result, err := ParallelRunner(30, 100, req, client)
	if err != nil {
		t.Fatal(err)
	}
	AssertRps(t, result, 30, -1)
}

// DoRequest send Http request according to req and client, return status code and error
func DoRequest(req *roundtripper.Request, client *http.Client) (int, error) {
	u := &url.URL{
		Scheme:   req.URL.Scheme,
		Host:     req.URL.Host,
		Path:     req.URL.Path,
		RawQuery: req.URL.RawQuery,
	}
	r, err := http.NewRequest(req.Method, u.String(), nil)
	if err != nil {
		return 0, err
	}

	if r.Host != "" {
		r.Host = req.Host
	}

	if req.Headers != nil {
		for name, values := range req.Headers {
			for _, value := range values {
				r.Header.Add(name, value)
			}
		}
	}

	if r.Body != nil {
		body, err := json.Marshal(req.Body)
		if err != nil {
			return 0, err
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(r)
	if err != nil {
		return 1, err
	}
	defer client.CloseIdleConnections()
	defer resp.Body.Close()

	return resp.StatusCode, nil
}

// ParallelRunner send Http request in parallel and count rps
func ParallelRunner(threads int, times int, req *roundtripper.Request, client *http.Client) (*Result, error) {
	var wg sync.WaitGroup
	result := &Result{
		Requests: times,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	startTime := time.Now()
	for i := 0; i < threads; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < times/threads; j++ {
				if ctx.Err() != nil {
					return
				}
				b2 := time.Now()
				statusCode, err := DoRequest(req, client)
				if err != nil {
					log.Printf("run() with failed: %v", err)
					continue
				}
				elapsed := time.Since(b2).Nanoseconds() / 1e6
				detailRecord := &DetailRecord{
					StatusCode: statusCode,
					ElapseMs:   elapsed,
				}
				result.DetailMaps.Store(rand.Int(), detailRecord)
				if statusCode >= 200 && statusCode < 300 {
					atomic.AddInt32(&result.Success, 1)
				} else {
					time.Sleep(50 * time.Millisecond)
				}
			}
		}()
	}

	wg.Wait()
	result.TotalCostMs = time.Since(startTime).Nanoseconds() / 1e6
	result.SuccessRps = float64(result.Success) * 1000 / float64(result.TotalCostMs)
	result.ActualRps = float64(result.Requests) * 1000 / float64(result.TotalCostMs)
	return result, nil
}

// AssertRps check actual rps is in expected range if tolerance is not -1
// else check actual success requests is less than expected
func AssertRps(t *testing.T, result *Result, expectedRps float64, tolerance float64) {
	if tolerance != -1 {
		fmt.Printf("Total Cost(s): %.2f, Total Request: %d, Total Success: %d, Actual RPS: %.2f, Expected Rps: %.2f, Success Rps: %.2f\n",
			float64(result.TotalCostMs)/1000, result.Requests, result.Success, result.ActualRps, expectedRps, result.SuccessRps)
		lo := expectedRps * (1 - tolerance)
		hi := expectedRps * (1 + tolerance)
		message := fmt.Sprintf("RPS `%.2f` should between `%.2f` - `%.2f`", result.SuccessRps, lo, hi)
		if result.SuccessRps < lo || result.SuccessRps > hi {
			t.Error(message)
		}
	} else {
		fmt.Printf("Total Cost(s): %.2f, Total Request: %d, Total Success: %d, Expected: %.2f\n",
			float64(result.TotalCostMs)/1000, result.Requests, result.Success, expectedRps)
		message := fmt.Sprintf("Success Requests should less than : %d, actual: %d", int32(expectedRps), result.Success)
		if result.Success > int32(expectedRps) {
			t.Error(message)
		}
	}
}

type DetailRecord struct {
	StatusCode int
	ElapseMs   int64
}

type Result struct {
	Requests    int
	Success     int32
	TotalCostMs int64
	SuccessRps  float64
	ActualRps   float64
	DetailMaps  sync.Map
}
