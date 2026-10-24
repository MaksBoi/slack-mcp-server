package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/slack-go/slack"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeGateway records the multipart request and answers with the given status and body.
type fakeGateway struct {
	file     string
	filename string
	language string
	hasLang  bool
}

func (g *fakeGateway) server(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		reader, err := r.MultipartReader()
		require.NoError(t, err)
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
			data, err := io.ReadAll(part)
			require.NoError(t, err)
			switch part.FormName() {
			case "file":
				g.file = string(data)
				g.filename = part.FileName()
			case "language":
				g.language = string(data)
				g.hasLang = true
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestUnitTranscribeAudio(t *testing.T) {
	ctx := context.Background()

	t.Run("streams the file and parses the answer", func(t *testing.T) {
		g := &fakeGateway{}
		srv := g.server(t, http.StatusOK, `{"text":"привет","language":"ru","duration":2.5,"provider":"winwispr"}`)
		got, err := transcribeAudio(ctx, srv.Client(), srv.URL, "voice.webm", strings.NewReader("AUDIO"), "ru")
		require.NoError(t, err)
		assert.Equal(t, "AUDIO", g.file)
		assert.Equal(t, "voice.webm", g.filename)
		assert.Equal(t, "ru", g.language)
		assert.Equal(t, &transcription{Text: "привет", Language: "ru", Duration: 2.5, Provider: "winwispr"}, got)
	})

	t.Run("omits an empty language", func(t *testing.T) {
		g := &fakeGateway{}
		srv := g.server(t, http.StatusOK, `{"text":""}`)
		_, err := transcribeAudio(ctx, srv.Client(), srv.URL, "voice.webm", strings.NewReader("AUDIO"), "")
		require.NoError(t, err)
		assert.False(t, g.hasLang)
	})

	t.Run("reports the gateway error", func(t *testing.T) {
		g := &fakeGateway{}
		srv := g.server(t, http.StatusBadGateway, `{"error":"all providers failed: winwispr: not ready"}`)
		_, err := transcribeAudio(ctx, srv.Client(), srv.URL, "voice.webm", strings.NewReader("AUDIO"), "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "502")
		assert.Contains(t, err.Error(), "all providers failed")
	})
}

func TestUnitCheckTranscribable(t *testing.T) {
	assert.NoError(t, checkTranscribable(&slack.File{Mimetype: "audio/webm", Size: 1000}))
	assert.NoError(t, checkTranscribable(&slack.File{Mimetype: "video/mp4", Size: maxTranscribeBytes}))

	err := checkTranscribable(&slack.File{Mimetype: "application/pdf", Size: 1000})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "application/pdf")

	err = checkTranscribable(&slack.File{Mimetype: "audio/mp4", Size: maxTranscribeBytes + 1})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds")
}

func TestUnitTranscriptionJSONShape(t *testing.T) {
	// The handler result keeps the field names agents read.
	out, err := json.Marshal(transcriptionResult{FileID: "F1", Filename: "v.webm", transcription: transcription{Text: "t", Provider: "openai"}})
	require.NoError(t, err)
	assert.JSONEq(t, `{"file_id":"F1","filename":"v.webm","text":"t","language":"","duration":0,"provider":"openai"}`, string(out))
}
