package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark3labs/mcp-go/mcp"
)

func overlappingTemplates() []ServerResourceTemplate {
	handlerFor := func(text string) ResourceTemplateHandlerFunc {
		return func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			return []mcp.ResourceContents{mcp.TextResourceContents{URI: request.Params.URI, Text: text}}, nil
		}
	}
	return []ServerResourceTemplate{
		{Template: mcp.NewResourceTemplate("db://{table}/{id}", "row"), Handler: handlerFor("row")},
		{Template: mcp.NewResourceTemplate("db://users/{id}", "user"), Handler: handlerFor("user")},
	}
}

func requireReadsHandledBy(t *testing.T, ctx context.Context, server *MCPServer, uri, want string) {
	t.Helper()
	for range 50 {
		response := server.HandleMessage(ctx, []byte(`{
			"jsonrpc": "2.0",
			"id": 1,
			"method": "resources/read",
			"params": {"uri": "`+uri+`"}
		}`))

		resp, ok := response.(mcp.JSONRPCResponse)
		require.True(t, ok, "expected a JSONRPCResponse, got %T", response)
		result, ok := resp.Result.(mcp.ReadResourceResult)
		require.True(t, ok, "expected a ReadResourceResult, got %T", resp.Result)
		require.Len(t, result.Contents, 1)
		text, ok := result.Contents[0].(mcp.TextResourceContents)
		require.True(t, ok)
		require.Equal(t, want, text.Text, "overlapping templates must resolve the same way on every read")
	}
}

func TestReadResource_OverlappingTemplatesMatchDeterministically(t *testing.T) {
	t.Run("global templates", func(t *testing.T) {
		server := NewMCPServer("test-server", "1.0.0", WithResourceCapabilities(false, false))
		server.AddResourceTemplates(overlappingTemplates()...)

		requireReadsHandledBy(t, context.Background(), server, "db://users/42", "user")
		requireReadsHandledBy(t, context.Background(), server, "db://orders/42", "row")
	})

	t.Run("session templates", func(t *testing.T) {
		server := NewMCPServer("test-server", "1.0.0", WithResourceCapabilities(false, false))
		session := &sessionTestClientWithResourceTemplates{
			sessionID:                "session-1",
			notificationChannel:      make(chan mcp.JSONRPCNotification, 10),
			sessionResourceTemplates: map[string]ServerResourceTemplate{},
		}
		for _, template := range overlappingTemplates() {
			session.sessionResourceTemplates[template.Template.URITemplate.Raw()] = template
		}
		session.initialized.Store(true)
		require.NoError(t, server.RegisterSession(t.Context(), session))
		ctx := server.WithContext(t.Context(), session)

		requireReadsHandledBy(t, ctx, server, "db://users/42", "user")
		requireReadsHandledBy(t, ctx, server, "db://orders/42", "row")
	})
}
