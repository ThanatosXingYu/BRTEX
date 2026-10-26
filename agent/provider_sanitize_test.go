package agent

import (
	"testing"

	"github.com/Autumn-27/norma/llm"
)

func TestConfigFromStripsInvisibleCharacters(t *testing.T) {
	c := ConfigFrom("openai", "Qwen3.8-Flash", "\u200bhttp://127.0.0.1:9999/v1\u200b", "\ufeff123", "\u200bhttp://127.0.0.1:7897")
	if c.BaseURL != "http://127.0.0.1:9999/v1" {
		t.Fatalf("BaseURL=%q", c.BaseURL)
	}
	if c.APIKey != "123" {
		t.Fatalf("APIKey=%q", c.APIKey)
	}
	if c.Model != "Qwen3.8-Flash" {
		t.Fatalf("Model=%q", c.Model)
	}
	if c.Proxy != "http://127.0.0.1:7897" {
		t.Fatalf("Proxy=%q", c.Proxy)
	}
	if c.Format != llm.FormatOpenAI {
		t.Fatalf("Format=%q", c.Format)
	}
}
