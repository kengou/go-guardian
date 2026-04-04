package admin

import (
	"context"
	"errors"
	"testing"

	"github.com/kengou/go-guardian/mcp-server/db"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func newTestStore(t *testing.T) *db.Store {
	t.Helper()
	store, err := db.NewStore(":memory:")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestWrapToolHandler(t *testing.T) {
	tests := []struct {
		name       string
		toolName   string
		agent      string
		handler    server.ToolHandlerFunc
		wantText   string
		wantErr    bool
		wantErrMsg string
	}{
		{
			name:     "successful handler is logged",
			toolName: "query_knowledge",
			agent:    "reviewer",
			handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return mcp.NewToolResultText("ok"), nil
			},
			wantText: "ok",
			wantErr:  false,
		},
		{
			name:     "handler error is logged",
			toolName: "check_owasp",
			agent:    "security",
			handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return nil, errors.New("scan failed")
			},
			wantErr:    true,
			wantErrMsg: "scan failed",
		},
		{
			name:     "no agent in params",
			toolName: "learn_from_lint",
			agent:    "",
			handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return mcp.NewToolResultText("learned"), nil
			},
			wantText: "learned",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newTestStore(t)
			wrapped := WrapToolHandler(store, tt.toolName, tt.handler)

			args := map[string]interface{}{
				"file": "main.go",
			}
			if tt.agent != "" {
				args["agent"] = tt.agent
			}

			req := mcp.CallToolRequest{}
			req.Params.Name = tt.toolName
			req.Params.Arguments = args

			result, err := wrapped(context.Background(), req)

			// Verify original result and error are returned.
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if err.Error() != tt.wantErrMsg {
					t.Errorf("error = %q, want %q", err.Error(), tt.wantErrMsg)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				// Check the result text content.
				if result == nil {
					t.Fatal("expected non-nil result")
				}
			}

			// Verify the request was logged.
			reqs, getErr := store.GetMCPRequests("", "", 100, 0)
			if getErr != nil {
				t.Fatalf("GetMCPRequests: %v", getErr)
			}
			if len(reqs) != 1 {
				t.Fatalf("expected 1 logged request, got %d", len(reqs))
			}

			logged := reqs[0]
			if logged.ToolName != tt.toolName {
				t.Errorf("logged tool_name = %q, want %q", logged.ToolName, tt.toolName)
			}
			if logged.Agent != tt.agent {
				t.Errorf("logged agent = %q, want %q", logged.Agent, tt.agent)
			}
			if logged.DurationMS < 0 {
				t.Errorf("logged duration_ms = %d, want >= 0", logged.DurationMS)
			}
			if tt.wantErr && logged.Error == "" {
				t.Error("expected non-empty error in logged request")
			}
			if !tt.wantErr && logged.Error != "" {
				t.Errorf("expected empty error in logged request, got %q", logged.Error)
			}
		})
	}
}

func TestTruncateParams(t *testing.T) {
	tests := []struct {
		name   string
		args   map[string]interface{}
		maxLen int
		want   string
	}{
		{
			name:   "nil map",
			args:   nil,
			maxLen: 200,
			want:   "{}",
		},
		{
			name:   "empty map",
			args:   map[string]interface{}{},
			maxLen: 200,
			want:   "{}",
		},
		{
			name:   "short params not truncated",
			args:   map[string]interface{}{"key": "val"},
			maxLen: 200,
			want:   `{"key":"val"}`,
		},
		{
			name:   "long params truncated",
			args:   map[string]interface{}{"data": "abcdefghijklmnopqrstuvwxyz"},
			maxLen: 15,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateParams(tt.args, tt.maxLen)
			if tt.want != "" && got != tt.want {
				t.Errorf("truncateParams = %q, want %q", got, tt.want)
			}
			if len(got) > tt.maxLen {
				t.Errorf("truncateParams length = %d, want <= %d", len(got), tt.maxLen)
			}
		})
	}
}
