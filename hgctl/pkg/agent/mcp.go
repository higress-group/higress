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
	"net/url"
	"os"

	"github.com/alibaba/higress/hgctl/pkg/agent/services"
	"github.com/alibaba/higress/hgctl/pkg/helm"
	"github.com/fatih/color"
	"github.com/higress-group/openapi-to-mcpserver/pkg/models"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

type MCPType string

const (
	OPENAPI string = "openapi"
	HTTP    string = "http"

	STREAMABLE string = "streamable"
	SSE        string = "sse"

	DIRECT_ROUTE string = "DIRECT_ROUTE"
	OPEN_API     string = "OPEN_API"
)

type MCPAddArg struct {
	HigressConsoleAuthArg
	HimarketAdminAuthArg

	name      string
	url       string
	typ       string
	transport string
	spec      string
	scope     string
	env       []string
	header    []string
	noPublish bool
	asProduct bool
}

type MCPAddHandler struct {
	core *AgenticCore
	arg  MCPAddArg
	w    io.Writer
}

func NewMCPCmd() *cobra.Command {
	mcpCmd := &cobra.Command{
		Use:   "mcp",
		Short: "for the mcp management",
	}

	mcpCmd.AddCommand(newMCPAddCmd())

	return mcpCmd
}

func newMCPAddCmd() *cobra.Command {
	// parameter
	arg := &MCPAddArg{}

	cmd := &cobra.Command{
		Use:   "add [name] [url]",
		Short: "add mcp server including http and openapi",
		Example: `  # Add HTTP type MCP Server
  hgctl mcp add http-mcp http://localhost:8080/mcp

  # Add MCP Server with environment variables and headers
  hgctl mcp add http-mcp http://localhost:8080/mcp -e API_KEY=secret -H "Authorization: Bearer token"

  # Add MCP Server use Openapi file
  hgctl mcp add swagger-mcp ./path/to/openapi.yaml --type openapi`,
		Run: func(cmd *cobra.Command, args []string) {

			arg.name = args[0]
			if arg.typ == HTTP {
				arg.url = args[1]
			} else {
				arg.spec = args[1]
			}

			resolveHigressConsoleAuth(&arg.HigressConsoleAuthArg)
			resolveHimarketAdminAuth(&arg.HimarketAdminAuthArg)
			cmdutil.CheckErr(handleAddMCP(cmd.OutOrStdout(), *arg))
			color.Cyan("Tip: Try doing 'kubectl port-forward' and add the server to the agent manually, if using Higress MCP Server and connection failed")
		},
		Args: cobra.ExactArgs(2),
	}

	cmd.PersistentFlags().StringVar(&arg.typ, "type", HTTP, "Determine the MCP Server's Type")
	cmd.PersistentFlags().StringVarP(&arg.transport, "transport", "t", STREAMABLE, `The MCP Server's transport`)
	cmd.PersistentFlags().StringVarP(&arg.scope, "scope", "s", "project", `Configuration scope (project or global)`)
	cmd.PersistentFlags().StringSliceVarP(&arg.env, "env", "e", nil, "Environment variables to pass to the MCP server (can be specified multiple times)")
	cmd.PersistentFlags().StringSliceVarP(&arg.header, "header", "H", nil, "HTTP headers to pass to the MCP server (can be specified multiple times)")
	cmd.PersistentFlags().BoolVar(&arg.noPublish, "no-publish", false, "If set then the mcp server will not be plubished to higress")
	cmd.PersistentFlags().BoolVar(&arg.asProduct, "as-product", false, "If it's set then the agent API will be published to Himarket (no-publish must be false)")

	// cmd.PersistentFlags().StringVar(&arg.spec, "spec", "", "Specification file (yaml/json) of the openapi api")

	addHigressConsoleAuthFlag(cmd, &arg.HigressConsoleAuthArg)
	addHimarketAdminAuthFlag(cmd, &arg.HimarketAdminAuthArg)

	return cmd
}

func newHanlder(c *AgenticCore, arg MCPAddArg, w io.Writer) *MCPAddHandler {
	return &MCPAddHandler{c, arg, w}
}

func (h *MCPAddHandler) validateArg() error {
	if !h.arg.noPublish {
		return h.arg.HigressConsoleAuthArg.validate()
	}
	return nil

}

func (h *MCPAddHandler) addHTTPMCP() error {
	if err := h.core.AddMCPServer(h.arg); err != nil {
		return fmt.Errorf("mcp add failed: %w", err)
	}

	if !h.arg.noPublish {
		return publishMCPToHigress(h.arg, h.arg.typ, nil)
	}
	return nil

}

// hgctl mcp add -t openapi --name test-name --spec openapi.json
func (h *MCPAddHandler) addOpenAPIMCP() error {
	// fmt.Printf("get mcp server: %s openapi-spec-file: %s\n", h.arg.name, h.arg.spec)
	config := h.parseOpenapiSpec()
	// fmt.Printf("get config struct: %v", config)

	// publish to higress
	if err := publishMCPToHigress(h.arg, "streamable", config); err != nil {
		return err
	}

	// add mcp server to agent
	gatewayURL := viper.GetString(HIGRESS_GATEWAY_URL)
	if gatewayURL == "" {
		svcIP, err := GetHigressGatewayServiceIP()
		if err != nil {
			color.Red(
				"failed to add mcp server [%s] while getting higress-gateway ip due to: %v \n You may try to do port-forward and add it to agent manually", h.arg.name, err)
			return err
		}
		gatewayURL = svcIP
	}

	mcpURL := fmt.Sprintf("%s/mcp-servers/%s", gatewayURL, h.arg.name)
	h.arg.url = mcpURL
	return h.core.AddMCPServer(h.arg)
}

func (h *MCPAddHandler) parseOpenapiSpec() *models.MCPConfig {
	config := parseOpenapi2MCP(h.arg)
	prepareOpenAPIMCPConfig(config)
	return config
}

// prepareOpenAPIMCPConfig is applied to converted OpenAPI MCP config before
// publish. It must not inject DefaultCredential; a required-but-missing
// credential stays empty so request auth returns a configuration error.
func prepareOpenAPIMCPConfig(config *models.MCPConfig) {
	if config == nil {
		return
	}
}

func handleAddMCP(w io.Writer, arg MCPAddArg) error {
	client, err := getCore()
	if err != nil {
		return fmt.Errorf("failed to get agent core: %s", err)
	}
	h := newHanlder(client, arg, w)
	if err := h.validateArg(); err != nil {
		return err
	}

	// spec -> OPENAPI
	// noPublish -> typ
	switch arg.typ {
	case HTTP:
		if err := h.addHTTPMCP(); err != nil {
			return err
		}

	case OPENAPI:
		if arg.spec == "" {
			return fmt.Errorf("--spec is required for openapi type")
		}
		if arg.noPublish {
			return fmt.Errorf("--no-publish is not supported for openapi type")
		}
		if arg.url != "" {
			return fmt.Errorf("--url is not supported for openapi type")
		}
		if err := h.addOpenAPIMCP(); err != nil {
			return err
		}

	}

	if !arg.noPublish && arg.asProduct {
		if err := publishAPIToHimarket("mcp", arg.name, arg.HimarketAdminAuthArg); err != nil {
			fmt.Println("failed to publish it to himarket, please do it mannually")
			return err
		}
	}

	return nil

}

func publishMCPToHigress(arg MCPAddArg, transport string, config *models.MCPConfig) error {
	// 1. parse the raw http url
	// 2. add service source
	// 3. add MCP server request
	client := services.NewHigressClient(arg.hgURL, arg.hgUser, arg.hgPassword)

	rawURL := arg.url
	// DIRECT_ROUTE or OPEN_API
	mcpType := DIRECT_ROUTE

	if config != nil {
		// TODO: here use tools's url directly, need to be considered
		rawURL = config.Tools[0].RequestTemplate.URL
		mcpType = OPEN_API
	}

	srvName := fmt.Sprintf("hgctl-%s", arg.name)

	// e.g. hgctl-mcp-deepwiki.dns
	body, targetSrvName, port, err := services.BuildServiceBodyAndSrv(srvName, rawURL)
	if err != nil {
		return fmt.Errorf("invalid url format: %s", err)
	}

	resp, err := services.HandleAddServiceSource(client, body)
	if err != nil {
		return fmt.Errorf("response body: %s %s\n", string(resp), err)
	}

	srvField := []map[string]interface{}{{
		"name":    targetSrvName,
		"port":    port,
		"version": "1.0",
		"weight":  100,
	}}

	body = map[string]interface{}{
		"name":        arg.name,
		"description": "A MCP Server added by hgctl",
		"type":        mcpType,
		"services":    srvField,
		"domains":     []interface{}{},
		"consumerAuthInfo": map[string]interface{}{
			"type":             "key-auth",
			"allowedConsumers": []string{},
		},
	}

	// Only DIRECT_ROUTE Type get below extra params
	if mcpType == DIRECT_ROUTE {
		res, _ := url.Parse(rawURL)
		body["directRouteConfig"] = map[string]interface{}{
			"path":          res.Path,
			"transportType": arg.transport,
		}
	}

	_, err = services.HandleAddMCPServer(client, body)
	if err != nil {
		return err
	}

	if mcpType == OPEN_API {
		addMCPToolConfig(client, config, srvField)
	}

	return nil
}

func addMCPToolConfig(client *services.HigressClient, config *models.MCPConfig, srvField []map[string]interface{}) {
	body := map[string]interface{}{
		"name":              config.Server.Name,
		"description":       "A MCP Server added by hgctl",
		"services":          srvField,
		"type":              OPEN_API,
		"rawConfigurations": convertMCPConfigToStr(config),
		"mcpServerName":     config.Server.Name,
		"domains":           []interface{}{},
		"consumerAuthInfo": map[string]interface{}{
			"type":             "key-auth",
			"allowedConsumers": []string{},
		},
	}

	_, err := services.HandleAddOpenAPITool(client, body)
	if err != nil {
		fmt.Printf("add openapi tools failed: %v\n", err)
		os.Exit(1)
	}
	// fmt.Println("get openapi tools add response: ", string(resp))
}

func tryToGetLocalCredential(arg *HigressConsoleAuthArg) error {
	profileContexts, err := getAllProfiles()

	// The higress is not installed by hgctl
	if err != nil || len(profileContexts) == 0 {
		return err
	}

	for _, ctx := range profileContexts {
		installTyp := ctx.Install
		if installTyp == helm.InstallK8s || installTyp == helm.InstallLocalK8s {
			user, pwd, err := getConsoleCredentials(ctx.Profile)
			if err != nil {
				continue
			}
			// TODO: always use the first one profile
			arg.hgUser = user
			arg.hgPassword = pwd
			return nil
		}
	}

	return nil
}
