package main

import (
	"encoding/json"
	"testing"
)

func TestBuildMimoASRPayload(t *testing.T) {
	payload := BuildMimoASRPayload("dGVzdA==", "wav", "mimo-v2.5-asr")

	if payload["model"] != "mimo-v2.5-asr" {
		t.Errorf("model = %v, want mimo-v2.5-asr", payload["model"])
	}

	messages, ok := payload["messages"].([]map[string]interface{})
	if !ok || len(messages) == 0 {
		t.Fatal("messages missing or empty")
	}

	msg := messages[0]
	if msg["role"] != "user" {
		t.Errorf("role = %v, want user", msg["role"])
	}

	content, ok := msg["content"].([]map[string]interface{})
	if !ok || len(content) == 0 {
		t.Fatal("content missing")
	}

	item := content[0]
	if item["type"] != "input_audio" {
		t.Errorf("type = %v, want input_audio", item["type"])
	}

	audio, ok := item["input_audio"].(map[string]string)
	if !ok {
		t.Fatal("input_audio missing")
	}

	wantDataURL := "data:audio/wav;base64,dGVzdA=="
	if audio["data"] != wantDataURL {
		t.Errorf("data URL = %q, want %q", audio["data"], wantDataURL)
	}
}

func TestBuildMimoASRPayloadMP3(t *testing.T) {
	payload := BuildMimoASRPayload("AAAA", "mp3", "mimo-v2.5-asr")
	messages := payload["messages"].([]map[string]interface{})
	content := messages[0]["content"].([]map[string]interface{})
	audio := content[0]["input_audio"].(map[string]string)

	want := "data:audio/mpeg;base64,AAAA"
	if audio["data"] != want {
		t.Errorf("data URL = %q, want %q", audio["data"], want)
	}
}

func TestExtractTranscriptString(t *testing.T) {
	resp := map[string]interface{}{
		"choices": []interface{}{
			map[string]interface{}{
				"message": map[string]interface{}{
					"content": "Hello world transcript",
				},
			},
		},
	}
	body, _ := json.Marshal(resp)

	text, err := ExtractTranscript(body)
	if err != nil {
		t.Fatal(err)
	}
	if text != "Hello world transcript" {
		t.Errorf("text = %q, want %q", text, "Hello world transcript")
	}
}

func TestExtractTranscriptList(t *testing.T) {
	resp := map[string]interface{}{
		"choices": []interface{}{
			map[string]interface{}{
				"message": map[string]interface{}{
					"content": []interface{}{
						map[string]interface{}{"text": "Part one"},
						map[string]interface{}{"text": "Part two"},
					},
				},
			},
		},
	}
	body, _ := json.Marshal(resp)

	text, err := ExtractTranscript(body)
	if err != nil {
		t.Fatal(err)
	}
	if text != "Part one\nPart two" {
		t.Errorf("text = %q, want %q", text, "Part one\nPart two")
	}
}

func TestExtractTranscriptListWithContent(t *testing.T) {
	resp := map[string]interface{}{
		"choices": []interface{}{
			map[string]interface{}{
				"message": map[string]interface{}{
					"content": []interface{}{
						map[string]interface{}{"content": "Via content field"},
					},
				},
			},
		},
	}
	body, _ := json.Marshal(resp)

	text, err := ExtractTranscript(body)
	if err != nil {
		t.Fatal(err)
	}
	if text != "Via content field" {
		t.Errorf("text = %q, want %q", text, "Via content field")
	}
}

func TestExtractTranscriptMissingChoices(t *testing.T) {
	body := []byte(`{"usage": {}}`)

	_, err := ExtractTranscript(body)
	if err == nil {
		t.Fatal("expected error for missing choices")
	}
}

func TestExtractTranscriptEmptyChoices(t *testing.T) {
	body := []byte(`{"choices": []}`)

	_, err := ExtractTranscript(body)
	if err == nil {
		t.Fatal("expected error for empty choices")
	}
}
