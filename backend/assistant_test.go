package main

import (
	"encoding/base64"
	"encoding/json"
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
