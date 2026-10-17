package gui

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/provider"
)

// #2409: the addresses a plugin's provider declared reach the page's JSON as
// `declared`, which the editor draws read-only under Endpoints. A provider
// whose plugin declared none carries none, and its editor is as it was.
func TestProviderDeclaredEndpointsReachThePage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	shown := func(p provider.Provider) string {
		t.Helper()
		b, err := json.Marshal(providerInfoWith(p, nil, nil))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	p := provider.Provider{
		ID:   "relayco",
		Name: "Relay Co",
		Chat: "plugin://relayco/v1",
		Declared: provider.Endpoints{
			Chat:      "https://relay.example.com/v1",
			Responses: "https://relay.example.com/r1",
		},
	}
	if b := shown(p); !strings.Contains(b, `"declared":{"chat":"https://relay.example.com/v1","responses":"https://relay.example.com/r1"}`) {
		t.Errorf("a plugin's declared endpoints didn't reach the page: %s", b)
	}
	// what it didn't declare, and a plugin provider that declared nothing
	p.Declared = provider.Endpoints{}
	if b := shown(p); strings.Contains(b, `"declared"`) {
		t.Errorf("a provider whose plugin declared nothing says declared: %s", b)
	}
}
