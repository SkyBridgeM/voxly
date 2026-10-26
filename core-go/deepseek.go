package main

import (
	"bytes"
	"context"
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

// generateDeepSeekMinutes is the deprecated legacy minutes adapter.
func generateDeepSeekMinutes(notesInputPath string, settings *Settings) (string, error) {
	return generateDeepSeekMinutesContext(context.Background(), notesInputPath, settings)
}

func generateDeepSeekMinutesContext(ctx context.Context, notesInputPath string, settings *Settings) (string, error) {
	notesData, err := os.ReadFile(notesInputPath)
	if err != nil {
		return "", fmt.Errorf("reading notes input: %w", err)
	}
	notesText := strings.TrimSpace(string(notesData))

	return generateDeepSeekContentContext(ctx, deepSeekSystemPrompt, notesText, settings)
}

// generateDeepSeekContent is the provider boundary for Processing Model V2.
// Template selection and transcript framing are owned by prompt composition,
// not by this adapter.
func generateDeepSeekContent(systemPrompt, userPrompt string, settings *Settings) (string, error) {
	return generateDeepSeekContentContext(context.Background(), systemPrompt, userPrompt, settings)
}

// generateDeepSeekContentContext is the cancellable form of the legacy adapter.
func generateDeepSeekContentContext(ctx context.Context, systemPrompt, userPrompt string, settings *Settings) (string, error) {
	outcome, err := generateDeepSeekContentDetailed(ctx, systemPrompt, userPrompt, settings)
	if err != nil {
		return "", err
	}
	if outcome.Text == "" {
		return "", fmt.Errorf("DeepSeek API returned empty content")
	}
	return outcome.Text, nil
}

// generateDeepSeekContentDetailed additionally reports the provider's structured
// finish reason so callers can tell a complete result apart from a truncated,
// refused, or unverifiable one.
func generateDeepSeekContentDetailed(ctx context.Context, systemPrompt, userPrompt string, settings *Settings) (generationOutcome, error) {
	apiKey, err := settings.RequireDeepSeekAPIKey()
	if err != nil {
		return generationOutcome{}, err
	}
	payload := map[string]interface{}{
		"model": settings.DeepSeekModel,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
		"stream": false,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return generationOutcome{}, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", settings.DeepSeekEndpoint, bytes.NewReader(body))
	if err != nil {
		return generationOutcome{}, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: time.Duration(settings.LLMTimeoutSeconds) * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return generationOutcome{}, fmt.Errorf("DeepSeek API request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return generationOutcome{}, fmt.Errorf("reading DeepSeek response: %w", err)
	}

	if err := checkDeepSeekStatus(resp.StatusCode, respBody); err != nil {
		return generationOutcome{}, err
	}

	return extractDeepSeekCompletion(respBody)
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

// extractDeepSeekCompletion parses the structured stop reason first and only
// then decides how much text the response must contain:
//   - a normal stop must carry text, otherwise the response is unusable;
//   - a refusal or a truncated stop may legitimately carry no text at all, and
//     keeps its own completion class instead of collapsing into a provider error;
//   - a malformed response (bad JSON, missing choice/message, wrongly typed
//     content) stays a parse error.
func extractDeepSeekCompletion(body []byte) (generationOutcome, error) {
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return generationOutcome{}, fmt.Errorf("DeepSeek API returned invalid JSON")
	}

	choices, ok := data["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return generationOutcome{}, fmt.Errorf("DeepSeek API response missing choices")
	}

	choice, ok := choices[0].(map[string]interface{})
	if !ok {
		return generationOutcome{}, fmt.Errorf("DeepSeek API response invalid choice format")
	}

	message, ok := choice["message"].(map[string]interface{})
	if !ok {
		return generationOutcome{}, fmt.Errorf("DeepSeek API response missing message")
	}

	finishReason := ""
	if raw, ok := choice["finish_reason"].(string); ok {
		finishReason = strings.TrimSpace(raw)
	}

	content := ""
	if rawContent, present := message["content"]; present && rawContent != nil {
		text, ok := rawContent.(string)
		if !ok {
			return generationOutcome{}, fmt.Errorf("DeepSeek API response content is not a string")
		}
		content = strings.TrimSpace(text)
	}

	if content == "" {
		switch classifyGenerationCompletion(finishReason) {
		case generationCompletionRefused, generationCompletionIncomplete:
			// The provider declined the request or stopped at a limit without
			// returning text; keep that class instead of reporting a parse error.
		default:
			return generationOutcome{}, fmt.Errorf("DeepSeek API returned empty content")
		}
	}

	return generationOutcome{Text: content, FinishReason: finishReason}, nil
}

// extractDeepSeekContent keeps the plain-text contract used by the legacy
// minutes adapter, which has no structured completion classification.
func extractDeepSeekContent(body []byte) (string, error) {
	outcome, err := extractDeepSeekCompletion(body)
	if err != nil {
		return "", err
	}
	if outcome.Text == "" {
		return "", fmt.Errorf("DeepSeek API returned empty content")
	}
	return outcome.Text, nil
}
