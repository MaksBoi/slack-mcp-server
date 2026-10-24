package handler

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newEditRequest(args map[string]any) mcp.CallToolRequest {
	req := mcp.CallToolRequest{}
	req.Params.Name = "conversations_edit_message"
	req.Params.Arguments = args
	return req
}

func TestUnitParseParamsToolEditMessage(t *testing.T) {
	ch := newUploadHandler()
	ctx := context.Background()
	valid := func() map[string]any {
		return map[string]any{"channel_id": "C123", "ts": "1700000000.000100", "text": "fixed typo"}
	}

	t.Run("disabled by default", func(t *testing.T) {
		setEnvUpload(t, "SLACK_MCP_EDIT_MESSAGE_TOOL", "")
		setEnvUpload(t, "SLACK_MCP_ENABLED_TOOLS", "")
		_, err := ch.parseParamsToolEditMessage(ctx, newEditRequest(valid()))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "SLACK_MCP_EDIT_MESSAGE_TOOL")
	})

	t.Run("enabled through SLACK_MCP_ENABLED_TOOLS", func(t *testing.T) {
		setEnvUpload(t, "SLACK_MCP_EDIT_MESSAGE_TOOL", "")
		setEnvUpload(t, "SLACK_MCP_ENABLED_TOOLS", "conversations_edit_message")
		_, err := ch.parseParamsToolEditMessage(ctx, newEditRequest(valid()))
		require.NoError(t, err)
	})

	t.Run("channel outside the allowlist", func(t *testing.T) {
		setEnvUpload(t, "SLACK_MCP_EDIT_MESSAGE_TOOL", "C999")
		_, err := ch.parseParamsToolEditMessage(ctx, newEditRequest(valid()))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not allowed")
	})

	t.Run("ts is required", func(t *testing.T) {
		setEnvUpload(t, "SLACK_MCP_EDIT_MESSAGE_TOOL", "true")
		args := valid()
		delete(args, "ts")
		_, err := ch.parseParamsToolEditMessage(ctx, newEditRequest(args))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ts")
	})

	t.Run("ts must be a timestamp", func(t *testing.T) {
		setEnvUpload(t, "SLACK_MCP_EDIT_MESSAGE_TOOL", "true")
		args := valid()
		args["ts"] = "1700000000"
		_, err := ch.parseParamsToolEditMessage(ctx, newEditRequest(args))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "1234567890.123456")
	})

	t.Run("text or blocks is required", func(t *testing.T) {
		setEnvUpload(t, "SLACK_MCP_EDIT_MESSAGE_TOOL", "true")
		args := valid()
		delete(args, "text")
		_, err := ch.parseParamsToolEditMessage(ctx, newEditRequest(args))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "text or blocks")
	})

	t.Run("valid params", func(t *testing.T) {
		setEnvUpload(t, "SLACK_MCP_EDIT_MESSAGE_TOOL", "C123")
		args := valid()
		args["content_type"] = "text/plain"
		args["blocks"] = `[{"type":"divider"}]`
		p, err := ch.parseParamsToolEditMessage(ctx, newEditRequest(args))
		require.NoError(t, err)
		assert.Equal(t, "C123", p.channel)
		assert.Equal(t, "1700000000.000100", p.ts)
		assert.Equal(t, "fixed typo", p.text)
		assert.Equal(t, "text/plain", p.contentType)
		require.Len(t, p.blocks, 1)
	})
}
