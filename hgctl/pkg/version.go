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
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/alibaba/higress/hgctl/pkg/kubernetes"
	"github.com/alibaba/higress/v2/pkg/cmd/options"
	"github.com/alibaba/higress/v2/pkg/cmd/version"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

const (
	higressCoreContainerName    = "higress-core"
	higressGatewayContainerName = "higress-gateway"
)

func newVersionCommand() *cobra.Command {
	var (
		output string
		client bool
	)

	versionCommand := &cobra.Command{
		Use:     "version",
		Aliases: []string{"versions", "v"},
		Short:   "Show version",
		Example: `  # Show versions of both client and server.
  hgctl version

  # Show versions of both client and server in JSON format.
  hgctl version --output=json

  # Show version of client without server.
  hgctl version --client
	  `,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(versions(cmd.OutOrStdout(), output, client))
		},
	}

	flags := versionCommand.Flags()
	options.AddKubeConfigFlags(flags)

	versionCommand.PersistentFlags().StringVarP(&output, "output", "o", yamlOutput, "One of 'yaml' or 'json'")

	versionCommand.PersistentFlags().BoolVarP(&client, "client", "r", false, "If true, only log client version.")

	return versionCommand
}

type VersionInfo struct {
	ClientVersion  string           `json:"client" yaml:"client"`
	ServerVersions []*ServerVersion `json:"server,omitempty" yaml:"server"`
}

type ServerVersion struct {
	types.NamespacedName `yaml:"namespacedName"`
	version.Info         `yaml:"versionInfo"`
}

func Get() VersionInfo {
	return VersionInfo{
		ClientVersion:  version.Get().HigressVersion,
		ServerVersions: make([]*ServerVersion, 0),
	}
}

func retrieveVersion(w io.Writer, v *VersionInfo, containerName string, cmd string, labelSelector string, c kubernetes.CLIClient, f versionFunc) error {
	pods, err := c.PodsForSelector(metav1.NamespaceAll, labelSelector)
	if err != nil {
		return errors.Wrap(err, "list Higress pods failed")
	}

	for _, pod := range pods.Items {
		if pod.Status.Phase != v1.PodRunning {

			fmt.Fprintf(w, "WARN: pod %s/%s is not running, skipping it.", pod.Namespace, pod.Name)
			continue
		}

		nn := types.NamespacedName{
			Namespace: pod.Namespace,
			Name:      pod.Name,
		}
		stdout, _, err := c.PodExec(nn, containerName, cmd)
		if err != nil {
			return fmt.Errorf("pod exec on %s/%s failed: %w", nn.Namespace, nn.Name, err)
		}

		info, err := f(stdout)
		if err != nil {
			return err
		}
		if info == nil {
			continue
		}

		v.ServerVersions = append(v.ServerVersions, &ServerVersion{
			NamespacedName: nn,
			Info:           *info,
		})
	}

	return nil
}

type versionFunc func(string) (*version.Info, error)

func versions(w io.Writer, output string, client bool) error {
	v := Get()

	if client {
		fmt.Fprintf(w, "clientVersion: %s", v.ClientVersion)
		return nil
	}

	c, err := kubernetes.NewCLIClient(options.DefaultConfigFlags.ToRawKubeConfigLoader())
	if err != nil {
		return fmt.Errorf("failed to build kubernetes client: %w", err)
	}

	if err := retrieveVersion(w, &v, higressCoreContainerName, "higress version -ojson", "app=higress-controller", c, func(s string) (*version.Info, error) {
		info := &version.Info{}
		if err := json.Unmarshal([]byte(s), info); err != nil {
			return nil, fmt.Errorf("unmarshall pod exec result failed: %w", err)
		}
		info.Type = "higress-controller"
		return info, nil
	}); err != nil {
		return err
	}

	if err := retrieveVersion(w, &v, higressGatewayContainerName, "envoy --version", "app=higress-gateway", c, func(s string) (*version.Info, error) {
		if len(strings.Split(s, ":")) != 2 {
			return nil, nil
		}
		proxyVersion := strings.TrimSpace(strings.Split(s, ":")[1])
		return &version.Info{
			GatewayVersion: proxyVersion,
			Type:           "higress-gateway",
		}, nil
	}); err != nil {
		return err
	}

	sort.Slice(v.ServerVersions, func(i, j int) bool {
		if v.ServerVersions[i].Namespace == v.ServerVersions[j].Namespace {
			return v.ServerVersions[i].Name < v.ServerVersions[j].Name
		}

		return v.ServerVersions[i].Namespace < v.ServerVersions[j].Namespace
	})

	var out []byte
	switch output {
	case yamlOutput:
		out, err = yaml.Marshal(v)
	case jsonOutput:
		out, err = json.MarshalIndent(v, "", "  ")
	default:
		out, err = json.MarshalIndent(v, "", "  ")
	}

	if err != nil {
		return err
	}
	fmt.Fprintln(w, string(out))

	return nil
}
