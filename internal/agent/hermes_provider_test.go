package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/appdir"
	"github.com/yetone/magpie/internal/catalog"
	"github.com/yetone/magpie/internal/provider"
)

// hermesPluginDir is where Hermes reads a provider plugin from in this home.
func hermesPluginDir(home string) string {
	return filepath.Join(appdir.HermesHomeIn(home), "plugins", "model-providers", "magpie")
}

func hermesHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HERMES_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	return home
}

// hermesDeclared reads the models.json magpie leaves beside the plugin.
func hermesDeclared(t *testing.T, home string) map[string]map[string]any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(hermesPluginDir(home), "models.json"))
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(b, &payload); err != nil {
		t.Fatalf("models.json isn't the shape GET /v1/models answers: %v", err)
	}
	byID := map[string]map[string]any{}
	for _, e := range payload.Data {
		id, _ := e["id"].(string)
		byID[id] = e
	}
	return byID
}

// Hermes routes an image natively or through a describing model from capability probes that
// never read a provider's own /v1/models, so a gateway's models read as unknown and every
// image took the describe path. The plugin is what magpie can do about it: it registers the
// declaration beside the provider entry Hermes is connected with.
func TestHermesWritesTheProviderProfile(t *testing.T) {
	home := hermesHome(t)
	// what the gateway sets: whether an image sent to a model that doesn't take
	// one is described before it is forwarded
	described := provider.Described
	defer func() { provider.Described = described }()
	provider.Described = func() bool { return false }
	if err := provider.Save(provider.Provider{
		ID: "deepseek", Name: "DeepSeek", Chat: "https://api.deepseek.com/v1", Key: "k",
		Models: []string{"pro", "no-such-model"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.SaveLive("deepseek", "https://api.deepseek.com/v1", []catalog.Model{
		{ID: "pro", Images: true, ImageInput: imageInputBool(true), Context: 1048576, Efforts: []string{"low", "high"}},
		{ID: "flash"},
	}); err != nil {
		t.Fatal(err)
	}
	a := hermes(home)
	if err := a.Apply("model", "magpie/deepseek/pro"); err != nil {
		t.Fatal(err)
	}

	dir := hermesPluginDir(home)
	for _, name := range hermesProviderOwned {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("connect left %s out: %v", name, err)
		}
	}
	// what Hermes loads is what magpie ships, byte for byte, or an upgrade
	// could leave a stale plugin declaring yesterday's models
	for _, f := range []struct{ shipped, loaded string }{
		{"plugin.yaml", "plugin.yaml"},
		{"provider.py", "__init__.py"},
	} {
		shipped, err := hermesProviderFS.ReadFile("hermes_provider/" + f.shipped)
		if err != nil {
			t.Fatal(err)
		}
		written, err := os.ReadFile(filepath.Join(dir, f.loaded))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(shipped, written) {
			t.Fatalf("%s isn't the plugin magpie ships:\n%s", f.loaded, written)
		}
	}

	// as magpie reports this provider: it takes images, this model's window is
	// known, and it has reasoning levels
	declared := hermesDeclared(t, home)
	pro := declared["deepseek/pro"]
	if pro == nil {
		t.Fatalf("no declaration for deepseek/pro: %v", declared)
	}
	modalities, _ := pro["modalities"].(map[string]any)
	if !strings.Contains(fmt.Sprint(modalities["input"]), "image") {
		t.Fatalf("deepseek/pro isn't declared as taking images: %v", pro)
	}
	if pro["context_window"] != float64(1048576) || pro["reasoning"] != true {
		t.Fatalf("deepseek/pro is declared short of what magpie reports: %v", pro)
	}
	// a model nothing knows claims nothing, so Hermes keeps describing images
	// to it instead of sending one magpie may not take
	if none := declared["deepseek/no-such-model"]; none == nil {
		t.Fatal("a configured model is missing from the declaration")
	} else if _, claimed := none["modalities"]; claimed {
		t.Fatalf("an unknown model was declared as taking images: %v", none)
	}

	// with the gateway describing images, every model is one it takes an image
	// for, and Hermes sending the image itself spares the describing call
	provider.Described = func() bool { return true }
	if err := a.Sync(); err != nil {
		t.Fatal(err)
	}
	if described := hermesDeclared(t, home)["deepseek/no-such-model"]; described == nil {
		t.Fatal("a described model left the declaration")
	} else if _, claimed := described["modalities"]; !claimed {
		t.Fatalf("a model magpie describes images to isn't declared: %v", described)
	}
}

// Every profile reads its own plugins folder, so the plugin goes where the
// provider entry goes — and Disconnect takes out exactly what was written.
func TestHermesSyncAndDisconnectMoveTheProviderProfile(t *testing.T) {
	home := hermesHome(t)
	if err := provider.Save(provider.Provider{
		ID: "deepseek", Name: "DeepSeek", Chat: "https://api.deepseek.com/v1", Key: "k",
		Models: []string{"pro"},
	}); err != nil {
		t.Fatal(err)
	}
	root := appdir.HermesHomeIn(home)
	coder := filepath.Join(root, "profiles", "coder", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(coder), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(coder, []byte("model:\n  default: kimi-k3\n  provider: moonshot\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := hermes(home)
	if err := a.Apply("model", "magpie/deepseek/pro"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(hermesPluginDir(home)); err != nil {
		t.Fatalf("the connected home got no plugin: %v", err)
	}
	if err := a.Sync(); err != nil {
		t.Fatal(err)
	}
	// magpie is in the coder profile now, so its plugins folder gets it too:
	// the profile's Hermes reads its own, never the root's
	coderDir := filepath.Join(root, "profiles", "coder", "plugins", "model-providers", "magpie")
	if _, err := os.Stat(filepath.Join(coderDir, "models.json")); err != nil {
		t.Fatalf("a profile magpie is in got no plugin: %v", err)
	}

	// a file of the user's own in there, which isn't magpie's to remove
	mine := filepath.Join(coderDir, "mine.json")
	if err := os.WriteFile(mine, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := a.Disconnect(); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{hermesPluginDir(home), coderDir} {
		for _, name := range hermesProviderOwned {
			if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
				t.Fatalf("disconnect left %s in %s", name, dir)
			}
		}
	}
	if _, err := os.Stat(mine); err != nil {
		t.Fatalf("disconnect took a file it didn't write: %v", err)
	}
	// disconnected: a sync doesn't put the plugin back either
	if err := a.Sync(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(hermesPluginDir(home), "plugin.yaml")); !os.IsNotExist(err) {
		t.Fatal("sync after disconnect put the plugin back")
	}
}
