// Copyright (c) 2025 Alibaba Group Holding Ltd.
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

package main

import (
	"bytes"
	"io"
	"net"
	"testing"
	"time"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

// Raw frames make DATA boundaries and omitted content headers deterministic.
func runHmacHTTP2Request(t *testing.T, host test.TestHost, headers [][2]string, chunks [][]byte) {
	t.Helper()
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	deadline := time.Now().Add(5 * time.Second)
	require.NoError(t, clientConn.SetDeadline(deadline))
	require.NoError(t, serverConn.SetDeadline(deadline))

	done := make(chan error, 1)
	go func() {
		defer serverConn.Close()
		preface := make([]byte, len(http2.ClientPreface))
		if _, err := io.ReadFull(serverConn, preface); err != nil {
			done <- err
			return
		}
		framer := http2.NewFramer(serverConn, serverConn)
		for {
			frame, err := framer.ReadFrame()
			if err != nil {
				done <- err
				return
			}
			switch f := frame.(type) {
			case *http2.HeadersFrame:
				decoder := hpack.NewDecoder(4096, nil)
				fields, err := decoder.DecodeFull(f.HeaderBlockFragment())
				if err != nil {
					done <- err
					return
				}
				requestHeaders := make([][2]string, 0, len(fields))
				for _, field := range fields {
					requestHeaders = append(requestHeaders, [2]string{field.Name, field.Value})
				}
				host.CallOnHttpRequestHeaders(requestHeaders)
			case *http2.DataFrame:
				host.CallOnHttpStreamingRequestBody(f.Data(), f.StreamEnded())
				if f.StreamEnded() {
					done <- nil
					return
				}
			}
		}
	}()

	_, err := io.WriteString(clientConn, http2.ClientPreface)
	require.NoError(t, err)
	framer := http2.NewFramer(clientConn, clientConn)
	require.NoError(t, framer.WriteSettings())
	var block bytes.Buffer
	encoder := hpack.NewEncoder(&block)
	for _, header := range headers {
		require.NoError(t, encoder.WriteField(hpack.HeaderField{Name: header[0], Value: header[1]}))
	}
	require.NoError(t, framer.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      1,
		BlockFragment: block.Bytes(),
		EndHeaders:    true,
	}))
	for i, chunk := range chunks {
		require.NoError(t, framer.WriteData(1, i == len(chunks)-1, chunk))
	}
	require.NoError(t, <-done)
}

func TestHmacHTTP2BodyDigest(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		payload := []byte("body-over-http2-data")
		tests := []struct {
			name            string
			chunks          [][]byte
			incorrectDigest bool
		}{
			{name: "single DATA without content headers", chunks: [][]byte{payload}},
			{name: "multiple DATA frames", chunks: [][]byte{payload[:3], payload[3:8], payload[8:]}},
			{name: "empty then non-empty DATA with incorrect digest", chunks: [][]byte{{}, payload}, incorrectDigest: true},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				host, status := test.NewTestHost(createConfig(
					[]map[string]interface{}{{"name": "consumer1", "access_key": "ak1", "secret_key": "sk1"}},
					map[string]interface{}{"global_auth": true, "validate_request_body": true},
				))
				defer host.Reset()
				require.Equal(t, types.OnPluginStartStatusOK, status)

				date := gmt()
				auth := authHeaderRequestTargetDate("ak1", "sk1", "hmac-sha256", "POST", "/p", date)
				digestBody := payload
				if tt.incorrectDigest {
					digestBody = []byte("different")
				}
				runHmacHTTP2Request(t, host, [][2]string{
					{":method", "POST"}, {":scheme", "http"}, {":path", "/p"}, {":authority", "e.com"},
					{"authorization", auth}, {"date", date}, {"digest", calculateBodyDigest(digestBody)},
				}, tt.chunks)

				response := host.GetLocalResponse()
				if tt.incorrectDigest {
					require.NotNil(t, response)
					require.Equal(t, uint32(401), response.StatusCode)
				} else {
					require.Nil(t, response)
				}
			})
		}
	})
}
