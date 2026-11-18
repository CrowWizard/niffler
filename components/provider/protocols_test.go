package main

import (
	"encoding/json"
	"testing"
)

func TestProviderProtocolsRoundTrip(t *testing.T) {
	for _, protocol := range []string{protocolResponses, protocolOpenAI, protocolChatAlias, protocolAnthropic} {
		t.Run(protocol, func(t *testing.T) {
			p := Provider{Nickname: "test", APIKey: "key", Protocol: protocol, BaseURL: "https://example.test/v1", Model: "test-model"}.withDefaults()
			if !validStoredProvider(p) {
				t.Fatalf("provider rejected: %#v", p)
			}
			raw, err := json.Marshal(p)
			if err != nil {
				t.Fatal(err)
			}
			var restored Provider
			if err = json.Unmarshal(raw, &restored); err != nil {
				t.Fatal(err)
			}
			want := protocol
			if want == protocolChatAlias {
				want = protocolOpenAI
			}
			if restored.Protocol != want || summarizeProvider(restored, false).Protocol != want {
				t.Fatalf("protocol lost: %#v", restored)
			}
		})
	}
}

func TestEnvironmentProtocol(t *testing.T) {
	for _, protocol := range []string{"", protocolChatAlias, protocolResponses, protocolOpenAI, protocolAnthropic} {
		t.Run(protocol, func(t *testing.T) {
			t.Setenv("NIF_OPENAI_PROTOCOL", protocol)
			want := protocol
			if want == "" || want == protocolChatAlias {
				want = protocolOpenAI
			}
			if got := environmentProvider().Protocol; got != want {
				t.Fatalf("protocol = %q want %q", got, want)
			}
		})
	}
}
