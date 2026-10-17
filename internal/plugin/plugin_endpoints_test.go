package plugin

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// #2409: a plugin's provider says where its vendor answers on each API
// (the config hook's provider[ID].endpoints, magpie's own field), and the
// host hands them on as the plugin said them: what is no address is
// dropped, one that names none is no list, and the provider's api stands
// for any API it didn't name.
func TestPluginEndpointsDeclared(t *testing.T) {
	sandbox(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	abs, err := filepath.Abs("testdata/endpoints")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Add(ctx, abs); err != nil {
		t.Fatal(err)
	}
	ps, err := Providers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]Provider{}
	for _, p := range ps {
		by[p.ID] = p
	}
	multi, ok := by["relayco"]
	if !ok {
		t.Fatalf("relayco not listed: %+v", ps)
	}
	want := map[string]string{
		"responses": "https://relay.invalid/responses",
		"anthropic": "https://relay.invalid/anthropic",
	}
	if !reflect.DeepEqual(multi.Endpoints, want) {
		t.Fatalf("relayco's endpoints = %#v, want %#v", multi.Endpoints, want)
	}
	// any API it named none for answers at its api
	if got := multi.Endpoint("chat"); got != "https://relay.invalid/v1" {
		t.Errorf("relayco's chat endpoint = %q, want its api", got)
	}
	if got := multi.Endpoint("responses"); got != want["responses"] {
		t.Errorf("relayco's responses endpoint = %q", got)
	}
	if got := multi.Endpoint("gemini"); got != "https://relay.invalid/v1" {
		t.Errorf("relayco's gemini endpoint = %q, want its api (the object declared is no address)", got)
	}
	if got := by["oneline"].Endpoint("anthropic"); got != "https://oneline.invalid/v1" {
		t.Errorf("oneline's anthropic endpoint = %q, want its api", got)
	}
	if got := by["oneline"].Endpoints; len(got) != 0 {
		t.Errorf("oneline declared endpoints = %#v, want none", got)
	}
	if got := by["silentco"].Endpoint("chat"); got != "" {
		t.Errorf("silentco's chat endpoint = %q, want none", got)
	}
}
