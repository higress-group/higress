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

package agent

import (
	"fmt"
	"io"

	"github.com/alibaba/higress/hgctl/pkg/agent/services"
	"github.com/spf13/cobra"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

// API Type
const (
	A2A   = "a2a"
	REST  = "restful"
	MODEL = "model"
)

func NewAgentCmd() *cobra.Command {
	agentCmd := &cobra.Command{
		Use:   "agent",
		Short: "Start the interactive agent window",
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(invokeAgentCore(cmd.OutOrStdout()))
		},
	}

	agentCmd.AddCommand(createAgentCmd())
	agentCmd.AddCommand(deployAgentCmd())
	agentCmd.AddCommand(newAgentAddCmd())

	return agentCmd
}

func invokeAgentCore(w io.Writer) error {
	core, err := getCore()
	if err != nil {
		return fmt.Errorf("failed to get core: %s", err)
	}
	return core.Start()
}

type AgentAddArg struct {
	HigressConsoleAuthArg
	HimarketAdminAuthArg

	name            string
	url             string
	typ             string
	scope           string
	externalBaseURL string
	a2aPluginURL    string

	asProduct bool
	noPublish bool
}

func newAgentAddCmd() *cobra.Command {
	arg := &AgentAddArg{}

	cmd := &cobra.Command{
		Use:   "add [name] [url]",
		Short: "add agent to local interactive window and publish it to higress (optional)",
		Run: func(cmd *cobra.Command, args []string) {
			arg.name = args[0]
			arg.url = args[1]

			resolveHigressConsoleAuth(&arg.HigressConsoleAuthArg)
			resolveHimarketAdminAuth(&arg.HimarketAdminAuthArg)
			cmdutil.CheckErr(handleAddAgent(cmd.OutOrStdout(), *arg))
		},
		Args: cobra.ExactArgs(2),
	}

	cmd.PersistentFlags().StringVarP(&arg.typ, "type", "t", MODEL, "Determine the agent's API type (a2a, model, restful) default is model")
	cmd.PersistentFlags().StringVarP(&arg.scope, "scope", "s", "project", `Configuration scope (project or global)`)
	cmd.PersistentFlags().StringVar(&arg.externalBaseURL, "a2a-external-base-url", "", "Public HTTPS JSON-RPC URL advertised by an A2A Agent Card")
	cmd.PersistentFlags().StringVar(&arg.a2aPluginURL, "a2a-plugin-url", "", "Operator-approved OCI image URL for the A2A protocol plugin (tag required)")
	cmd.PersistentFlags().BoolVar(&arg.noPublish, "no-publish", false, "If it's set then the agent API will not be plubished to Higress")
	cmd.PersistentFlags().BoolVar(&arg.asProduct, "as-product", false, "If it's set then the agent API will be published to Himarket (no-publish must be false)")

	addHigressConsoleAuthFlag(cmd, &arg.HigressConsoleAuthArg)
	addHimarketAdminAuthFlag(cmd, &arg.HimarketAdminAuthArg)
	return cmd
}

func handleAddAgent(writer io.Writer, arg AgentAddArg) error {
	if err := validateArg(arg); err != nil {
		return err
	}

	if !arg.noPublish {
		if err := publishAgentAPIToHigress(arg); err != nil {
			fmt.Printf("failed to publish agent api to higress: %s\n", err)
			return err
		}

		fmt.Printf("Agent %s is published to Higress successfully\n", arg.name)

		if arg.asProduct {
			if err := publishAPIToHimarket(arg.typ, arg.name, arg.HimarketAdminAuthArg); err != nil {
				fmt.Println("failed to publish it to himarket, please do it mannually")
				return err
			}
			fmt.Printf("Agent %s is published to Himarket successfully\n", arg.name)
		}
		// TODO: pop up higress window
	}

	return nil
}

func publishAgentAPIToHigress(arg AgentAddArg) error {
	client := services.NewHigressClient(arg.hgURL, arg.hgUser, arg.hgPassword)

	switch arg.typ {
	case A2A:
		if err := services.PublishA2A(client, arg.name, arg.url, arg.externalBaseURL, arg.a2aPluginURL); err != nil {
			return err
		}
	case MODEL:
		// add ai service
		body := services.BuildAIProviderServiceBody(arg.name, arg.url)
		// Debug
		// fmt.Printf("services: body: %v\n", body)
		if resp, err := services.HandleAddAIProviderService(client, body); err != nil {
			fmt.Println(string(resp))
			return err
		}

		// add ai route
		body = services.BuildAddAIRouteBody(arg.name, arg.url)
		// fmt.Printf("Route body: %v\n", body)
		if res, err := services.HandleAddAIRoute(client, body); err != nil {
			fmt.Println(string(res))
			return err
		}

	case REST:
		srvName := fmt.Sprintf("agent-%s", arg.name)
		body, targetSrvName, _, err := services.BuildServiceBodyAndSrv(srvName, arg.url)
		if err != nil {
			return fmt.Errorf("invalid url format: %s", err)
		}

		if resp, err := services.HandleAddServiceSource(client, body); err != nil {
			fmt.Println(string(resp))
			return err
		}

		if resp, err := services.HandleAddRoute(client, services.BuildAPIRouteBody(arg.name, targetSrvName)); err != nil {
			fmt.Println(string(resp))
			return err
		}
	default:
		return fmt.Errorf("unsupported agent protocol type: %s", arg.typ)

	}

	return nil
}

func validateArg(arg AgentAddArg) error {
	if !arg.noPublish {
		return arg.HigressConsoleAuthArg.validate()
	}
	return nil
}
