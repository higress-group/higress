/*
Copyright 2022 The Kubernetes Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"path"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/config"
	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/roundtripper"
)

type Assertion struct {
	Meta     AssertionMeta
	Request  AssertionRequest
	Response AssertionResponse
}

type AssertionMeta struct {
	// TestCaseName is the User Given TestCase name
	TestCaseName string
	// TargetBackend defines the target backend service
	TargetBackend string
	// TargetNamespace defines the target backend namespace
	TargetNamespace string
	// CompareTarget defines who's header&body to compare in test, either CompareTargetResponse or CompareTargetRequest
	CompareTarget string
}

type AssertionRequest struct {
	// ActualRequest defines the request to make.
	ActualRequest Request

	// ExpectedRequest defines the request that
	// is expected to arrive at the backend. If
	// not specified, the backend request will be
	// expected to match Request.
	ExpectedRequest *ExpectedRequest
	RedirectRequest *roundtripper.RedirectRequest
}

type AssertionResponse struct {
	// ExpectedResponse defines what response the test case
	// should receive.
	ExpectedResponse Response
	// AdditionalResponseHeaders is a set of headers
	// the echoserver should set in its response.
	AdditionalResponseHeaders map[string]string
	// set not need to judge response has request info
	ExpectedResponseNoRequest bool
}

const (
	ContentTypeApplicationJson string = "application/json"
	ContentTypeFormUrlencoded         = "application/x-www-form-urlencoded"
	ContentTypeMultipartForm          = "multipart/form-data"
	ContentTypeTextPlain              = "text/plain"
	ContentTypeTextEventStream        = "text/event-stream"
)

const (
	CompareTargetRequest  = "Request"
	CompareTargetResponse = "Response"
)

// Request can be used as both the request to make and a means to verify
// that echoserver received the expected request. Note that multiple header
// values can be provided, as a comma-separated value.
type Request struct {
	Host             string
	Protocol         string
	Method           string
	Path             string
	Headers          map[string]string
	RawHeaders       http.Header
	Body             []byte
	ContentType      string
	UnfollowRedirect bool
	TLSConfig        *TLSConfig
}

// TLSConfig defines the TLS configuration for the client.
// When this field is set, the HTTPS protocol is used.
type TLSConfig struct {
	// MinVersion specifies the minimum TLS version,
	// e.g. tls.VersionTLS12.
	MinVersion uint16
	// MinVersion specifies the maximum TLS version,
	// e.g. tls.VersionTLS13.
	MaxVersion uint16
	// SNI is short for Server Name Indication.
	// If this field is not specified, the value will be equal to `Host`.
	SNI string
	// CipherSuites can specify multiple client cipher suites,
	// e.g. tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA.
	CipherSuites []uint16
	// Certificates defines the certificate chain
	Certificates Certificates
}

// Certificates contains CA and client certificate chain
type Certificates struct {
	CACerts        [][]byte
	ClientKeyPairs []ClientKeyPair
}

// ClientKeyPair is a pair of client certificate and private key.
type ClientKeyPair struct {
	ClientCert []byte
	ClientKey  []byte
}

// ExpectedRequest defines expected properties of a request that reaches a backend.
type ExpectedRequest struct {
	Request

	// AbsentHeaders are names of headers that are expected
	// *not* to be present on the request.
	AbsentHeaders []string
}

// Response defines expected properties of a response from a backend.
type Response struct {
	StatusCode           int
	Headers              map[string]string
	Body                 []byte
	JsonBodyIgnoreFields []string
	ContentType          string
	AbsentHeaders        []string
}

// requiredConsecutiveSuccesses is the number of requests that must succeed in a row
// for MakeRequestAndExpectEventuallyConsistentResponse to consider the response "consistent"
// before making additional assertions on the response body. If this number is not reached within
// maxTimeToConsistency, the test will fail.
const requiredConsecutiveSuccesses = 3

// MakeRequestAndExpectEventuallyConsistentResponse makes a request with the given parameters,
// understanding that the request may fail for some amount of time.
//
// Once the request succeeds consistently with the response having the expected status code, make
// additional assertions on the response body using the provided ExpectedResponse.
func MakeRequestAndExpectEventuallyConsistentResponse(t *testing.T, r roundtripper.RoundTripper, timeoutConfig config.TimeoutConfig, gwAddr string, expected Assertion) {
	t.Helper()

	var (
		scheme    = "http"
		tlsConfig *roundtripper.TLSConfig
	)
	if expected.Request.ActualRequest.TLSConfig != nil {
		scheme = "https"
		clientKeyPairs := make([]roundtripper.ClientKeyPair, 0, len(expected.Request.ActualRequest.TLSConfig.Certificates.ClientKeyPairs))
		for _, keyPair := range expected.Request.ActualRequest.TLSConfig.Certificates.ClientKeyPairs {
			clientKeyPairs = append(clientKeyPairs, roundtripper.ClientKeyPair{
				ClientCert: keyPair.ClientCert,
				ClientKey:  keyPair.ClientKey,
			})
		}
		tlsConfig = &roundtripper.TLSConfig{
			MinVersion:   expected.Request.ActualRequest.TLSConfig.MinVersion,
			MaxVersion:   expected.Request.ActualRequest.TLSConfig.MaxVersion,
			SNI:          expected.Request.ActualRequest.TLSConfig.SNI,
			CipherSuites: expected.Request.ActualRequest.TLSConfig.CipherSuites,
			Certificates: roundtripper.Certificates{
				CACert:         expected.Request.ActualRequest.TLSConfig.Certificates.CACerts,
				ClientKeyPairs: clientKeyPairs,
			},
		}
		if tlsConfig.SNI == "" {
			tlsConfig.SNI = expected.Request.ActualRequest.Host
		}
	}
	if expected.Meta.CompareTarget == "" {
		expected.Meta.CompareTarget = CompareTargetRequest
	}
	if expected.Request.ActualRequest.Method == "" {
		expected.Request.ActualRequest.Method = "GET"
	}

	if expected.Request.ActualRequest.Body != nil && len(expected.Request.ActualRequest.Body) > 0 {
		if len(expected.Request.ActualRequest.ContentType) == 0 {
			t.Error(`please set Content-Type in ActualRequest manually if you want to send a request with body.
			For example, \"ContentType: http.ContentTypeApplicationJson\"`)
		}
	}

	expected.Request.ActualRequest.Method = strings.ToUpper(expected.Request.ActualRequest.Method)

	if expected.Response.ExpectedResponse.StatusCode == 0 {
		expected.Response.ExpectedResponse.StatusCode = http.StatusOK
	}

	t.Logf("Making %s request to %s://%s%s", expected.Request.ActualRequest.Method, scheme, gwAddr, expected.Request.ActualRequest.Path)

	path, query, _ := strings.Cut(expected.Request.ActualRequest.Path, "?")

	protocol := "HTTP/1.1"
	if expected.Request.ActualRequest.Protocol != "" {
		protocol = expected.Request.ActualRequest.Protocol
	}

	req := roundtripper.Request{
		Method:           expected.Request.ActualRequest.Method,
		Host:             expected.Request.ActualRequest.Host,
		URL:              url.URL{Scheme: scheme, Host: gwAddr, Path: path, RawQuery: query},
		Protocol:         protocol,
		Headers:          map[string][]string{},
		Body:             expected.Request.ActualRequest.Body,
		ContentType:      expected.Request.ActualRequest.ContentType,
		UnfollowRedirect: expected.Request.ActualRequest.UnfollowRedirect,
		TLSConfig:        tlsConfig,
	}

	if expected.Request.ActualRequest.Headers != nil {
		for name, value := range expected.Request.ActualRequest.Headers {
			vals := strings.Split(value, ",")
			for _, val := range vals {
				req.Headers[name] = append(req.Headers[name], strings.TrimSpace(val))
			}
		}
	}

	if expected.Request.ActualRequest.RawHeaders != nil {
		for name, values := range expected.Request.ActualRequest.RawHeaders {
			for _, value := range values {
				req.Headers[name] = append(req.Headers[name], strings.TrimSpace(value))
			}
		}
	}

	backendSetHeaders := make([]string, 0, len(expected.Response.AdditionalResponseHeaders))
	for name, val := range expected.Response.AdditionalResponseHeaders {
		backendSetHeaders = append(backendSetHeaders, name+":"+val)
	}
	req.Headers["X-Echo-Set-Header"] = []string{strings.Join(backendSetHeaders, ",")}

	WaitForConsistentResponse(t, r, req, expected, requiredConsecutiveSuccesses, timeoutConfig.MaxTimeToConsistency)
}

// awaitConvergence runs the given function until it returns 'true' `threshold` times in a row.
// Each failed attempt has a 1s delay; successful attempts have no delay.
func awaitConvergence(t *testing.T, threshold int, maxTimeToConsistency time.Duration, fn func(elapsed time.Duration) bool) {
	successes := 0
	attempts := 0
	start := time.Now()
	to := time.After(maxTimeToConsistency)
	delay := time.Second
	for {
		select {
		case <-to:
			t.Fatalf("timeout while waiting after %d attempts", attempts)
		default:
		}

		completed := fn(time.Now().Sub(start))
		attempts++
		if completed {
			successes++
			if successes >= threshold {
				return
			}
			// Skip delay if we have a success
			continue
		}

		successes = 0
		select {
		// Capture the overall timeout
		case <-to:
			t.Fatalf("timeout while waiting after %d attempts, %d/%d sucessess", attempts, successes, threshold)
			// And the per-try delay
		case <-time.After(delay):
		}
	}
}

// WaitForConsistentResponse repeats the provided request until it completes with a response having
// the expected response consistently. The provided threshold determines how many times in
// a row this must occur to be considered "consistent".
func WaitForConsistentResponse(t *testing.T, r roundtripper.RoundTripper, req roundtripper.Request, expected Assertion, threshold int, maxTimeToConsistency time.Duration) {
	awaitConvergence(t, threshold, maxTimeToConsistency, func(elapsed time.Duration) bool {
		cReq, cRes, err := r.CaptureRoundTrip(req)
		if err != nil {
			t.Logf("Request failed, not ready yet: %v (after %v)", err.Error(), elapsed)
			return false
		}
		// CompareTarget为Request（默认）时，ExpectedRequest中设置的所有断言均支持；除ExpectedResponse.Body外，ExpectedResponse中设置的所有断言均支持。目前支持echo-server作为backend
		// CompareTarget为Response时，不支持设定ExpectedRequest断言，ExpectedResponse中设置的所有断言均支持。支持任意backend，如echo-body
		if expected.Meta.CompareTarget == CompareTargetRequest {
			if expected.Response.ExpectedResponse.Body != nil {
				t.Logf(`detected CompareTarget is Request, but ExpectedResponse.Body is set.
				You can only choose one to compare between Response and Request.`)
				return false
			}

			if cRes.StatusCode == http.StatusOK && !expected.Response.ExpectedResponseNoRequest && cReq.Host == "" && cReq.Path == "" && cReq.Headers == nil && cReq.Body == nil {
				t.Logf(`decoding client's response failed. Maybe you have chosen a wrong backend.
				Choose echo-server if you want to check expected request header&body instead of response header&body.`)
				return false
			}
			if err = CompareRequest(&req, cReq, cRes, expected); err != nil {
				t.Logf("request expectation failed for actual request: %v  not ready yet: %v (after %v)", req, err, elapsed)
				return false
			}
		} else if expected.Meta.CompareTarget == CompareTargetResponse {
			if expected.Request.ExpectedRequest != nil {
				t.Logf(`detected CompareTarget is Response, but ExpectedRequest is set.
				You can only choose one to compare between Response and Request.`)
				return false
			}
			if err = CompareResponse(cRes, expected); err != nil {
				t.Logf("Response expectation failed for actual request: %v  not ready yet: %v (after %v)", req, err, elapsed)
				return false
			}
		} else {
			t.Logf("invalid CompareTarget: %v please set it CompareTargetRequest or CompareTargetResponse", expected.Meta.CompareTarget)
			return false
		}

		return true
	})
	t.Logf("Request passed")
}

func CompareRequest(req *roundtripper.Request, cReq *roundtripper.CapturedRequest, cRes *roundtripper.CapturedResponse, expected Assertion) error {
	if expected.Response.ExpectedResponse.StatusCode != cRes.StatusCode {
		return fmt.Errorf("expected status code to be %d, got %d", expected.Response.ExpectedResponse.StatusCode, cRes.StatusCode)
	}
	if cRes.StatusCode == http.StatusOK && !expected.Response.ExpectedResponseNoRequest {
		// The request expected to arrive at the backend is
		// the same as the request made, unless otherwise
		// specified.
		if expected.Request.ExpectedRequest == nil {
			expected.Request.ExpectedRequest = &ExpectedRequest{Request: expected.Request.ActualRequest}
		}

		if expected.Request.ExpectedRequest.Method == "" {
			expected.Request.ExpectedRequest.Method = "GET"
		}

		if expected.Request.ExpectedRequest.Host != "" && expected.Request.ExpectedRequest.Host != cReq.Host {
			return fmt.Errorf("expected host to be %s, got %s", expected.Request.ExpectedRequest.Host, cReq.Host)
		}

		if expected.Request.ExpectedRequest.Path != cReq.Path {
			return fmt.Errorf("expected path to be %s, got %s", expected.Request.ExpectedRequest.Path, cReq.Path)
		}
		if expected.Request.ExpectedRequest.Method != cReq.Method {
			return fmt.Errorf("expected method to be %s, got %s", expected.Request.ExpectedRequest.Method, cReq.Method)
		}
		if expected.Meta.TargetNamespace != cReq.Namespace {
			return fmt.Errorf("expected namespace to be %s, got %s", expected.Meta.TargetNamespace, cReq.Namespace)
		}
		if expected.Request.ExpectedRequest.Headers != nil {
			if cReq.Headers == nil {
				return fmt.Errorf("no headers captured, expected %v", len(expected.Request.ExpectedRequest.Headers))
			}
			for name, val := range cReq.Headers {
				cReq.Headers[strings.ToLower(name)] = val
			}
			for name, expectedVal := range expected.Request.ExpectedRequest.Headers {
				actualVal, ok := cReq.Headers[strings.ToLower(name)]
				if !ok {
					return fmt.Errorf("expected %s header to be set, actual headers: %v", name, cReq.Headers)
				} else if strings.Join(actualVal, ",") != expectedVal {
					return fmt.Errorf("expected %s header to be set to %s, got %s", name, expectedVal, strings.Join(actualVal, ","))
				}
			}
		}
		if expected.Request.ExpectedRequest.Body != nil && len(expected.Request.ExpectedRequest.Body) > 0 {
			// 对ExpectedRequest.Body做断言时，须手动指定ExpectedRequest.ContentType
			if len(expected.Request.ExpectedRequest.ContentType) == 0 {
				return fmt.Errorf("ExpectedRequest.ContentType should not be empty since ExpectedRequest.Body is set")
			}

			if cReq.Headers["Content-Type"] == nil || len(cReq.Headers["Content-Type"]) == 0 {
				cReq.Headers["Content-Type"] = []string{expected.Request.ExpectedRequest.ContentType}
			}

			eTyp, eParams, err := mime.ParseMediaType(expected.Request.ExpectedRequest.ContentType)
			if err != nil {
				return fmt.Errorf("ExpectedRequest Content-Type: %s failed to parse: %s", expected.Request.ExpectedRequest.ContentType, err.Error())
			}

			cTyp := cReq.Headers["Content-Type"][0]

			if eTyp != cTyp {
				if !(eTyp == ContentTypeMultipartForm && strings.Contains(cTyp, eTyp)) {
					return fmt.Errorf("expected %s Content-Type to be set, got %s", eTyp, cTyp)
				}
			}
			var ok bool
			switch eTyp {
			case ContentTypeTextPlain:
				if string(expected.Request.ExpectedRequest.Body) != cReq.Body.(string) {
					return fmt.Errorf("expected %s body to be %s, got %s", eTyp, string(expected.Request.ExpectedRequest.Body), cReq.Body.(string))
				}
			case ContentTypeApplicationJson:
				var eReqBody map[string]interface{}
				var cReqBody map[string]interface{}

				err := json.Unmarshal(expected.Request.ExpectedRequest.Body, &eReqBody)
				if err != nil {
					return fmt.Errorf("failed to unmarshall ExpectedRequest body %s, %s", string(expected.Request.ExpectedRequest.Body), err.Error())
				}

				if cReqBody, ok = cReq.Body.(map[string]interface{}); !ok {
					return fmt.Errorf("failed to parse CapturedRequest body")
				}

				if !reflect.DeepEqual(eReqBody, cReqBody) {
					eRBJson, _ := json.Marshal(eReqBody)
					cRBJson, _ := json.Marshal(cReqBody)
					return fmt.Errorf("expected %s body to be %s, got result: %s", eTyp, string(eRBJson), string(cRBJson))
				}
			case ContentTypeFormUrlencoded:
				var eReqBody map[string][]string
				var cReqBody map[string][]string
				eReqBody, err = ParseFormUrlencodedBody(expected.Request.ExpectedRequest.Body)
				if err != nil {
					return fmt.Errorf("failed to parse ExpectedRequest body %s, %s", string(expected.Request.ExpectedRequest.Body), err.Error())
				}

				if cReqBody, ok = cReq.Body.(map[string][]string); !ok {
					return fmt.Errorf("failed to parse CapturedRequest body")
				}

				if !reflect.DeepEqual(eReqBody, cReqBody) {
					eRBJson, _ := json.Marshal(eReqBody)
					cRBJson, _ := json.Marshal(cReqBody)
					return fmt.Errorf("expected %s body to be %s, got result: %s", eTyp, string(eRBJson), string(cRBJson))
				}
			case ContentTypeMultipartForm:
				var eReqBody map[string][]string
				var cReqBody map[string][]string

				eReqBody, err = ParseMultipartFormBody(expected.Request.ExpectedRequest.Body, eParams["boundary"])
				if err != nil {
					return fmt.Errorf("failed to parse ExpectedRequest body %s, %s", string(expected.Request.ExpectedRequest.Body), err.Error())
				}
				if cReqBody, ok = cReq.Body.(map[string][]string); !ok {
					return fmt.Errorf("failed to parse CapturedRequest body")
				}

				if !reflect.DeepEqual(eReqBody, cReqBody) {
					eRBJson, _ := json.Marshal(eReqBody)
					cRBJson, _ := json.Marshal(cReqBody)
					return fmt.Errorf("expected %s body to be %s, got result: %s", eTyp, string(eRBJson), string(cRBJson))
				}
			default:
				return fmt.Errorf("Content-Type: %s invalid or not support.", eTyp)
			}
		}
		if expected.Response.ExpectedResponse.Headers != nil {
			if cRes.Headers == nil {
				return fmt.Errorf("no headers captured, expected %v", len(expected.Request.ExpectedRequest.Headers))
			}
			for name, val := range cRes.Headers {
				cRes.Headers[strings.ToLower(name)] = val
			}

			for name, expectedVal := range expected.Response.ExpectedResponse.Headers {
				actualVal, ok := cRes.Headers[strings.ToLower(name)]
				if !ok {
					return fmt.Errorf("expected %s header to be set, actual headers: %v", name, cRes.Headers)
				} else if strings.Join(actualVal, ",") != expectedVal {
					return fmt.Errorf("expected %s header to be set to %s, got %s", name, expectedVal, strings.Join(actualVal, ","))
				}
			}
		}

		if len(expected.Response.ExpectedResponse.AbsentHeaders) > 0 {
			for name, val := range cRes.Headers {
				cRes.Headers[strings.ToLower(name)] = val
			}

			for _, name := range expected.Response.ExpectedResponse.AbsentHeaders {
				val, ok := cRes.Headers[strings.ToLower(name)]
				if ok {
					return fmt.Errorf("expected %s header to not be set, got %s", name, val)
				}
			}
		}

		// Verify that headers expected *not* to be present on the
		// request are actually not present.
		if len(expected.Request.ExpectedRequest.AbsentHeaders) > 0 {
			for name, val := range cReq.Headers {
				cReq.Headers[strings.ToLower(name)] = val
			}

			for _, name := range expected.Request.ExpectedRequest.AbsentHeaders {
				val, ok := cReq.Headers[strings.ToLower(name)]
				if ok {
					return fmt.Errorf("expected %s header to not be set, got %s", name, val)
				}
			}
		}

		if !strings.HasPrefix(cReq.Pod, expected.Meta.TargetBackend) {
			return fmt.Errorf("expected pod name to start with %s, got %s", expected.Meta.TargetBackend, cReq.Pod)
		}

	} else if roundtripper.IsRedirect(cRes.StatusCode) {
		if expected.Request.RedirectRequest == nil {
			return nil
		}

		setRedirectRequestDefaults(req, cRes, &expected)

		if expected.Request.RedirectRequest.Host != cRes.RedirectRequest.Host {
			return fmt.Errorf("expected redirected hostname to be %s, got %s", expected.Request.RedirectRequest.Host, cRes.RedirectRequest.Host)
		}

		if expected.Request.RedirectRequest.Port != cRes.RedirectRequest.Port {
			return fmt.Errorf("expected redirected port to be %s, got %s", expected.Request.RedirectRequest.Port, cRes.RedirectRequest.Port)
		}

		if expected.Request.RedirectRequest.Scheme != cRes.RedirectRequest.Scheme {
			return fmt.Errorf("expected redirected scheme to be %s, got %s", expected.Request.RedirectRequest.Scheme, cRes.RedirectRequest.Scheme)
		}

		if expected.Request.RedirectRequest.Path != cRes.RedirectRequest.Path {
			return fmt.Errorf("expected redirected path to be %s, got %s", expected.Request.RedirectRequest.Path, cRes.RedirectRequest.Path)
		}
	}
	return nil
}

// QueryPrometheus queries Prometheus API and returns the raw response body.
// It handles URL construction, timeout, and basic error checking.
// The response body is limited to 10MB to prevent excessive memory usage.
func QueryPrometheus(prometheusAddr string, query string) ([]byte, error) {
	// Normalize base URL
	baseURL := prometheusAddr
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		baseURL = "http://" + baseURL
	}

	// Parse base URL to ensure it's valid
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid prometheus address %q: %w", prometheusAddr, err)
	}

	// Construct query URL properly
	parsedURL.Path = path.Join(parsedURL.Path, "/api/v1/query")
	queryParams := parsedURL.Query()
	queryParams.Set("query", query)
	parsedURL.RawQuery = queryParams.Encode()

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	// Execute query
	resp, err := client.Get(parsedURL.String())
	if err != nil {
		return nil, fmt.Errorf("prometheus query failed: %w", err)
	}
	defer resp.Body.Close()

	// Check HTTP status code
	if resp.StatusCode != 200 {
		// Limit error response body to 1KB
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("prometheus returned status %d: %s", resp.StatusCode, string(body))
	}

	// Read response body with size limit (10MB)
	// This prevents excessive memory usage when Prometheus returns large datasets
	const maxResponseSize = 10 * 1024 * 1024 // 10MB
	limitedReader := io.LimitReader(resp.Body, maxResponseSize)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read prometheus response: %w", err)
	}

	// Check if response was truncated
	if len(body) == maxResponseSize {
		// Try to read one more byte to confirm truncation
		extraByte := make([]byte, 1)
		if n, _ := resp.Body.Read(extraByte); n > 0 {
			return nil, fmt.Errorf("prometheus response exceeds maximum size of %d bytes (likely too many metrics or large time range)", maxResponseSize)
		}
	}

	return body, nil
}

func CompareResponse(cRes *roundtripper.CapturedResponse, expected Assertion) error {
	if expected.Response.ExpectedResponse.StatusCode != cRes.StatusCode {
		return fmt.Errorf("expected status code to be %d, got %d", expected.Response.ExpectedResponse.StatusCode, cRes.StatusCode)
	}
	if cRes.StatusCode == 200 {
		if len(expected.Meta.TargetNamespace) > 0 {
			if cRes.Headers["Namespace"] == nil || len(cRes.Headers["Namespace"]) == 0 {
				return fmt.Errorf("expected namespace to be %s, field not found in CaptureResponse", expected.Meta.TargetNamespace)
			}
			if expected.Meta.TargetNamespace != cRes.Headers["Namespace"][0] {
				return fmt.Errorf("expected namespace to be %s, got %s", expected.Meta.TargetNamespace, cRes.Headers["Namespace"][0])
			}
		}

		if len(expected.Meta.TargetBackend) > 0 {
			if cRes.Headers["Pod"] == nil || len(cRes.Headers["Pod"]) == 0 {
				return fmt.Errorf("expected pod to be %s, field not found in CaptureResponse", expected.Meta.TargetBackend)
			}
			if !strings.HasPrefix(cRes.Headers["Pod"][0], expected.Meta.TargetBackend) {
				return fmt.Errorf("expected pod to be %s, got %s", expected.Meta.TargetBackend, cRes.Headers["Pod"][0])
			}
		}

		if expected.Response.ExpectedResponse.Headers != nil {
			if cRes.Headers == nil {
				return fmt.Errorf("no headers captured, expected %v", len(expected.Response.ExpectedResponse.Headers))
			}
			for name, val := range cRes.Headers {
				cRes.Headers[strings.ToLower(name)] = val
			}
			for name, expectedVal := range expected.Response.ExpectedResponse.Headers {
				actualVal, ok := cRes.Headers[strings.ToLower(name)]
				if !ok {
					return fmt.Errorf("expected %s header to be set, actual headers: %v", name, cRes.Headers)
				} else if strings.Join(actualVal, ",") != expectedVal {
					return fmt.Errorf("expected %s header to be set to %s, got %s", name, expectedVal, strings.Join(actualVal, ","))
				}
			}
		}
		if expected.Response.ExpectedResponse.Body != nil && len(expected.Response.ExpectedResponse.Body) > 0 {
			// 对ExpectedResponse.Body做断言时，必须指定ExpectedResponse.ContentType
			if len(expected.Response.ExpectedResponse.ContentType) == 0 {
				return fmt.Errorf("ExpectedResponse.ContentType should not be empty since ExpectedResponse.Body is set")
			}

			if cRes.Headers["Content-Type"] == nil || len(cRes.Headers["Content-Type"]) == 0 {
				cRes.Headers["Content-Type"] = []string{expected.Response.ExpectedResponse.ContentType}
			}

			eTyp, eParams, err := mime.ParseMediaType(expected.Response.ExpectedResponse.ContentType)
			if err != nil {
				return fmt.Errorf("ExpectedResponse Content-Type: %s failed to parse: %s", expected.Response.ExpectedResponse.ContentType, err.Error())
			}
			cTyp, cParams, err := mime.ParseMediaType(cRes.Headers["Content-Type"][0])
			if err != nil {
				return fmt.Errorf("CapturedResponse Content-Type: %s failed to parse: %s", cRes.Headers["Content-Type"][0], err.Error())
			}

			if eTyp != cTyp {
				return fmt.Errorf("expected %s Content-Type to be set, got %s", expected.Response.ExpectedResponse.ContentType, cRes.Headers["Content-Type"][0])
			}

			switch cTyp {
			case ContentTypeTextPlain:
			case ContentTypeTextEventStream:
				if !bytes.Equal(expected.Response.ExpectedResponse.Body, cRes.Body) {
					return fmt.Errorf("expected %s body to be %s, got %s", cTyp, string(expected.Response.ExpectedResponse.Body), string(cRes.Body))
				}
			case ContentTypeApplicationJson:
				eResBody := make(map[string]interface{})
				cResBody := make(map[string]interface{})
				err := json.Unmarshal(expected.Response.ExpectedResponse.Body, &eResBody)
				if err != nil {
					return fmt.Errorf("failed to unmarshall ExpectedResponse body %s, %s", string(expected.Response.ExpectedResponse.Body), err.Error())
				}
				err = json.Unmarshal(cRes.Body, &cResBody)
				if err != nil {
					return fmt.Errorf("failed to unmarshall CapturedResponse body %s, %s", string(cRes.Body), err.Error())
				}

				if err := CompareJSONWithIgnoreFields(eResBody, cResBody, expected.Response.ExpectedResponse.JsonBodyIgnoreFields); err != nil {
					return fmt.Errorf("expected %s body to be %s, got %s", cTyp, string(expected.Response.ExpectedResponse.Body), string(cRes.Body))
				}
			case ContentTypeFormUrlencoded:
				eResBody, err := ParseFormUrlencodedBody(expected.Response.ExpectedResponse.Body)
				if err != nil {
					return fmt.Errorf("failed to parse ExpectedResponse body %s, %s", string(expected.Response.ExpectedResponse.Body), err.Error())
				}
				cResBody, err := ParseFormUrlencodedBody(cRes.Body)
				if err != nil {
					return fmt.Errorf("failed to parse CapturedResponse body %s, %s", string(cRes.Body), err.Error())
				}

				if !reflect.DeepEqual(eResBody, cResBody) {
					return fmt.Errorf("expected %s body to be %s, got %s", cTyp, string(expected.Response.ExpectedResponse.Body), string(cRes.Body))
				}
			case ContentTypeMultipartForm:
				eResBody, err := ParseMultipartFormBody(expected.Response.ExpectedResponse.Body, eParams["boundary"])
				if err != nil {
					return fmt.Errorf("failed to parse ExpectedResponse body %s, %s", string(expected.Response.ExpectedResponse.Body), err.Error())
				}
				cResBody, err := ParseMultipartFormBody(cRes.Body, cParams["boundary"])
				if err != nil {
					return fmt.Errorf("failed to parse CapturedResponse body %s, %s", string(cRes.Body), err.Error())
				}
				if !reflect.DeepEqual(eResBody, cResBody) {
					return fmt.Errorf("expected %s body to be %s, got %s", cTyp, string(expected.Response.ExpectedResponse.Body), string(cRes.Body))
				}
			default:
				return fmt.Errorf("Content-Type: %s invalid or not support.", cTyp)
			}
		}
		if len(expected.Response.ExpectedResponse.AbsentHeaders) > 0 {
			for name, val := range cRes.Headers {
				cRes.Headers[strings.ToLower(name)] = val
			}

			for _, name := range expected.Response.ExpectedResponse.AbsentHeaders {
				val, ok := cRes.Headers[strings.ToLower(name)]
				if ok {
					return fmt.Errorf("expected %s header to not be set, got %s", name, val)
				}
			}
		}
	}
	return nil
}

// CompareJSONWithIgnoreFields compares two JSON objects, ignoring specified fields
func CompareJSONWithIgnoreFields(eResBody, cResBody map[string]interface{}, ignoreFields []string) error {
	for key, eVal := range eResBody {
		if contains(ignoreFields, key) {
			continue
		}

		cVal, exists := cResBody[key]
		if !exists {
			return fmt.Errorf("field %s exists in expected response but not in captured response", key)
		}

		if !reflect.DeepEqual(eVal, cVal) {
			return fmt.Errorf("field %s mismatch: expected %v, got %v", key, eVal, cVal)
		}
	}

	// Check if captured response has extra fields (excluding ignored fields)
	for key := range cResBody {
		if contains(ignoreFields, key) {
			continue
		}

		if _, exists := eResBody[key]; !exists {
			return fmt.Errorf("field %s exists in captured response but not in expected response", key)
		}
	}

	return nil
}

func contains(slice []string, str string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}

func ParseFormUrlencodedBody(body []byte) (map[string][]string, error) {
	ret := make(map[string][]string)
	kvs, err := url.ParseQuery(string(body))
	if err != nil {
		return nil, err
	}
	for k, vs := range kvs {
		ret[k] = vs
	}

	return ret, nil
}

func ParseMultipartFormBody(body []byte, boundary string) (map[string][]string, error) {
	ret := make(map[string][]string)
	mr := multipart.NewReader(bytes.NewReader(body), boundary)
	for {
		p, err := mr.NextPart()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		formName := p.FormName()
		fileName := p.FileName()
		if formName == "" || fileName != "" {
			continue
		}
		formValue, err := io.ReadAll(p)
		if err != nil {
			return nil, err
		}
		ret[formName] = append(ret[formName], string(formValue))
	}
	return ret, nil
}

// Get User-defined test case name or generate from expected response to a given request.
func (er *Assertion) GetTestCaseName(i int) string {
	// If TestCase name is provided then use that or else generate one.
	if er.Meta.TestCaseName != "" {
		return er.Meta.TestCaseName
	}

	headerStr := ""
	reqStr := ""

	if er.Request.ActualRequest.Headers != nil || er.Request.ActualRequest.RawHeaders != nil {
		headerStr = " with headers"
	}

	reqStr = fmt.Sprintf("%d request to '%s%s'%s", i, er.Request.ActualRequest.Host, er.Request.ActualRequest.Path, headerStr)

	if er.Meta.TargetBackend != "" {
		return fmt.Sprintf("%s should go to %s", reqStr, er.Meta.TargetBackend)
	}
	return fmt.Sprintf("%s should receive a %d", reqStr, er.Response.ExpectedResponse.StatusCode)
}

func setRedirectRequestDefaults(req *roundtripper.Request, cRes *roundtripper.CapturedResponse, expected *Assertion) {
	// If the expected host is nil it means we do not test host redirect.
	// In that case we are setting it to the one we got from the response because we do not know the ip/host of the gateway.
	if expected.Request.RedirectRequest.Host == "" {
		expected.Request.RedirectRequest.Host = cRes.RedirectRequest.Host
	}

	if expected.Request.RedirectRequest.Port == "" {
		expected.Request.RedirectRequest.Port = req.URL.Port()
	}

	if expected.Request.RedirectRequest.Scheme == "" {
		expected.Request.RedirectRequest.Scheme = req.URL.Scheme
	}

	if expected.Request.RedirectRequest.Path == "" {
		expected.Request.RedirectRequest.Path = req.URL.Path
	}
}
