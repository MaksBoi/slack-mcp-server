package handler

import (
	"testing"

	"github.com/slack-go/slack"
	"github.com/stretchr/testify/assert"
)

func TestUnitAttachmentFields(t *testing.T) {
	t.Run("no files", func(t *testing.T) {
		ids, types := attachmentFields(nil)
		assert.Empty(t, ids)
		assert.Empty(t, types)
	})

	t.Run("types follow ids order", func(t *testing.T) {
		ids, types := attachmentFields([]slack.File{
			{ID: "F1", Name: "voice.webm", Mimetype: "audio/webm"},
			{ID: "F2", Name: "report.pdf", Mimetype: "application/pdf"},
		})
		assert.Equal(t, "F1 (voice.webm), F2 (report.pdf)", ids)
		assert.Equal(t, "audio/webm, application/pdf", types)
	})

	t.Run("file without name and mime", func(t *testing.T) {
		ids, types := attachmentFields([]slack.File{{ID: "F3"}})
		assert.Equal(t, "F3", ids)
		assert.Equal(t, "unknown", types)
	})
}
