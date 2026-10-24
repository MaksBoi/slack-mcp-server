package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/slack-go/slack"
	"go.uber.org/zap"
)

// maxTranscribeBytes caps attachment_transcribe downloads. stt-gateway uses the same limit.
const maxTranscribeBytes = 100 << 20

// transcription is the answer of an OpenAI-compatible transcription endpoint (stt-gateway).
type transcription struct {
	Text     string  `json:"text"`
	Language string  `json:"language"`
	Duration float64 `json:"duration"`
	Provider string  `json:"provider"`
}

// transcriptionResult is what attachment_transcribe returns to the agent.
type transcriptionResult struct {
	FileID   string `json:"file_id"`
	Filename string `json:"filename"`
	transcription
}

// transcribeTimeout covers stt-gateway's whole provider chain, which gives up after 280 s.
const transcribeTimeout = 300 * time.Second

type transcribeParams struct {
	endpoint string
	fileID   string
	language string
}

// AttachmentTranscribeHandler downloads an audio or video attachment and transcribes it through
// the OpenAI-compatible endpoint in SLACK_MCP_TRANSCRIBE_URL (stt-gateway on this machine).
// Request params are not logged: the file is private and the answer is its content.
func (ch *ConversationsHandler) AttachmentTranscribeHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	params, err := ch.parseParamsToolTranscribe(request)
	if err != nil {
		return nil, err
	}

	if ready, err := ch.apiProvider.IsReady(); !ready {
		ch.logger.Error("API provider not ready", zap.Error(err))
		return nil, err
	}

	fileInfo, _, _, err := ch.apiProvider.Slack().GetFileInfoContext(ctx, params.fileID, 0, 0)
	if err != nil {
		ch.logger.Error("Slack GetFileInfoContext failed", zap.Error(err))
		return nil, err
	}
	if err := checkTranscribable(fileInfo); err != nil {
		return nil, err
	}

	tmp, err := os.CreateTemp("", "slack-transcribe-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp file: %w", err)
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	if err := ch.downloadSlackFile(ctx, fileInfo, tmp); err != nil {
		ch.logger.Error("Slack GetFileContext failed", zap.Error(err))
		return nil, err
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}

	filename := fileInfo.Name
	if filename == "" {
		filename = fileInfo.ID
	}
	got, err := transcribeAudio(ctx, &http.Client{Timeout: transcribeTimeout}, params.endpoint, filename, tmp, params.language)
	if err != nil {
		ch.logger.Error("Transcription failed", zap.String("file_id", fileInfo.ID), zap.Error(err))
		return nil, err
	}

	out, err := json.Marshal(transcriptionResult{FileID: fileInfo.ID, Filename: fileInfo.Name, transcription: *got})
	if err != nil {
		return nil, err
	}
	return mcp.NewToolResultText(string(out)), nil
}

func (ch *ConversationsHandler) parseParamsToolTranscribe(request mcp.CallToolRequest) (*transcribeParams, error) {
	endpoint := os.Getenv("SLACK_MCP_TRANSCRIBE_URL")
	if endpoint == "" {
		return nil, errors.New("attachment_transcribe needs SLACK_MCP_TRANSCRIBE_URL, the full URL of an OpenAI-compatible transcription endpoint, e.g. http://127.0.0.1:8790/v1/audio/transcriptions")
	}
	fileID := request.GetString("file_id", "")
	if fileID == "" {
		return nil, errors.New("file_id is required")
	}
	return &transcribeParams{
		endpoint: endpoint,
		fileID:   fileID,
		language: request.GetString("language", ""),
	}, nil
}

// checkTranscribable rejects files that are not audio or video, or too big to download.
func checkTranscribable(f *slack.File) error {
	if !strings.HasPrefix(f.Mimetype, "audio/") && !strings.HasPrefix(f.Mimetype, "video/") {
		return fmt.Errorf("file %s is %q, only audio and video can be transcribed", f.ID, f.Mimetype)
	}
	if f.Size > maxTranscribeBytes {
		return fmt.Errorf("file size %d bytes exceeds maximum allowed size of %d bytes for transcription", f.Size, maxTranscribeBytes)
	}
	return nil
}

// downloadSlackFile writes a Slack file to w, preferring the download URL.
func (ch *ConversationsHandler) downloadSlackFile(ctx context.Context, f *slack.File, w io.Writer) error {
	downloadURL := f.URLPrivateDownload
	if downloadURL == "" {
		downloadURL = f.URLPrivate
	}
	if downloadURL == "" {
		return errors.New("file has no downloadable URL")
	}
	return ch.apiProvider.Slack().GetFileContext(ctx, downloadURL, w)
}

// transcribeAudio streams one file to an OpenAI-compatible transcription endpoint.
// The body is produced through io.Pipe, so a large recording is never held in memory.
func transcribeAudio(ctx context.Context, client *http.Client, endpoint, filename string, file io.Reader, language string) (*transcription, error) {
	pr, pw := io.Pipe()
	form := multipart.NewWriter(pw)
	go func() {
		part, err := form.CreateFormFile("file", filename)
		if err == nil {
			_, err = io.Copy(part, file)
		}
		if err == nil && language != "" {
			err = form.WriteField("language", language)
		}
		if err == nil {
			err = form.Close()
		}
		pw.CloseWithError(err)
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, pr)
	if err != nil {
		pr.CloseWithError(err)
		return nil, err
	}
	req.Header.Set("Content-Type", form.FormDataContentType())

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("transcription request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("reading transcription response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		var failure struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(body, &failure) == nil && failure.Error != "" {
			return nil, fmt.Errorf("transcription endpoint returned HTTP %d: %s", resp.StatusCode, failure.Error)
		}
		return nil, fmt.Errorf("transcription endpoint returned HTTP %d", resp.StatusCode)
	}

	var out transcription
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("transcription endpoint returned invalid JSON: %w", err)
	}
	return &out, nil
}
