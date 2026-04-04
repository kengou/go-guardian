package tools

import (
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// ToolRegistrar abstracts MCP tool registration, allowing callers to wrap
// handlers (e.g., for request logging) without modifying individual Register
// functions. *server.MCPServer satisfies this interface.
type ToolRegistrar interface {
	AddTool(tool mcp.Tool, handler server.ToolHandlerFunc)
}
