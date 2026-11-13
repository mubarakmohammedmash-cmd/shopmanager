package main

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPrepareAssistantAttachments(t *testing.T) {
	tests := []struct {
		name        string
		attachment  assistantAttachment
		wantMIME    string
		wantErrText string
	}{
		{
			name:       "image",
			attachment: assistantAttachment{Name: "receipt.png", Data: base64.StdEncoding.EncodeToString([]byte("image"))},
			wantMIME:   "image/png",
		},
		{
			name:       "PDF",
			attachment: assistantAttachment{Name: "report.pdf", Data: base64.StdEncoding.EncodeToString([]byte("pdf"))},
			wantMIME:   "application/pdf",
		},
		{
			name:        "unsupported type",
			attachment:  assistantAttachment{Name: "program.exe", Data: base64.StdEncoding.EncodeToString([]byte("data"))},
			wantErrText: "unsupported attachment type",
		},
		{
			name:        "invalid base64",
			attachment:  assistantAttachment{Name: "notes.txt", Data: "not-base64!"},
			wantErrText: "could not be decoded",
		},
		{
			name:        "oversized",
			attachment:  assistantAttachment{Name: "large.txt", Data: base64.StdEncoding.EncodeToString(make([]byte, maxAssistantAttachmentSize+1))},
			wantErrText: "4 MB or smaller",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			attachments, err := prepareAssistantAttachments([]assistantAttachment{test.attachment})
			if test.wantErrText != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErrText) {
					t.Fatalf("prepareAssistantAttachments() error = %v, want containing %q", err, test.wantErrText)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(attachments) != 1 || attachments[0].MIMEType != test.wantMIME {
				t.Fatalf("prepared attachment = %+v, want MIME %q", attachments, test.wantMIME)
			}
		})
	}
}

func TestAssistantResponsesInputIncludesFiles(t *testing.T) {
	messages := []assistantMessage{
		{Role: "user", Content: "What is in this image and PDF?"},
	}
	attachments := []preparedAssistantAttachment{
		{Name: "photo.jpg", MIMEType: "image/jpeg", Data: []byte("photo")},
		{Name: "report.pdf", MIMEType: "application/pdf", Data: []byte("document")},
	}
	input := assistantResponsesInput(messages, attachments)
	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, expected := range []string{
		`"type":"input_image"`,
		`"type":"input_file"`,
		`"filename":"report.pdf"`,
		"data:image/jpeg;base64,",
		"data:application/pdf;base64,",
	} {
		if !strings.Contains(text, expected) {
			t.Errorf("Responses input is missing %q: %s", expected, text)
		}
	}
}

func TestRequestOpenAITranscription(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("transcription method = %s, want POST", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("transcription authorization = %q", r.Header.Get("Authorization"))
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("parse transcription form: %v", err)
			http.Error(w, "bad multipart form", http.StatusBadRequest)
			return
		}
		if model := r.FormValue("model"); model != "gpt-transcribe" {
			t.Errorf("transcription model = %q, want gpt-transcribe", model)
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Errorf("transcription file missing: %v", err)
			http.Error(w, "missing file", http.StatusBadRequest)
			return
		}
		defer file.Close()
		if header.Filename != "voice.webm" {
			t.Errorf("transcription filename = %q, want voice.webm", header.Filename)
		}
		data, err := io.ReadAll(file)
		if err != nil || string(data) != "audio bytes" {
			t.Errorf("transcription file data = %q, error = %v", data, err)
		}
		_, _ = w.Write([]byte(`{"text":"hello ShopManager"}`))
	}))
	defer server.Close()

	originalURL := assistantTranscriptionAPIURL
	assistantTranscriptionAPIURL = server.URL
	t.Cleanup(func() { assistantTranscriptionAPIURL = originalURL })

	transcription, err := requestOpenAITranscription(
		t.Context(), "test-key", "voice.webm", []byte("audio bytes"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if transcription.Text != "hello ShopManager" {
		t.Fatalf("transcription text = %q, want hello ShopManager", transcription.Text)
	}
}

func TestAssistantTranscribeRejectsUnsupportedAudio(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	var body strings.Builder
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("audio", "script.exe")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("not audio"))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/assistant/transcribe", strings.NewReader(body.String()))
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()

	assistantTranscribe(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("unsupported audio status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestRequestOpenAISpeech(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("speech method = %s, want POST", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("speech authorization = %q", r.Header.Get("Authorization"))
		}
		var request struct {
			Model          string `json:"model"`
			Voice          string `json:"voice"`
			Input          string `json:"input"`
			Instructions   string `json:"instructions"`
			ResponseFormat string `json:"response_format"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode speech request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if request.Model != "gpt-4o-mini-tts" || request.Voice != "cedar" ||
			request.Input != "Hello there" || request.ResponseFormat != "mp3" ||
			request.Instructions == "" {
			t.Errorf("unexpected speech request: %+v", request)
		}
		_, _ = w.Write([]byte("mp3 audio"))
	}))
	defer server.Close()

	originalURL := assistantSpeechAPIURL
	assistantSpeechAPIURL = server.URL
	t.Cleanup(func() { assistantSpeechAPIURL = originalURL })
	t.Setenv("OPENAI_TTS_MODEL", "")

	audio, err := requestOpenAISpeech(t.Context(), "test-key", "Hello there", "cedar")
	if err != nil {
		t.Fatal(err)
	}
	if string(audio) != "mp3 audio" {
		t.Fatalf("speech audio = %q, want mocked audio", audio)
	}
}
