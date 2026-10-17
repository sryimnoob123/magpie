package provider

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/plugin"
)

// #2409: a plugin that says where its vendor answers on each API has those
// shown beside the plugin://<id> every request is routed by — the routing
// stays the plugin's own, and what it didn't name answers at its api.
func TestPluginDeclaredEndpointsShownNotRouted(t *testing.T) {
	pp := plugin.Provider{
		ID:   "relayco",
		Name: "Relay Co",
		API:  "https://relay.example.com/v1",
		Endpoints: map[string]string{
			"responses": "https://relay.example.com/r1",
			"anthropic": " https://relay.example.com/an ",
		},
		Models: []plugin.Model{
			{ID: "relay-chat", NPM: "@ai-sdk/openai-compatible"},
			{ID: "relay-gpt", NPM: "@ai-sdk/openai"},
			{ID: "relay-claude", NPM: "@ai-sdk/anthropic"},
		},
	}
	p := pluginProvider(pp, pluginLogin{})
	// routed exactly as before: plugin://<id>, which pluginFetch trims
	if p.Chat != pluginBase+"relayco/v1" || p.Responses != pluginBase+"relayco/v1" || p.Anthropic != pluginBase+"relayco" {
		t.Fatalf("routed at %q, %q, %q, want plugin://relayco/…", p.Chat, p.Responses, p.Anthropic)
	}
	want := Endpoints{
		Chat:      "https://relay.example.com/v1", // none named for chat: its api
		Responses: "https://relay.example.com/r1",
		Anthropic: "https://relay.example.com/an",
	}
	if p.Declared != want {
		t.Fatalf("declared = %+v, want %+v", p.Declared, want)
	}
	// the editor reads it as p.declared: only the APIs it serves
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"declared":{"chat":"https://relay.example.com/v1","responses":"https://relay.example.com/r1","anthropic":"https://relay.example.com/an"}`) {
		t.Errorf("marshalled declared endpoints: %s", b)
	}
}

// A plugin that says nothing about where its vendor answers is shown as it
// was: no endpoints, only the plugin://<id> a request is routed by.
func TestPluginProviderWithoutDeclaredEndpointsSaysNone(t *testing.T) {
	pp := plugin.Provider{ID: "silentco", Models: []plugin.Model{{ID: "silent-1"}}}
	p := pluginProvider(pp, pluginLogin{})
	if p.Chat != pluginBase+"silentco/v1" {
		t.Fatalf("routed at %q, want plugin://silentco/v1", p.Chat)
	}
	if p.Declared != (Endpoints{}) {
		t.Fatalf("declared = %+v, want none", p.Declared)
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), `"declared"`) {
		t.Errorf("a provider whose plugin declared nothing says declared: %s", b)
	}
}

// A plugin in front of Gemini's API has its models spoken to as Code
// Assist's inner request: the gemini address it declares is that vendor's.
func TestPluginDeclaredEndpointsGemini(t *testing.T) {
	pp := plugin.Provider{
		ID:        "googleco",
		API:       "https://googleco.example.com/v1beta",
		Endpoints: map[string]string{"gemini": "https://googleco.example.com/gemini"},
		Models:    []plugin.Model{{ID: "gemini-9", NPM: "@ai-sdk/google"}},
	}
	p := pluginProvider(pp, pluginLogin{})
	if p.Declared.Gemini != "https://googleco.example.com/gemini" {
		t.Fatalf("declared = %+v, want its gemini address", p.Declared)
	}
	if p.Declared.Chat != "" || p.Declared.Responses != "" {
		t.Errorf("declared = %+v, want the APIs it serves alone", p.Declared)
	}
}
