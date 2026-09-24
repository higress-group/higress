package gorm

import (
	"errors"
	"fmt"

	"github.com/alibaba/higress/plugins/golang-filter/mcp-session/common"
	"github.com/envoyproxy/envoy/contrib/golang/common/go/api"
	"github.com/mark3labs/mcp-go/mcp"
)

const Version = "1.0.0"

func init() {
	common.GlobalRegistry.RegisterServer("database", &DBConfig{})
}

type DBConfig struct {
	dbType      string
	dsn         string
	description string
}

func (c *DBConfig) Clone() common.Server {
	cloned := *c
	return &cloned
}

func (c *DBConfig) ParseConfig(config map[string]any) error {
	dsn, ok := config["dsn"].(string)
	if !ok {
		return errors.New("missing dsn")
	}
	c.dsn = dsn

	dbType, ok := config["dbType"].(string)
	if !ok {
		return errors.New("missing database type")
	}
	c.dbType = dbType
	api.LogDebugf("DBConfig ParseConfig: %+v", config)
	c.description, ok = config["description"].(string)
	if !ok {
		c.description = ""
	}
	return nil
}

func (c *DBConfig) NewServer(serverName string) (*common.MCPServer, error) {
	mcpServer := common.NewMCPServer(
		serverName,
		Version,
		common.WithInstructions(fmt.Sprintf("This is a %s database server", c.dbType)),
	)

	dbClient := NewDBClient(c.dsn, c.dbType, mcpServer.GetDestoryChannel())
	descriptionSuffix := fmt.Sprintf("in database %s. Database description: %s", c.dbType, c.description)
	// Add query tool
	mcpServer.AddTool(
		mcp.NewToolWithRawSchema("query", fmt.Sprintf("Run a read-only SQL query %s", descriptionSuffix), GetQueryToolSchema()),
		HandleQueryTool(dbClient),
	)
	mcpServer.AddTool(
		mcp.NewToolWithRawSchema("execute", fmt.Sprintf("Execute an insert, update, or delete SQL %s", descriptionSuffix), GetExecuteToolSchema()),
		HandleExecuteTool(dbClient),
	)
	mcpServer.AddTool(
		mcp.NewToolWithRawSchema("list tables", fmt.Sprintf("List all tables %s", descriptionSuffix), GetListTablesToolSchema()),
		HandleListTablesTool(dbClient),
	)
	mcpServer.AddTool(
		mcp.NewToolWithRawSchema("describe table", fmt.Sprintf("Get the structure of a specific table %s", descriptionSuffix), GetDescribeTableToolSchema()),
		HandleDescribeTableTool(dbClient),
	)

	return mcpServer, nil
}
