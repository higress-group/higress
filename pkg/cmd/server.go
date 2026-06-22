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
	"fmt"

	"github.com/alibaba/higress/v2/pkg/bootstrap"
	innerconstants "github.com/alibaba/higress/v2/pkg/config/constants"
	"github.com/spf13/cobra"
	"istio.io/istio/pilot/pkg/features"
	"istio.io/istio/pkg/cmd"
	"istio.io/istio/pkg/config/constants"
	"istio.io/istio/pkg/env"
	"istio.io/istio/pkg/keepalive"
	"istio.io/istio/pkg/log"
)

var (
	serverArgs     *bootstrap.ServerArgs
	loggingOptions = log.DefaultOptions()

	serverProvider = func(args *bootstrap.ServerArgs) (bootstrap.ServerInterface, error) {
		return bootstrap.NewServer(args)
	}

	waitForMonitorSignal = func(stop chan struct{}) {
		cmd.WaitSignal(stop)
	}

	keepConfigLabels = env.Register(
		"CONTROLLER_KEEP_XDS_CONFIG_LABELS",
		true,
		"If enabled, Higress Controller will keep all the labels when converting configs to xDS resources."+
			" By default this is enabled. So far, this feature only works for Gateway resource.",
	).Get()

	keepConfigAnnotations = env.Register(
		"CONTROLLER_KEEP_XDS_CONFIG_ANNOTATIONS",
		true,
		"If enabled, Higress Controller will keep the annotations when converting configs to xDS resources."+
			" By default this is enabled. So far, this feature only works for Gateway resource.",
	).Get()
)

// getServerCommand returns the server cobra command to be executed.
func getServerCommand() *cobra.Command {
	serveCmd := &cobra.Command{
		Use:     "serve",
		Aliases: []string{"serve"},
		Short:   "Starts the higress ingress controller",
		Example: "higress serve",
		PreRunE: func(c *cobra.Command, args []string) error {
			return log.Configure(loggingOptions)
		},
		RunE: func(c *cobra.Command, args []string) error {
			cmd.PrintFlags(c.Flags())

			stop := make(chan struct{})

			server, err := serverProvider(serverArgs)
			if err != nil {
				return fmt.Errorf("fail to create higress server: %v", err)
			}

			if err := server.Start(stop); err != nil {
				return fmt.Errorf("fail to start higress server: %v", err)
			}

			waitForMonitorSignal(stop)

			server.WaitUntilCompletion()
			return nil
		},
	}

	serverArgs = &bootstrap.ServerArgs{
		Debug:                true,
		NativeIstio:          true,
		HttpAddress:          ":8888",
		CertHttpAddress:      ":8889",
		GrpcAddress:          ":15051",
		GrpcKeepAliveOptions: keepalive.DefaultOption(),
		XdsOptions: bootstrap.XdsOptions{
			DebounceAfter:         features.DebounceAfter,
			DebounceMax:           features.DebounceMax,
			EnableEDSDebounce:     features.EnableEDSDebounce,
			KeepConfigLabels:      keepConfigLabels,
			KeepConfigAnnotations: keepConfigAnnotations,
		},
	}

	serveCmd.PersistentFlags().StringVar(&serverArgs.GatewaySelectorKey, "gatewaySelectorKey", "higress", "gateway resource selector label key")
	serveCmd.PersistentFlags().StringVar(&serverArgs.GatewaySelectorValue, "gatewaySelectorValue", "higress-system-higress-gateway", "gateway resource selector label value")
	serveCmd.PersistentFlags().BoolVar(&serverArgs.EnableStatus, "enableStatus", true, "enable the ingress status syncer which use to update the ip in ingress's status")
	serveCmd.PersistentFlags().StringVar(&serverArgs.IngressClass, "ingressClass", innerconstants.DefaultIngressClass, "if not empty, only watch the ingresses have the specified class, otherwise watch all ingresses")
	serveCmd.PersistentFlags().StringVar(&serverArgs.GatewayClass, "gatewayClass", innerconstants.DefaultGatewayClass, "if not empty, only process Gateway API resources that belong to the specified GatewayClass")
	serveCmd.PersistentFlags().StringVar(&serverArgs.WatchNamespace, "watchNamespace", "", "if not empty, only wath the ingresses in the specified namespace, otherwise watch in all namespacees")
	serveCmd.PersistentFlags().BoolVar(&serverArgs.Debug, "debug", serverArgs.Debug, "if true, enables more debug http api")
	serveCmd.PersistentFlags().StringVar(&serverArgs.HttpAddress, "httpAddress", serverArgs.HttpAddress, "the http address")
	serveCmd.PersistentFlags().StringVar(&serverArgs.GrpcAddress, "grpcAddress", serverArgs.GrpcAddress, "the grpc address")
	serveCmd.PersistentFlags().BoolVar(&serverArgs.KeepStaleWhenEmpty, "keepStaleWhenEmpty", false, "keep the stale service entry when there are no endpoints in the service")
	serveCmd.PersistentFlags().StringVar(&serverArgs.RegistryOptions.ClusterRegistriesNamespace, "clusterRegistriesNamespace",
		serverArgs.RegistryOptions.ClusterRegistriesNamespace, "Namespace for ConfigMap which stores clusters configs")
	serveCmd.PersistentFlags().StringVar(&serverArgs.RegistryOptions.KubeConfig, "kubeconfig", "",
		"Use a Kubernetes configuration file instead of in-cluster configuration")
	// RegistryOptions Controller options
	serveCmd.PersistentFlags().StringVar(&serverArgs.RegistryOptions.KubeOptions.DomainSuffix, "domain", constants.DefaultClusterLocalDomain,
		"DNS domain suffix")
	serveCmd.PersistentFlags().StringVar((*string)(&serverArgs.RegistryOptions.KubeOptions.ClusterID), "clusterID", "Kubernetes",
		"The ID of the cluster that this instance resides")
	serveCmd.PersistentFlags().StringToStringVar(&serverArgs.RegistryOptions.KubeOptions.ClusterAliases, "clusterAliases", map[string]string{},
		"Alias names for clusters")
	serveCmd.PersistentFlags().Float32Var(&serverArgs.RegistryOptions.KubeOptions.KubernetesAPIQPS, "kubernetesApiQPS", 80.0,
		"Maximum QPS when communicating with the kubernetes API")

	serveCmd.PersistentFlags().IntVar(&serverArgs.RegistryOptions.KubeOptions.KubernetesAPIBurst, "kubernetesApiBurst", 160,
		"Maximum burst for throttle when communicating with the kubernetes API")
	serveCmd.PersistentFlags().Uint32Var(&serverArgs.GatewayHttpPort, "gatewayHttpPort", 80,
		"Http listening port of gateway pod")
	serveCmd.PersistentFlags().Uint32Var(&serverArgs.GatewayHttpsPort, "gatewayHttpsPort", 443,
		"Https listening port of gateway pod")

	serveCmd.PersistentFlags().BoolVar(&serverArgs.EnableAutomaticHttps, "enableAutomaticHttps", false, "if true, enables automatic https")
	serveCmd.PersistentFlags().StringVar(&serverArgs.AutomaticHttpsEmail, "automaticHttpsEmail", "", "email for automatic https")
	serveCmd.PersistentFlags().StringVar(&serverArgs.CertHttpAddress, "certHttpAddress", serverArgs.CertHttpAddress, "the cert http address")

	loggingOptions.AttachCobraFlags(serveCmd)
	serverArgs.GrpcKeepAliveOptions.AttachCobraFlags(serveCmd)

	return serveCmd
}
