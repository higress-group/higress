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

package test

import (
	"flag"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/networking/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/alibaba/higress/v2/test/e2e/conformance/tests"
	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/flags"
	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/suite"
)

func TestHigressConformanceTests(t *testing.T) {
	flag.Parse()

	cfg, err := config.GetConfig()
	require.NoError(t, err)

	client, err := client.New(cfg, client.Options{})
	require.NoError(t, err)

	require.NoError(t, v1.AddToScheme(client.Scheme()))

	cSuite := suite.New(suite.Options{
		Client:               client,
		IngressClassName:     *flags.IngressClassName,
		Debug:                *flags.ShowDebug,
		CleanupBaseResources: *flags.CleanupBaseResources,
		WASMOptions: suite.WASMOptions{
			IsWasmPluginTest: *flags.IsWasmPluginTest,
			WasmPluginName:   *flags.WasmPluginName,
			WasmPluginType:   *flags.WasmPluginType,
		},
		ExecuteTests:               *flags.ExecuteTests,
		GatewayAddress:             *flags.GatewayAddress,
		EnableAllSupportedFeatures: true,
		IsEnvoyConfigTest:          *flags.IsEnvoyConfigTest,
	})

	// Run the test suite
	testArea := *flags.TestArea
	testArea = strings.ToLower(testArea)
	switch testArea {
	case suite.TestAreaAll:
		cSuite.Setup(t)
		cSuite.Run(t, tests.ConformanceTests)
	case suite.TestAreaRun:
		cSuite.Run(t, tests.ConformanceTests)
	case suite.TestAreaSetup:
		cSuite.Cleanup = false
		cSuite.Setup(t)
	case suite.TestAreaClean:
		cSuite.Clean(t)
	}
}
