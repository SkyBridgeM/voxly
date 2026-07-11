package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const deepSeekSystemPrompt = `你是一个会议纪要助手。请严格遵循以下底线约束：

- 只根据用户提供的转写文本和 prompt 生成纪要；
- 不编造任何事实、人物、日期、数字或行动项；
- 不确定的内容标注"不确定"或"未提及"；
- 输出使用 Markdown 格式；
- 用户 prompt 中的格式要求优先于上述原则。`

// generateDeepSeekMinutes calls the DeepSeek API to generate meeting minutes from notes_input.md.
func generateDeepSeekMinutes(notesInputPath string, settings *Settings) (string, error) {
	apiKey, err := settings.RequireDeepSeekAPIKey()
	if err != nil {
		return "", err
	}

	notesData, err := os.ReadFile(notesInputPath)
	if err != nil {
		return "", fmt.Errorf("reading notes input: %w", err)
	}
	notesText := strings.TrimSpace(string(notesData))

	payload := map[string]interface{}{
		"model": settings.DeepSeekModel,
		"messages": []map[string]string{
			{"role": "system", "content": deepSeekSystemPrompt},
			{"role": "user", "content": notesText},
		},
		"stream": false,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", settings.DeepSeekEndpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: time.Duration(settings.LLMTimeoutSeconds) * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("DeepSeek API request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading DeepSeek response: %w", err)
	}

	if err := checkDeepSeekStatus(resp.StatusCode, respBody); err != nil {
		return "", err
	}

	return extractDeepSeekContent(respBody)
}

func checkDeepSeekStatus(statusCode int, body []byte) error {
	switch {
	case statusCode == 401:
		return fmt.Errorf("DeepSeek API authentication failed (401). Check DEEPSEEK_API_KEY")
	case statusCode == 402:
		return fmt.Errorf("DeepSeek API returned 402. Check your account balance")
	case statusCode == 429:
		return fmt.Errorf("DeepSeek API rate limit exceeded (429). Retry later")
	case statusCode >= 400 && statusCode < 500:
		return fmt.Errorf("DeepSeek API client error (%d): %s", statusCode, extractDeepSeekError(body))
	case statusCode >= 500:
		return fmt.Errorf("DeepSeek API server error (%d): %s", statusCode, extractDeepSeekError(body))
	default:
		return nil
	}
}

func extractDeepSeekError(body []byte) string {
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return truncate(string(body), 300)
	}
	if errObj, ok := data["error"].(map[string]interface{}); ok {
		if msg, ok := errObj["message"].(string); ok {
			return msg
		}
	}
	return truncate(string(body), 300)
}

func extractDeepSeekContent(body []byte) (string, error) {
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return "", fmt.Errorf("DeepSeek API returned invalid JSON")
	}

	choices, ok := data["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return "", fmt.Errorf("DeepSeek API response missing choices")
	}

	choice, ok := choices[0].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("DeepSeek API response invalid choice format")
	}

	message, ok := choice["message"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("DeepSeek API response missing message")
	}

	content, ok := message["content"].(string)
	if !ok {
		return "", fmt.Errorf("DeepSeek API response content is not a string")
	}

	content = strings.TrimSpace(content)
	if content == "" {
		return "", fmt.Errorf("DeepSeek API returned empty content")
	}

	return content, nil
}
