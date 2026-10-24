package handler

import (
	"net/url"
	"testing"

	"github.com/slack-go/slack"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// applyContentOptions renders message options into the form values Slack would receive.
func applyContentOptions(t *testing.T, opts []slack.MsgOption) url.Values {
	t.Helper()
	_, values, err := slack.UnsafeApplyMsgOptions("", "C1", "https://slack.com/api/", opts...)
	require.NoError(t, err)
	return values
}

func TestUnitMessageContentOptions(t *testing.T) {
	ch := newUploadHandler()
	rawBlocks := []slack.Block{slack.NewDividerBlock()}

	t.Run("add plain text sends text without blocks", func(t *testing.T) {
		opts, err := ch.messageContentOptions("hello", "text/plain", nil, false)
		require.NoError(t, err)
		v := applyContentOptions(t, opts)
		assert.Equal(t, "hello", v.Get("text"))
		assert.Equal(t, "false", v.Get("mrkdwn"))
		assert.False(t, v.Has("blocks"))
	})

	t.Run("add markdown sends blocks only", func(t *testing.T) {
		opts, err := ch.messageContentOptions("*hello*", "text/markdown", nil, false)
		require.NoError(t, err)
		v := applyContentOptions(t, opts)
		assert.NotEmpty(t, v.Get("blocks"))
		assert.NotEqual(t, "[]", v.Get("blocks"))
		assert.False(t, v.Has("text"))
	})

	t.Run("raw blocks keep text as fallback", func(t *testing.T) {
		opts, err := ch.messageContentOptions("fallback", "text/markdown", rawBlocks, false)
		require.NoError(t, err)
		v := applyContentOptions(t, opts)
		assert.Contains(t, v.Get("blocks"), "divider")
		assert.Equal(t, "fallback", v.Get("text"))
	})

	t.Run("edit plain text clears old blocks", func(t *testing.T) {
		opts, err := ch.messageContentOptions("hello", "text/plain", nil, true)
		require.NoError(t, err)
		v := applyContentOptions(t, opts)
		assert.Equal(t, "hello", v.Get("text"))
		assert.Equal(t, "[]", v.Get("blocks"))
	})

	t.Run("edit markdown refreshes fallback text", func(t *testing.T) {
		opts, err := ch.messageContentOptions("*hello*", "text/markdown", nil, true)
		require.NoError(t, err)
		v := applyContentOptions(t, opts)
		assert.NotEqual(t, "[]", v.Get("blocks"))
		assert.NotEmpty(t, v.Get("blocks"))
		assert.Equal(t, "*hello*", v.Get("text"))
	})

	t.Run("unknown content type is rejected", func(t *testing.T) {
		_, err := ch.messageContentOptions("hello", "text/html", nil, false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "content_type")
	})
}
