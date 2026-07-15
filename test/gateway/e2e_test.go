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
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"

	"sigs.k8s.io/gateway-api/conformance"
	"sigs.k8s.io/gateway-api/conformance/utils/roundtripper"
)

const dialLocalhostEnv = "HIGRESS_GATEWAY_API_TEST_DIAL_LOCALHOST"
const localHTTPPortEnv = "HIGRESS_GATEWAY_API_TEST_LOCAL_HTTP_PORT"
const localHTTPSPortEnv = "HIGRESS_GATEWAY_API_TEST_LOCAL_HTTPS_PORT"

type localPortForward struct {
	cmd  *exec.Cmd
	port string
}

type localGatewayDialer struct {
	mu       sync.Mutex
	forwards map[string]localPortForward
}

var forwardingAddress = regexp.MustCompile(`Forwarding from 127\.0\.0\.1:(\d+) ->`)

func TestGatewayAPIConformance(t *testing.T) {
	opts := conformance.DefaultOptions(t)
	if os.Getenv(dialLocalhostEnv) == "true" {
		dialer := &localGatewayDialer{forwards: map[string]localPortForward{}}
		t.Cleanup(dialer.close)
		opts.RoundTripper = &roundtripper.DefaultRoundTripper{
			Debug:             opts.Debug,
			TimeoutConfig:     opts.TimeoutConfig,
			CustomDialContext: dialer.dialContext,
		}
	}
	conformance.RunConformanceWithOptions(t, opts)
}

func (d *localGatewayDialer) dialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	parts := strings.Split(host, ".")
	if len(parts) >= 3 && parts[2] == "svc" {
		port, err = d.forward(parts[1], parts[0], port)
		if err != nil {
			return nil, err
		}
	} else {
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
	}
	var dialer net.Dialer
	return dialer.DialContext(ctx, network, net.JoinHostPort("127.0.0.1", port))
}

func (d *localGatewayDialer) forward(namespace, service, remotePort string) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	key := namespace + "/" + service + ":" + remotePort
	if forward, found := d.forwards[key]; found {
		return forward.port, nil
	}

	resource, targetPort, err := servicePortForwardTarget(namespace, service, remotePort)
	if err != nil {
		return "", err
	}
	cmd := exec.Command("kubectl", "-n", namespace, "port-forward", "--address=127.0.0.1", resource, ":"+targetPort)
	output, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return "", err
	}
	scanner := bufio.NewScanner(output)
	var commandOutput strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		commandOutput.WriteString(line)
		commandOutput.WriteByte('\n')
		match := forwardingAddress.FindStringSubmatch(line)
		if len(match) != 2 {
			continue
		}
		d.forwards[key] = localPortForward{cmd: cmd, port: match[1]}
		go func() {
			for scanner.Scan() {
			}
		}()
		return match[1], nil
	}
	if err := scanner.Err(); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return "", err
	}
	if err := cmd.Wait(); err != nil {
		return "", fmt.Errorf("kubectl port-forward for service %s/%s exited before becoming ready: %w: %s", namespace, service, err, strings.TrimSpace(commandOutput.String()))
	}
	return "", fmt.Errorf("kubectl port-forward for service %s/%s exited before becoming ready: %s", namespace, service, strings.TrimSpace(commandOutput.String()))
}

func servicePortForwardTarget(namespace, service, port string) (string, string, error) {
	output, err := exec.Command("kubectl", "-n", namespace, "get", "service", service, "-o", "json").Output()
	if err != nil {
		return "", "", fmt.Errorf("get service %s/%s: %w", namespace, service, err)
	}
	targetPort, useEndpointPod, err := serviceTargetPortFromJSON(output, port)
	if err != nil {
		return "", "", err
	}
	if !useEndpointPod {
		return "service/" + service, targetPort, nil
	}

	output, err = exec.Command("kubectl", "-n", namespace, "get", "endpointslice", "-l", "kubernetes.io/service-name="+service, "-o", "json").Output()
	if err != nil {
		return "", "", fmt.Errorf("get EndpointSlices for service %s/%s: %w", namespace, service, err)
	}
	pod, err := endpointSlicePodFromJSON(output)
	if err != nil {
		return "", "", fmt.Errorf("resolve endpoint pod for service %s/%s: %w", namespace, service, err)
	}
	return "pod/" + pod, targetPort, nil
}

func serviceTargetPortFromJSON(data []byte, port string) (string, bool, error) {
	requestedPort, err := strconv.ParseInt(port, 10, 32)
	if err != nil {
		return "", false, fmt.Errorf("parse service port %q: %w", port, err)
	}

	var service struct {
		Spec struct {
			Ports []struct {
				Port       int32           `json:"port"`
				TargetPort json.RawMessage `json:"targetPort"`
			} `json:"ports"`
		} `json:"spec"`
	}
	if err := json.Unmarshal(data, &service); err != nil {
		return "", false, fmt.Errorf("decode service: %w", err)
	}

	for _, servicePort := range service.Spec.Ports {
		if servicePort.Port != int32(requestedPort) {
			continue
		}
		if len(servicePort.TargetPort) == 0 || string(servicePort.TargetPort) == "null" {
			return port, false, nil
		}
		var targetPortNumber int32
		if err := json.Unmarshal(servicePort.TargetPort, &targetPortNumber); err == nil && targetPortNumber > 0 {
			return strconv.Itoa(int(targetPortNumber)), true, nil
		}
		return "", false, fmt.Errorf("service port %d does not have a numeric targetPort", servicePort.Port)
	}

	return "", false, fmt.Errorf("service does not expose port %s", port)
}

func endpointSlicePodFromJSON(data []byte) (string, error) {
	var endpointSlices struct {
		Items []struct {
			Endpoints []struct {
				Conditions struct {
					Ready *bool `json:"ready"`
				} `json:"conditions"`
				TargetRef struct {
					Kind string `json:"kind"`
					Name string `json:"name"`
				} `json:"targetRef"`
			} `json:"endpoints"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &endpointSlices); err != nil {
		return "", fmt.Errorf("decode EndpointSlices: %w", err)
	}

	for _, endpointSlice := range endpointSlices.Items {
		for _, endpoint := range endpointSlice.Endpoints {
			if endpoint.Conditions.Ready != nil && !*endpoint.Conditions.Ready {
				continue
			}
			if endpoint.TargetRef.Kind != "Pod" || endpoint.TargetRef.Name == "" {
				continue
			}
			return endpoint.TargetRef.Name, nil
		}
	}

	return "", fmt.Errorf("no ready Pod endpoint found")
}

func (d *localGatewayDialer) close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, forward := range d.forwards {
		_ = forward.cmd.Process.Kill()
		_ = forward.cmd.Wait()
	}
}

func TestServiceTargetPortFromJSON(t *testing.T) {
	tests := []struct {
		name           string
		port           string
		json           string
		want           string
		useEndpointPod bool
	}{
		{
			name:           "numeric target port",
			port:           "80",
			json:           `{"spec":{"ports":[{"port":80,"targetPort":30080}]}}`,
			want:           "30080",
			useEndpointPod: true,
		},
		{
			name: "default target port",
			port: "8080",
			json: `{"spec":{"ports":[{"port":8080}]}}`,
			want: "8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, useEndpointPod, err := serviceTargetPortFromJSON([]byte(tt.json), tt.port)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("serviceTargetPortFromJSON() = %q, want %q", got, tt.want)
			}
			if useEndpointPod != tt.useEndpointPod {
				t.Fatalf("serviceTargetPortFromJSON() useEndpointPod = %t, want %t", useEndpointPod, tt.useEndpointPod)
			}
		})
	}
}

func TestEndpointSlicePodFromJSON(t *testing.T) {
	pod, err := endpointSlicePodFromJSON([]byte(`{
  "items": [
    {"endpoints": [
      {"conditions": {"ready": false}, "targetRef": {"kind": "Pod", "name": "not-ready"}},
      {"conditions": {"ready": true}, "targetRef": {"kind": "Pod", "name": "gateway-123"}}
    ]}
  ]
}`))
	if err != nil {
		t.Fatal(err)
	}
	if pod != "gateway-123" {
		t.Fatalf("endpointSlicePodFromJSON() = %q, want gateway-123", pod)
	}
}
