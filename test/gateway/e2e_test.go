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
	"fmt"
	"net"
	"os"
	"os/exec"
	"regexp"
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

	cmd := exec.Command("kubectl", "-n", namespace, "port-forward", "--address=127.0.0.1", "service/"+service, ":"+remotePort)
	output, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return "", err
	}
	scanner := bufio.NewScanner(output)
	for scanner.Scan() {
		match := forwardingAddress.FindStringSubmatch(scanner.Text())
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
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("kubectl port-forward for service %s/%s exited before becoming ready", namespace, service)
}

func (d *localGatewayDialer) close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, forward := range d.forwards {
		_ = forward.cmd.Process.Kill()
		_ = forward.cmd.Wait()
	}
}
