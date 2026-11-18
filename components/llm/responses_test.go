package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	openai "github.com/sashabaranov/go-openai"
)

func TestChatResponsesAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("request path/auth = %s / %s", r.URL.Path, r.Header.Get("Authorization"))
		}
		for _, header := range []string{"chatgpt-account-id", "OpenAI-Beta", "originator", "session-id"} {
			if r.Header.Get(header) != "" {
				t.Errorf("unexpected Codex header %s", header)
			}
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		if body["model"] != "gpt-test" || body["max_output_tokens"] != float64(2048) || body["store"] != false || body["stream"] != true {
			t.Errorf("request = %#v", body)
		}
		for _, field := range []string{"include", "text", "prompt_cache_key"} {
			if _, ok := body[field]; ok {
				t.Errorf("unexpected Codex field %s", field)
			}
		}
		if body["instructions"] != "system" || body["reasoning"].(map[string]any)["effort"] != "high" {
			t.Errorf("instructions/reasoning = %#v", body)
		}
		input := body["input"].([]any)
		call := input[1].(map[string]any)
		output := input[2].(map[string]any)
		if call["call_id"] != "call_old" || output["call_id"] != "call_old" || output["type"] != "function_call_output" || output["output"] != "ok" {
			t.Errorf("input = %#v", input)
		}
		tools := body["tools"].([]any)
		if tools[0].(map[string]any)["name"] != "bash" {
			t.Errorf("tools = %#v", tools)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		events := []string{
			`{"type":"response.output_text.delta","delta":"Hello"}`,
			`{"type":"response.output_item.added","output_index":1,"item":{"type":"function_call","id":"fc_new","call_id":"call_new","name":"bash","arguments":""}}`,
			`{"type":"response.function_call_arguments.delta","output_index":1,"delta":"{\"cmd\":\"pwd\"}"}`,
			`{"type":"response.completed","response":{"model":"gpt-actual","usage":{"input_tokens":20,"output_tokens":8,"total_tokens":28,"input_tokens_details":{"cached_tokens":12},"output_tokens_details":{"reasoning_tokens":3}}}}`,
		}
		for _, event := range events {
			fmt.Fprintf(w, "data: %s\n\n", event)
		}
	}))
	defer server.Close()
	args := chatArgs{ReasoningEffort: "high", SessionID: "session", Messages: []chatMessage{
		{Role: "system", Content: "system"}, {Role: "user", Content: "hello"},
		{Role: "assistant", ToolCalls: []openai.ToolCall{{ID: "call_old|fc_old", Type: openai.ToolTypeFunction, Function: openai.FunctionCall{Name: "bash", Arguments: `{"cmd":"true"}`}}}},
		{Role: "tool", ToolCallID: "call_old|fc_old", Content: "ok"},
	}, Tools: []openai.Tool{{Type: openai.ToolTypeFunction, Function: &openai.FunctionDefinition{Name: "bash", Parameters: map[string]any{"type": "object"}}}}}
	result, err := chatDispatch(context.Background(), nil, resolvedConfig{Provider: provider{Protocol: protocolResponses, BaseURL: server.URL + "/v1", APIKey: "test-key"}, Model: "gpt-test", ProviderName: "test", Context: 128000}, "gpt-test", args, 2048)
	if err != nil {
		t.Fatal(err)
	}
	got := result.(map[string]any)
	if got["content"] != "Hello" || got["model"] != "gpt-actual" {
		t.Fatalf("result = %#v", got)
	}
	calls := got["tool_calls"].([]map[string]any)
	if len(calls) != 1 || calls[0]["id"] != "call_new|fc_new" || calls[0]["function"].(map[string]any)["arguments"] != `{"cmd":"pwd"}` {
		t.Fatalf("calls = %#v", calls)
	}
	usage := got["usage"].(map[string]any)
	if usage["prompt_tokens"] != 20 || usage["prompt_tokens_details"].(map[string]any)["cached_tokens"] != 12 || usage["completion_tokens_details"].(map[string]any)["reasoning_tokens"] != 3 {
		t.Fatalf("usage = %#v", usage)
	}
}

func TestResponsesTerminalEvents(t *testing.T) {
	for _, test := range []struct {
		name, event, wantErr string
		length               bool
	}{
		{name: "truncated stream", event: `{"type":"response.output_text.delta","delta":"partial"}`, wantErr: "without a terminal response"},
		{name: "failed", event: `{"type":"response.failed","response":{"error":{"message":"rejected"}}}`, wantErr: "rejected"},
		{name: "incomplete", event: `{"type":"response.incomplete","response":{"status":"incomplete"}}`, length: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprintf(w, "data: %s\n\n", test.event)
			}))
			defer server.Close()
			result, err := chatResponses(context.Background(), nil, provider{BaseURL: server.URL, APIKey: "key"}, "model", "test", chatArgs{Messages: []chatMessage{{Role: "user", Content: "hello"}}}, 128000, 2048, false)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("error = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if test.length && result.(map[string]any)["finish_reason"] != finishLength {
				t.Fatalf("result = %#v", result)
			}
		})
	}
}

func TestLoadProviderProtocols(t *testing.T) {
	t.Setenv("NIF_OPENAI_PROTOCOL", protocolResponses)
	t.Setenv("NIF_LLM_PROVIDERS", `{"legacy":{"protocol":"openai-chat"},"chat":{"protocol":"openai-chat-completions"},"responses":{"protocol":"openai-responses"},"claude":{"protocol":"anthropic"}}`)
	ps, err := loadProviders()
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"default": protocolResponses, "legacy": protocolOpenAI, "chat": protocolOpenAI, "responses": protocolResponses, "claude": protocolAnthropic} {
		if ps[name].Protocol != want {
			t.Errorf("%s protocol = %q", name, ps[name].Protocol)
		}
	}
}
