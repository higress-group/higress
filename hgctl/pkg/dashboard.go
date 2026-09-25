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

package hgctl

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/types"

	"github.com/alibaba/higress/hgctl/pkg/docker"
	"github.com/alibaba/higress/hgctl/pkg/kubernetes"
	"github.com/alibaba/higress/v2/pkg/cmd/options"
)

var (
	listenPort     = 0
	promPort       = 0
	grafanaPort    = 0
	consolePort    = 0
	controllerPort = 0

	bindAddress = "localhost"

	// open browser or not, default is true
	browser = true

	// label selector
	labelSelector = ""

	addonNamespace = ""

	envoyDashNs = ""

	proxyAdminPort int

	project = "higress"

	dockerCli = false
)

const (
	defaultPrometheusPort = 9090
	defaultGrafanaPort    = 3000
	defaultConsolePort    = 8080
	defaultControllerPort = 8888
)

func newDashboardCmd() *cobra.Command {
	dashboardCmd := &cobra.Command{
		Use:     "dashboard",
		Aliases: []string{"dash", "d"},
		Short:   "Access to Higress web UIs",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return fmt.Errorf("unknown dashboard %q", args[0])
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.HelpFunc()(cmd, args)
			return nil
		},
	}

	dashboardCmd.PersistentFlags().IntVarP(&listenPort, "port", "p", 0, "Local port to listen to")
	dashboardCmd.PersistentFlags().BoolVar(&browser, "browser", true,
		"When --browser is supplied as false, hgctl dashboard will not open the browser. "+
			"Default is true which means hgctl dashboard will always open a browser to view the dashboard.")
	dashboardCmd.PersistentFlags().StringVarP(&addonNamespace, "namespace", "n", "higress-system",
		"Namespace where the addon is running, if not specified, higress-system would be used")
	dashboardCmd.PersistentFlags().StringVarP(&bindAddress, "listen", "l", "localhost", "The address to bind to")

	prom := promDashCmd()
	prom.PersistentFlags().IntVar(&promPort, "ui-port", defaultPrometheusPort, "The component dashboard UI port.")
	dashboardCmd.AddCommand(prom)

	graf := grafanaDashCmd()
	graf.PersistentFlags().IntVar(&grafanaPort, "ui-port", defaultGrafanaPort, "The component dashboard UI port.")
	dashboardCmd.AddCommand(graf)

	envoy := envoyDashCmd()
	envoy.PersistentFlags().StringVarP(&labelSelector, "selector", "s", "app=higress-gateway", "Label selector")
	envoy.PersistentFlags().StringVarP(&envoyDashNs, "namespace", "n", "",
		"Namespace where the addon is running, if not specified, higress-system would be used")
	envoy.PersistentFlags().IntVar(&proxyAdminPort, "ui-port", defaultProxyAdminPort, "The component dashboard UI port.")
	dashboardCmd.AddCommand(envoy)

	consoleCmd := consoleDashCmd()
	consoleCmd.PersistentFlags().IntVar(&consolePort, "ui-port", defaultConsolePort, "The component dashboard UI port.")
	consoleCmd.PersistentFlags().BoolVar(&dockerCli, "docker", false, "Search higress console from docker")
	dashboardCmd.AddCommand(consoleCmd)

	controllerDebugCmd := controllerDebugCmd()
	controllerDebugCmd.PersistentFlags().IntVar(&controllerPort, "ui-port", defaultControllerPort, "The component dashboard UI port.")
	dashboardCmd.AddCommand(controllerDebugCmd)
	flags := dashboardCmd.PersistentFlags()
	options.AddKubeConfigFlags(flags)
	return dashboardCmd
}

// port-forward to Higress System Prometheus; open browser
func promDashCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "prometheus",
		Short: "Open Prometheus web UI",
		Long:  `Open Higress's Prometheus dashboard`,
		Example: `  hgctl dashboard prometheus

  # with short syntax
  hgctl dash prometheus
  hgctl d prometheus`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := kubernetes.NewCLIClient(options.DefaultConfigFlags.ToRawKubeConfigLoader())
			if err != nil {
				return fmt.Errorf("build CLI client fail: %w", err)
			}

			pl, err := client.PodsForSelector(addonNamespace, "app=higress-console-prometheus")
			if err != nil {
				return fmt.Errorf("not able to locate Prometheus pod: %v", err)
			}

			if len(pl.Items) < 1 {
				return errors.New("no Prometheus pods found")
			}

			// only use the first pod in the list
			return portForward(pl.Items[0].Name, addonNamespace, "Prometheus",
				"http://%s", bindAddress, promPort, client, cmd.OutOrStdout(), browser)
		},
	}

	return cmd
}

// port-forward to Higress System Console; open browser
func consoleDashCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "console",
		Short: "Open Console web UI",
		Long:  `Open Higress Console`,
		Example: `  hgctl dashboard console

  # with short syntax
  hgctl dash console
  hgctl d console`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dockerCli {
				return accessDockerCompose(cmd)
			}
			client, err := kubernetes.NewCLIClient(options.DefaultConfigFlags.ToRawKubeConfigLoader())
			if err != nil {
				fmt.Printf("build kubernetes CLI client fail: %v\ntry to access docker container\n", err)
				return accessDockerCompose(cmd)
			}
			pl, err := client.PodsForSelector(addonNamespace, "app.kubernetes.io/name=higress-console")
			if err != nil {
				fmt.Printf("build kubernetes CLI client fail: %v\ntry to access docker container\n", err)
				return accessDockerCompose(cmd)
			}

			if len(pl.Items) < 1 {
				fmt.Printf("no higress console pods found\ntry to access docker container\n")
				return accessDockerCompose(cmd)
			}

			// only use the first pod in the list
			return portForward(pl.Items[0].Name, addonNamespace, "Console",
				"http://%s", bindAddress, consolePort, client, cmd.OutOrStdout(), browser)
		},
	}

	return cmd
}

// accessDockerCompose access docker compose ps
func accessDockerCompose(cmd *cobra.Command) error {
	cli, err := docker.NewCompose(cmd.OutOrStdout())
	if err != nil {
		return errors.Wrap(err, "failed to build the docker compose client")
	}

	list, err := cli.Ps(context.TODO(), project)
	if err != nil {
		return errors.Wrap(err, "failed to build the docker compose ps")
	}
	for _, container := range list {
		if strings.Contains(container.Service, "console") {
			// not support define ip address
			if len(container.Publishers) > 0 {
				url := fmt.Sprintf("http://localhost:%d", container.Publishers[0].PublishedPort)
				openBrowser(url, cmd.OutOrStdout(), browser)
			}

			return nil
		}
	}
	return errors.New("no higress console container found")
}

// port-forward to Higress System Grafana; open browser
func grafanaDashCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "grafana",
		Short: "Open Grafana web UI",
		Long:  `Open Higress's Grafana dashboard`,
		Example: `  hgctl dashboard grafana

  # with short syntax
  hgctl dash grafana
  hgctl d grafana`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := kubernetes.NewCLIClient(options.DefaultConfigFlags.ToRawKubeConfigLoader())
			if err != nil {
				return fmt.Errorf("build CLI client fail: %w", err)
			}
			pl, err := client.PodsForSelector(addonNamespace, "app=higress-console-grafana")
			if err != nil {
				return fmt.Errorf("not able to locate Grafana pod: %v", err)
			}

			if len(pl.Items) < 1 {
				return errors.New("no Grafana pods found")
			}

			// only use the first pod in the list
			return portForward(pl.Items[0].Name, addonNamespace, "Grafana",
				"http://%s", bindAddress, grafanaPort, client, cmd.OutOrStdout(), browser)
		},
	}

	return cmd
}

// port-forward to sidecar Envoy admin port; open browser
func envoyDashCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "envoy [<type>/]<name>[.<namespace>]",
		Short: "Open Envoy admin web UI",
		Long:  `Open the Envoy admin dashboard for a higress gateway`,
		Example: `  # Open Envoy dashboard for the higress-gateway-56f9b9797-b9nnc
  hgctl dashboard envoy higress-gateway-56f9b9797-b9nnc

  # with short syntax
  hgctl dash envoy
  hgctl d envoy
`,
		RunE: func(c *cobra.Command, args []string) error {
			kubeClient, err := kubernetes.NewCLIClient(options.DefaultConfigFlags.ToRawKubeConfigLoader())
			if err != nil {
				return fmt.Errorf("build CLI client fail: %w", err)
			}

			if labelSelector == "" && len(args) < 1 {
				c.Println(c.UsageString())
				return fmt.Errorf("specify a pod or --selector")
			}

			if err != nil {
				return fmt.Errorf("failed to create k8s client: %v", err)
			}

			var podName, ns string
			if labelSelector != "" {
				pl, err := kubeClient.PodsForSelector(envoyDashNs, labelSelector)
				if err != nil {
					return fmt.Errorf("not able to locate pod with selector %s: %v", labelSelector, err)
				}

				if len(pl.Items) < 1 {
					return errors.New("no pods found")
				}
				// only use the first pod in the list
				podName = pl.Items[0].Name
				ns = pl.Items[0].Namespace
			} else if len(args) > 0 {
				po, err := kubeClient.Pod(types.NamespacedName{Name: args[0], Namespace: envoyDashNs})
				if err != nil {
					return err
				}

				podName = po.Name
				ns = po.Namespace
			}

			return portForward(podName, ns, fmt.Sprintf("Envoy sidecar %s", podName),
				"http://%s", bindAddress, proxyAdminPort, kubeClient, c.OutOrStdout(), browser)
		},
	}

	return cmd
}

// port-forward to Higress System Console; open browser
func controllerDebugCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "controller",
		Short: "Open Controller debug web UI",
		Long:  `Open Higress Controller`,
		Example: `  hgctl dashboard controller

  # with short syntax
  hgctl dash controller
  hgctl d controller`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := kubernetes.NewCLIClient(options.DefaultConfigFlags.ToRawKubeConfigLoader())
			if err != nil {
				return fmt.Errorf("build CLI client fail: %w", err)
			}

			pl, err := client.PodsForSelector(addonNamespace, "app=higress-controller")
			if err != nil {
				return fmt.Errorf("not able to locate controller pod: %v", err)
			}

			if len(pl.Items) < 1 {
				return errors.New("no higress controller pods found")
			}

			// only use the first pod in the list
			return portForward(pl.Items[0].Name, addonNamespace, "Controller",
				"http://%s/debug", bindAddress, controllerPort, client, cmd.OutOrStdout(), browser)
		},
	}

	return cmd
}

// portForward first tries to forward localhost:remotePort to podName:remotePort, falls back to dynamic local port
func portForward(podName, namespace, flavor, urlFormat, localAddress string, remotePort int,
	client kubernetes.CLIClient, writer io.Writer, browser bool,
) error {
	// port preference:
	// - If --listenPort is specified, use it
	// - without --listenPort, prefer the remotePort but fall back to a random port
	var portPrefs []int
	if listenPort != 0 {
		portPrefs = []int{listenPort}
	} else {
		portPrefs = []int{remotePort}
	}

	var err error
	for _, localPort := range portPrefs {
		var fw kubernetes.PortForwarder
		fw, err = kubernetes.NewLocalPortForwarder(client, types.NamespacedName{Namespace: namespace, Name: podName}, localPort, remotePort, bindAddress)
		if err != nil {
			return fmt.Errorf("could not build port forwarder for %s: %v", flavor, err)
		}

		if err := fw.Start(); err != nil {
			fw.Stop()
			// Try the next port
			continue
		}

		// Close the port forwarder when the command is terminated.
		ClosePortForwarderOnInterrupt(fw)

		openBrowser(fmt.Sprintf(urlFormat, fw.Address()), writer, browser)

		// Wait for stop
		fw.WaitForStop()
	}

	if err != nil {
		return fmt.Errorf("failure running port forward process: %v", err)
	}
	return nil
}

func ClosePortForwarderOnInterrupt(fw kubernetes.PortForwarder) {
	go func() {
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, os.Interrupt)
		defer signal.Stop(signals)
		<-signals
		fw.Stop()
	}()
}

func openBrowser(url string, writer io.Writer, browser bool) {
	fmt.Fprintf(writer, "%s\n", url)

	if !browser {
		fmt.Fprint(writer, "skipping opening a browser")
		return
	}

	switch runtime.GOOS {
	case "linux":
		openCommand(writer, "xdg-open", url)
	case "windows":
		openCommand(writer, "rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		openCommand(writer, "open", url)
	default:
		fmt.Fprintf(writer, "Unsupported platform %q; open %s in your browser.\n", runtime.GOOS, url)
	}
}

func openCommand(writer io.Writer, command string, args ...string) {
	_, err := exec.LookPath(command)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			fmt.Fprintf(writer, "Could not open your browser. Please open it manually.\n")
			return
		}
		fmt.Fprintf(writer, "Failed to open browser; open %s in your browser.\nError: %s\n", args[0], err.Error())
		return
	}

	err = exec.Command(command, args...).Start()
	if err != nil {
		fmt.Fprintf(writer, "Failed to open browser; open %s in your browser.\nError: %s\n", args[0], err.Error())
	}
}
