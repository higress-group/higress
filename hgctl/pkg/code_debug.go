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
	"net"
	"os"
	"regexp"
	"time"

	"github.com/alibaba/higress/hgctl/pkg/helm"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	DefaultIp   = "127.0.0.1"
	DefaultPort = ":15051"
)

func newCodeDebugCmd() *cobra.Command {
	codeDebugCmd := &cobra.Command{
		Use:   "code-debug",
		Short: "Start or stop code debug",
	}

	codeDebugCmd.AddCommand(getStartCodeDebugCmd())
	codeDebugCmd.AddCommand(getStopCodeDebugCmd())

	return codeDebugCmd
}

func getStartCodeDebugCmd() *cobra.Command {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("fail to get user home dir: %v", err)
		os.Exit(1)
	}
	kubeConfigDir := homeDir + "/.kube/config"

	startCodeDebugCmd := &cobra.Command{
		Use:     "start",
		Aliases: []string{"start"},
		Short:   "Start code debug",
		Example: "hgctl code-debug start",
		RunE: func(c *cobra.Command, args []string) error {
			writer := c.OutOrStdout()

			// wait for user to confirm
			if !promptCodeDebug(writer, "local grpc address") {
				return nil
			}

			// check profile type is local or not
			fmt.Fprintf(writer, "Checking profile type...\n")
			profiles, err := getAllProfiles()
			if err != nil {
				return fmt.Errorf("fail to get all profiles: %v", err)
			}
			if len(profiles) == 0 {
				fmt.Fprintf(writer, "Higress hasn't been installed yet!\n")
				return nil
			}
			for _, profile := range profiles {
				if profile.Install != helm.InstallLocalK8s {
					fmt.Fprintf(writer, "\nHigress needs to be installed locally!\n")
					return nil
				}
			}

			// get kubernetes clientSet
			fmt.Fprintf(writer, "Getting kubernetes clientset...\n")
			config, err := clientcmd.BuildConfigFromFlags("", kubeConfigDir)
			if err != nil {
				fmt.Fprintf(writer, "fail to build config from kubeconfig: %v", err)
				return nil
			}
			clientSet, err := kubernetes.NewForConfig(config)
			if err != nil {
				fmt.Fprintf(writer, "fail to create kubernetes clientset: %v", err)
				return nil
			}

			// get non-loopback IPv4 address
			fmt.Fprintf(writer, "Getting non-loopback IPv4 address...\n")
			ip, err := getNonLoopbackIPv4()
			if err != nil {
				fmt.Fprintf(writer, "fail to get non-loopback IPv4 address: %v", err)
				return nil
			}

			// update the xds address in higress-config ConfigMap
			// and trigger rollout for higress-controller and higress-gateway deployments
			fmt.Fprintf(writer, "Updating xds address in higress-config ConfigMap "+
				"and triggering rollout for higress-controller and higress-gateway deployments...\n")
			err = updateXdsIpAndRollout(clientSet, ip, DefaultPort)
			if err != nil {
				fmt.Fprintf(writer, "fail to update xds address in higress-config ConfigMap: %v", err)
				return nil
			}

			fmt.Fprintf(writer, "Code debug started!\n")

			return nil
		},
	}

	startCodeDebugCmd.PersistentFlags().StringVar(&kubeConfigDir, "kubeconfig", kubeConfigDir,
		"Use a Kubernetes configuration file instead of in-cluster configuration")

	return startCodeDebugCmd
}

func getStopCodeDebugCmd() *cobra.Command {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("fail to get user home dir: %v", err)
		os.Exit(1)
	}
	kubeConfigDir := homeDir + "/.kube/config"

	stopCodeDebugCmd := &cobra.Command{
		Use:     "stop",
		Aliases: []string{"stop"},
		Short:   "Stop code debug",
		Example: "hgctl code-debug stop",
		RunE: func(c *cobra.Command, args []string) error {
			// wait for user to confirm
			writer := c.OutOrStdout()
			if !promptCodeDebug(writer, "default grpc address") {
				return nil
			}

			// check profile type is local or not
			fmt.Fprintf(writer, "Checking profile type...\n")
			profiles, err := getAllProfiles()
			if err != nil {
				return fmt.Errorf("fail to get all profiles: %v", err)
			}
			if len(profiles) == 0 {
				fmt.Fprintf(writer, "Higress hasn't been installed yet!\n")
				return nil
			}
			for _, profile := range profiles {
				if profile.Install != helm.InstallLocalK8s {
					fmt.Fprintf(writer, "\nHigress needs to be installed locally!\n")
					return nil
				}
			}

			// get kubernetes clientSet
			fmt.Fprintf(writer, "Getting kubernetes clientset...\n")
			config, err := clientcmd.BuildConfigFromFlags("", kubeConfigDir)
			if err != nil {
				fmt.Fprintf(writer, "fail to build config from kubeconfig: %v", err)
				return nil
			}
			clientSet, err := kubernetes.NewForConfig(config)
			if err != nil {
				fmt.Fprintf(writer, "fail to create kubernetes clientset: %v", err)
				return nil
			}

			// recover the xds address in higress-config ConfigMap
			// and trigger rollout for higress-controller and higress-gateway deployments
			fmt.Fprintf(writer, "Recovering xds address in higress-config ConfigMap "+
				"and triggering rollout for higress-controller and higress-gateway deployments...\n")
			err = updateXdsIpAndRollout(clientSet, DefaultIp, DefaultPort)
			if err != nil {
				fmt.Fprintf(writer, "fail to recover xds address in higress-config ConfigMap: %v", err)
				return nil
			}

			fmt.Fprintf(writer, "Code debug stopped!\n")

			return nil
		},
	}

	stopCodeDebugCmd.PersistentFlags().StringVar(&kubeConfigDir, "kubeconfig", kubeConfigDir,
		"Use a Kubernetes configuration file instead of in-cluster configuration")

	return stopCodeDebugCmd
}

// getNonLoopbackIPv4 returns the first non-loopback IPv4 address of the host.
func getNonLoopbackIPv4() (string, error) {
	// get all network interfaces
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}

	// traverse all network interfaces
	for _, i := range interfaces {
		// exclude loopback interface and virtual interface
		if i.Flags&net.FlagLoopback == 0 && i.Flags&net.FlagUp != 0 {
			// get all addresses of the interface
			addrs, err := i.Addrs()
			if err != nil {
				return "", err
			}

			// traverse all addresses of the interface
			for _, addr := range addrs {
				// check the type of the address is IP address
				if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
					// check the IP address is IPv4 address
					if ipnet.IP.To4() != nil {
						return ipnet.IP.String(), nil
					}
				}
			}
		}
	}

	return "", fmt.Errorf("Non-loopback IPv4 address not found")
}

// updateXdsIpAndRollout updates the xds address in higress-config ConfigMap
// and triggers rollout for higress-controller and higress-gateway deployments
// also can recover the xds address in higress-config ConfigMap
func updateXdsIpAndRollout(c *kubernetes.Clientset, ip string, port string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get higress-config ConfigMap
	cm, err := c.CoreV1().ConfigMaps("higress-system").Get(ctx, "higress-config", metav1.GetOptions{})
	if err != nil {
		return err
	}

	// Update mesh field in higress-config ConfigMap
	if _, ok := cm.Data["mesh"]; !ok {
		return fmt.Errorf("mesh not found in configmap higress-config")
	}
	mesh := cm.Data["mesh"]
	newMesh, err := replaceXDSAddress(mesh, ip, port)
	if err != nil {
		return err
	}
	cm.Data["mesh"] = newMesh

	// Update higress-config ConfigMap
	_, err = c.CoreV1().ConfigMaps("higress-system").Update(ctx, cm, metav1.UpdateOptions{})
	if err != nil {
		return err
	}

	// Trigger rollout for higress-controller deployment
	err = triggerRollout(c, "higress-controller")
	if err != nil {
		return err
	}

	// Trigger rollout for higress-gateway deployment
	err = triggerRollout(c, "higress-gateway")
	if err != nil {
		return err
	}

	return nil
}

// triggerRollout triggers rollout for the specified deployment
func triggerRollout(clientset *kubernetes.Clientset, deploymentName string) error {
	deploymentsClient := clientset.AppsV1().Deployments("higress-system")

	// Get the deployment
	deployment, err := deploymentsClient.Get(context.TODO(), deploymentName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	// Increment the deployment's revision to trigger a rollout
	if deployment.Spec.Template.ObjectMeta.Labels == nil {
		deployment.Spec.Template.ObjectMeta.Labels = make(map[string]string)
	}
	deployment.Spec.Template.ObjectMeta.Labels["version"] = time.Now().Format("20060102150405")

	// Update the deployment
	_, err = deploymentsClient.Update(context.TODO(), deployment, metav1.UpdateOptions{})
	if err != nil {
		return err
	}

	return nil
}

// replaceXDSAddress replaces the xds address in the config string with new IP and Port
func replaceXDSAddress(configString, newIP, newPort string) (string, error) {
	// define the regular expression to match xds address
	xdsRegex := regexp.MustCompile(`xds://[0-9.:]+`)

	// find the first match
	match := xdsRegex.FindString(configString)
	if match == "" {
		// if no match, return error
		return "", fmt.Errorf("no xds address found in config string")
	}

	// replace xds address with new IP and Port
	newXDSAddress := fmt.Sprintf("xds://%s%s", newIP, newPort)
	result := xdsRegex.ReplaceAllString(configString, newXDSAddress)

	return result, nil
}

// promptCodeDebug prompts user to confirm code debug
func promptCodeDebug(writer io.Writer, t string) bool {
	answer := ""
	for {
		fmt.Fprintf(writer, "This will start set xds address to %s in higress-config ConfigMap "+
			"and trigger rollout for higress-controller and higress-gateway deployments. \nProceed? (y/N)", t)
		fmt.Scanln(&answer)
		if answer == "y" {
			return true
		}
		if answer == "N" {
			fmt.Fprintf(writer, "Cancelled.\n")
			return false
		}
	}
}
