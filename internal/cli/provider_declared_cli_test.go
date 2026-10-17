package cli

import (
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/provider"
)

// #2409: `magpie provider <id>` shows where a plugin's provider says its
// requests end up, and says they are reached through the plugin — the
// plugin://<id> a request is routed by is no address to print. A provider
// whose plugin declared none prints as it did.
func TestProviderShowsPluginDeclaredEndpoints(t *testing.T) {
	groupsHome(t)
	p := provider.Provider{
		ID:        "relayco",
		Name:      "Relay Co",
		Chat:      "plugin://relayco/v1",
		Responses: "plugin://relayco/v1",
		Anthropic: "plugin://relayco",
		Declared: provider.Endpoints{
			Chat:      "https://relay.example.com/v1",
			Responses: "https://relay.example.com/r1",
			Anthropic: "https://relay.example.com/an",
		},
	}
	out, err := printed(t, func() error { return showProvider(p) })
	if err != nil {
		t.Fatalf("showProvider: %v", err)
	}
	for _, want := range []string{
		"https://relay.example.com/v1  through the plugin",
		"https://relay.example.com/r1  through the plugin",
		"https://relay.example.com/an  through the plugin",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("printed without %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "plugin://") {
		t.Errorf("printed the routing base:\n%s", out)
	}

	// a plugin that declared nothing: no chat line at all, as before
	quiet := provider.Provider{ID: "silentco", Name: "Silent Co", Chat: "plugin://silentco/v1"}
	out, err = printed(t, func() error { return showProvider(quiet) })
	if err != nil {
		t.Fatalf("showProvider: %v", err)
	}
	if strings.Contains(out, "plugin://") {
		t.Errorf("a provider whose plugin declared nothing printed endpoints:\n%s", out)
	}
}
