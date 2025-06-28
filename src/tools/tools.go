package tools

import (
	"github.com/mark3labs/mcp-go/server"
)

// RegisterTools registers all the tools with the MCP server.
// It takes an MCP server instance and a Kubernetes client as parameters.
// Each tool is created and added to the server with its corresponding handler.
// This allows the server to handle requests for each tool defined in the tools package.
func RegisterTools(s *server.MCPServer, client Client) {
	tools := []Tools{
		NewListTool(client),
		NewLogTool(client),
		NewDescribeTool(client),
		NewRolloutTool(client),          // Register the new rollout tool
		NewChangeEnvTool(),              // Register the new change_env tool
		NewListGCPSecretTool(),          // Register the new list_gcp_secret tool
		NewListIngressPathsTool(client), // Register the new list ingress paths tool
	}
	for _, t := range tools {
		s.AddTool(t.Tool(), t.Handler)
	}
}
