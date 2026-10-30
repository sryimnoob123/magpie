package library

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// #2540: each of Hermes's bots is a profile with a config.yaml and a skills
// folder of its own, which reads neither the root's (agent.hermesProfiles):
// the library gives each of them what it gives the root.
func TestHermesProfilesMCP(t *testing.T) {
	h := sandbox(t)
	root := filepath.Join(h, "hermes-home")
	t.Setenv("HERMES_HOME", root)
	write(t, filepath.Join(root, "config.yaml"), hermesConfig)
	// two bots of its own, each with settings magpie must leave as they are
	own := map[string]string{
		"alpha": "# alpha's own\nmodel:\n  default: mine/other\nmcp_servers:\n  theirs:\n    command: uvx-desktop\n    args: [alpha]\n",
		"beta":  "model:\n  default: mine/beta\n",
	}
	for name, cfg := range own {
		write(t, filepath.Join(root, "profiles", name, "config.yaml"), cfg)
	}
	// a profile only a tombstone is left of, and a folder with no config.yaml
	// of its own: neither is one magpie writes into. The tombstone is a
	// regular file at profiles/.deleted/<name>, so this asserts that file
	// rather than a config.yaml beneath it, which a file can't hold on any
	// platform.
	write(t, filepath.Join(root, "profiles", ".deleted", "gone"), "")
	os.MkdirAll(filepath.Join(root, "profiles", "nocfg"), 0o755)
	holdCLI(t, h, "hermes") // the config alone is gone (#2173, #2177)

	tg := targetByID("hermes")
	if tg == nil || tg.MCP == nil || tg.MCP.Path != filepath.Join(root, "config.yaml") {
		t.Fatalf("hermes has no MCP target: %+v", tg)
	}
	s := Server{Name: "web", Transport: "http", URL: "https://example.com/mcp", Agents: []string{"hermes"}}
	ok(t)(SaveServer("", s))
	cfg := func(dir string) map[string]map[string]any {
		t.Helper()
		var f struct {
			Servers map[string]map[string]any `yaml:"mcp_servers"`
			Model   struct {
				Default string `yaml:"default"`
			} `yaml:"model"`
		}
		got := read(t, filepath.Join(dir, "config.yaml"))
		if err := yaml.Unmarshal([]byte(got), &f); err != nil {
			t.Fatalf("%s: %v:\n%s", dir, err, got)
		}
		if f.Servers["web"] == nil {
			t.Errorf("%s got no server:\n%s", dir, got)
		}
		return f.Servers
	}
	if servers := cfg(root); servers["mine"] == nil || servers["web"] == nil {
		t.Errorf("the root home: %v", servers)
	}
	for name, want := range own {
		dir := filepath.Join(root, "profiles", name)
		servers := cfg(dir)
		// the profile's own server and settings stay
		if got := read(t, filepath.Join(dir, "config.yaml")); !strings.Contains(got, "uvx-desktop") && strings.Contains(want, "uvx-desktop") {
			t.Errorf("%s lost its own server:\n%s", name, got)
		}
		if name == "alpha" && servers["theirs"] == nil {
			t.Errorf("alpha lost its own server: %v", servers)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "profiles", "nocfg", "config.yaml")); !os.IsNotExist(err) {
		t.Error("magpie made a profile a config.yaml")
	}
	for _, dir := range []string{"profiles/.deleted/gone", "profiles/nocfg"} {
		if _, err := os.Stat(filepath.Join(root, dir, "config.yaml")); err == nil {
			t.Errorf("%s got a config.yaml", dir)
		}
	}

	// a sync that changes nothing writes nothing
	before := map[string]string{}
	for _, p := range []string{"config.yaml", "profiles/alpha/config.yaml", "profiles/beta/config.yaml"} {
		before[p] = read(t, filepath.Join(root, p))
	}
	ok(t)(Sync())
	for p, was := range before {
		if got := read(t, filepath.Join(root, p)); got != was {
			t.Errorf("%s was written by a sync with nothing to do:\n%s", p, got)
		}
	}

	// a bot made after the sync is given what the library has, at the next one
	write(t, filepath.Join(root, "profiles", "gamma", "config.yaml"), "model:\n  default: mine/gamma\n")
	ok(t)(Sync())
	cfg(filepath.Join(root, "profiles", "gamma"))

	// and it is taken out of every profile with the library's, not left behind
	ok(t)(RemoveServer("web"))
	for _, p := range []string{"config.yaml", "profiles/alpha/config.yaml", "profiles/beta/config.yaml", "profiles/gamma/config.yaml"} {
		got := read(t, filepath.Join(root, p))
		if strings.Contains(got, "example.com") {
			t.Errorf("%s still has it:\n%s", p, got)
		}
	}
	if got := read(t, filepath.Join(root, "profiles/alpha/config.yaml")); !strings.Contains(got, "uvx-desktop") {
		t.Errorf("taking it out took alpha's own server too:\n%s", got)
	}
}

// #2540, the skills side: a profile's skills folder is its own, so each is
// given the library's links.
func TestHermesProfilesSkills(t *testing.T) {
	h := sandbox(t)
	root := filepath.Join(h, "hermes-home")
	t.Setenv("HERMES_HOME", root)
	write(t, filepath.Join(root, "config.yaml"), hermesConfig)
	for _, name := range []string{"alpha", "beta"} {
		write(t, filepath.Join(root, "profiles", name, "config.yaml"), "model:\n  default: mine/"+name+"\n")
	}
	holdCLI(t, h, "hermes")
	if tg := targetByID("hermes"); tg == nil || tg.Skills != filepath.Join(root, "skills") {
		t.Fatalf("hermes skills: %+v", tg)
	}

	src := filepath.Join(h, "src")
	skill(t, filepath.Join(src, "pdf"), "pdf", "PDFs")
	ok(t)(InstallSkills(src, []string{"pdf"}, []string{"hermes"}))
	for _, dir := range []string{"skills", "profiles/alpha/skills", "profiles/beta/skills"} {
		p := filepath.Join(root, dir, "pdf", "SKILL.md")
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s: %v", dir, err)
			continue
		}
		if !linked(filepath.Join(root, dir, "pdf")) {
			t.Errorf("%s/pdf isn't a link to the library's", dir)
		}
	}
	// the folders it made are links' parents only: nothing else went in
	if _, err := os.Stat(filepath.Join(root, "profiles", "alpha", "skills", "pdf", "removed.md")); !os.IsNotExist(err) {
		t.Error("a file of the library's own there")
	}

	ok(t)(RemoveSkill("pdf"))
	for _, dir := range []string{"skills", "profiles/alpha/skills", "profiles/beta/skills"} {
		if _, err := os.Stat(filepath.Join(root, dir, "pdf")); !os.IsNotExist(err) {
			t.Errorf("%s/pdf is still there: %v", dir, err)
		}
	}
}
