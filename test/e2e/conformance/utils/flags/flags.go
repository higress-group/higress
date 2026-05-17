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

package flags

import (
	"flag"
)

var (
	IngressClassName     = flag.String("ingress-class", "higress", "Name of IngressClass to use for tests")
	ShowDebug            = flag.Bool("debug", false, "Whether to print debug logs")
	CleanupBaseResources = flag.Bool("cleanup-base-resources", true, "Whether to cleanup base test resources after the run")
	SupportedFeatures    = flag.String("supported-features", "", "Supported features included in conformance tests suites")
	ExemptFeatures       = flag.String("exempt-features", "", "Exempt Features excluded from conformance tests suites")
	ExecuteTests         = flag.String("execute-tests", "", "Execute the specific conformance tests")
	IsWasmPluginTest     = flag.Bool("isWasmPluginTest", true, "Determine if run wasm plugin conformance test")
	WasmPluginType       = flag.String("wasmPluginType", "GO", "Define wasm plugin type, currently supports GO, CPP")
	WasmPluginName       = flag.String("wasmPluginName", "", "Define wasm plugin name")
	IsEnvoyConfigTest    = flag.Bool("isEnvoyConfigTest", false, "Determine if run envoy config conformance test")
	TestArea             = flag.String("test-area", "all", "Test area to run, like all to run setup/run/clean, setup to prepare test environment, run to run test cases, clean to clean test environment")
)
