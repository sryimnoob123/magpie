package agent

// The provider profile magpie leaves in Hermes's own plugins folder.
//
// Hermes routes an image one of two ways (agent/image_routing.py): natively, as an image
// part the model sees, or through an auxiliary model that describes it first and hands the
// text on. The route comes from capability probes that read a models.dev catalog entry, a
// managed local runtime's /props, or Ollama's /api/show — never a provider's own GET
// /v1/models. A gateway's models are in no catalog, so magpie's read as unknown, and every
// image it served was described first: an extra call per image, and only that text reached
// the conversation.
//
// What magpie can do from its side is declare it where Hermes looks. The plugin below
// registers a "magpie" provider profile whose model_capabilities carry what magpie itself
// reports for each model (hermesProviderModelsJSON, the entries of GET /v1/models), and puts
// the same declaration on Hermes's "custom" profile, which is the name a named custom
// provider is canonicalized to before the probes run. It is written beside the config entry
// magpie connects with — in the plugins folder of Hermes's home and of every profile magpie
// is in — and Disconnect takes it out.

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/yetone/magpie/internal/catalog"
	"github.com/yetone/magpie/internal/edit"
)

//go:embed hermes_provider
var hermesProviderFS embed.FS

// hermesProviderDir is the folder Hermes reads a provider plugin from, under a home.
var hermesProviderDir = filepath.Join("plugins", "model-providers", magpieID)

// hermesProviderOwned are magpie's files in it: the plugin's two, and the declaration
// magpie writes beside them.
var hermesProviderOwned = []string{"plugin.yaml", "__init__.py", "models.json"}

// hermesProviderModelsJSON is the declaration the plugin reads: what magpie reports for the
// models this agent is shown, in the shape GET /v1/models answers them, so the plugin needs
// nothing of the catalog. Images come from magpie's own answer for a model (its Images,
// which is also true of a model magpie describes images to: handing it one over the gateway
// takes them); a model nothing knows is left without modalities and Hermes keeps describing
// images to it, as it did before this file existed.
func hermesProviderModelsJSON(ms []catalog.Model) ([]byte, error) {
	entries := []map[string]any{}
	for _, m := range ms {
		e := map[string]any{"id": m.ID}
		if m.Images {
			e["modalities"] = map[string]any{"input": []string{"text", "image"}}
		}
		if m.Context > 0 {
			e["context_window"] = m.Context
		}
		if len(m.Efforts) > 0 {
			e["reasoning"] = true
		}
		entries = append(entries, e)
	}
	return json.MarshalIndent(map[string]any{"data": entries}, "", "  ")
}

// hermesProviderWrite leaves the plugin in one Hermes home's plugins folder. A file already
// holding exactly what would be written is left as it is: Hermes notices a new plugin by the
// folder's mtime, and a connect that changes nothing shouldn't move it.
func hermesProviderWrite(home string, ms []catalog.Model) error {
	decl, err := hermesProviderModelsJSON(ms)
	if err != nil {
		return err
	}
	files := map[string][]byte{"models.json": decl}
	// Hermes loads the plugin as __init__.py; what is embedded is provider.py,
	// since go:embed skips a file whose name begins with an underscore
	for from, to := range map[string]string{"plugin.yaml": "plugin.yaml", "provider.py": "__init__.py"} {
		b, err := hermesProviderFS.ReadFile("hermes_provider/" + from)
		if err != nil {
			return err
		}
		files[to] = b
	}
	dir := filepath.Join(home, hermesProviderDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, name := range hermesProviderOwned {
		p := filepath.Join(dir, name)
		if cur, err := edit.Read(p); err == nil && bytes.Equal(cur, files[name]) {
			continue
		}
		if err := edit.WriteAtomic(p, files[name]); err != nil {
			return err
		}
	}
	return nil
}

// hermesProviderRemove takes magpie's plugin out, the way Disconnect takes magpie's provider
// out of the config: only the files written here, so anything a user left beside them stays,
// and the folder goes only once it is empty.
func hermesProviderRemove(home string) error {
	dir := filepath.Join(home, hermesProviderDir)
	var errs []error
	for _, name := range hermesProviderOwned {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err != nil {
			continue
		}
		if err := edit.Remove(p); err != nil {
			errs = append(errs, err)
		}
	}
	_ = os.Remove(dir) // fails, and is meant to, while anything else is in there
	return errors.Join(errs...)
}

// hermesProviderHomes is the Hermes home magpie connects at dir and every profile's it is
// already in — each profile reads its own plugins folder, as it reads its own config.
func hermesProviderHomes(at place, dir string) []string {
	homes := []string{dir}
	for _, p := range hermesOthers(at, dir) {
		if edit.GetYAMLMap(p, "providers."+magpieID) == nil {
			continue
		}
		homes = append(homes, filepath.Dir(p))
	}
	return homes
}
