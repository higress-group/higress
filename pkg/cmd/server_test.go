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

package cmd

import (
	"os"
	"testing"
	"time"

	"github.com/alibaba/higress/v2/pkg/bootstrap"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"istio.io/istio/pilot/pkg/features"
)

func TestServe(t *testing.T) {
	serveCmd := getServerCommand()
	runEBackup := serveCmd.RunE
	argsBackup := os.Args
	serverProviderBackup := serverProvider
	executed := false

	serverProvider = func(args *bootstrap.ServerArgs) (bootstrap.ServerInterface, error) {
		return &delayedServer{Args: args, Delay: time.Second * 5}, nil
	}

	serveCmd.RunE = func(cmd *cobra.Command, args []string) error {
		executed = true
		return runEBackup(cmd, args)
	}
	defer func() {
		serverProvider = serverProviderBackup
		os.Args = argsBackup
		serveCmd.RunE = runEBackup
	}()

	a := assert.New(t)

	delay := time.Second * 5

	start := time.Now()
	os.Args = []string{"/app/higress", "serve"}
	waitForMonitorSignal = func(stop chan struct{}) {
		time.Sleep(delay)
		close(stop)
	}

	serveCmd.Execute()

	end := time.Now()

	cost := end.Sub(start)
	a.GreaterOrEqual(cost, delay)

	a.True(executed)
}

// TestServeGrpcAddressDefault pins the shipped default for the MCP-over-xDS config-push
// listener. It must stay a wildcard bind: `hgctl code-debug start` rewrites istiod's
// configSources entry to xds://<non-loopback-IPv4>:15051 (hgctl/pkg/code_debug.go), so a
// developer-run controller has to be reachable off-loopback. Co-located deployments that
// want a loopback-only bind use --grpcAddress.
func TestServeGrpcAddressDefault(t *testing.T) {
	getServerCommand()

	a := assert.New(t)
	a.Equal(":15051", serverArgs.GrpcAddress)
}

func TestServeGrpcAddressOverride(t *testing.T) {
	serveCmd := getServerCommand()

	a := assert.New(t)
	a.NoError(serveCmd.ParseFlags([]string{"--grpcAddress", "127.0.0.1:15051"}))
	a.Equal("127.0.0.1:15051", serverArgs.GrpcAddress)
}

// TestXDSAuthFeatureDefault pins the contract Server.initAuthenticators relies on: the xDS
// authenticators are wired by default. The flag alone authenticates nothing on the plaintext
// :15051 listener, because pkg/security/authentication.go skips authentication for non-TLS
// peers unless XDS_AUTH_PLAINTEXT is also set.
func TestXDSAuthFeatureDefault(t *testing.T) {
	a := assert.New(t)
	a.True(features.XDSAuth)
}

type delayedServer struct {
	Args  *bootstrap.ServerArgs
	Delay time.Duration
	stop  <-chan struct{}
}

func (d *delayedServer) Start(stop <-chan struct{}) error {
	d.stop = stop
	return nil
}

func (d *delayedServer) WaitUntilCompletion() {
	<-d.stop
}
