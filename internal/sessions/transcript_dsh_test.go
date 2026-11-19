package sessions

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
)

// dshSaidLines are a version 4 session's events, the shapes taken from real
// ones with the words replaced: the working directory dsh writes itself, the
// prompts typed, the model's reasoning, words and call of a tool, what the
// tool answered, the runtime context, a compacted session's summary, a
// prompt spliced into a running turn that says what a user/message says
// again, one that only the splice carries, and a splice dsh canceled.
func dshSaidLines() []string {
	return []string{
		`{"type":"session","version":4,"id":"dsh-a","cwd":"E:\\proj","createdAt":1790503200000}`,
		`{"type":"user/message","seq":1,"time":1790503201000,"data":{"role":"user","id":"m1","source":{"kind":"working-directory"},"content":[{"type":"text","text":"The working directory changed."}]}}`,
		`{"type":"user/message","seq":2,"time":1790503202000,"data":{"role":"user","id":"m2","source":{"kind":"user"},"content":[{"type":"text","text":"Fix the login bug"}]}}`,
		`{"type":"assistant/message","seq":3,"time":1790503203000,"data":{"turn":1,"step":1,"message":{"role":"assistant","source":{"kind":"model","provider":"magpie","model":"deepseek-v4-flash"},"content":[{"type":"reasoning","text":"The check is inverted."}]}}}`,
		`{"type":"assistant/message","seq":4,"time":1790503204000,"data":{"turn":1,"step":1,"message":{"role":"assistant","source":{"kind":"model","provider":"magpie","model":"deepseek-v4-flash"},"content":[{"type":"text","text":"I'll read the file."},{"type":"tool-call","id":"c1","name":"read","arguments":"{\"path\":\"/a\"}"}]}}}`,
		`{"type":"tool/call","seq":5,"time":1790503204500,"data":{"turn":1,"step":1,"callId":"c1","name":"read","arguments":"{\"path\":\"/a\"}"}}`,
		`{"type":"tool/result","seq":6,"time":1790503205000,"data":{"turn":1,"step":1,"message":{"role":"tool","toolCallId":"c1","content":[{"type":"text","text":"package login"}]}}}`,
		`{"type":"user/message","seq":7,"time":1790503206000,"data":{"role":"user","id":"m3","source":{"kind":"runtime-context","form":"snapshot"},"content":[{"type":"text","text":"<system-reminder />"}]}}`,
		`{"type":"assistant/message","seq":8,"time":1790503207000,"data":{"turn":1,"step":2,"message":{"role":"assistant","source":{"kind":"model","provider":"magpie","model":"deepseek-v4-flash"},"content":[{"type":"text","text":"Fixed: the check was inverted."}]}}}`,
		`{"type":"compaction/summary","seq":9,"time":1790503207500,"data":{"summary":[{"type":"text","text":"A summary so far."}]}}`,
		`{"type":"agent/inbox/spliced","seq":10,"time":1790503208000,"data":{"target":"next-turn","start":0,"inserted":[{"role":"user","id":"m4","source":{"kind":"user"},"content":[{"type":"image","attachment":{"mediaType":"image/png","width":10,"height":10}},{"type":"text","text":"Thanks"}]}]}}`,
		`{"type":"user/message","seq":11,"time":1790503209000,"data":{"role":"user","id":"m4","source":{"kind":"user"},"content":[{"type":"image","attachment":{"mediaType":"image/png","width":10,"height":10}},{"type":"text","text":"Thanks"}]}}`,
		`{"type":"agent/inbox/spliced","seq":12,"time":1790503210000,"data":{"target":"next-turn","start":0,"inserted":[{"role":"user","id":"m5","source":{"kind":"user"},"content":[{"type":"text","text":"Typed while it worked"}]}]}}`,
		`{"type":"agent/inbox/spliced","seq":13,"time":1790503211000,"data":{"target":"next-turn","start":0,"outcome":"canceled","inserted":[{"role":"user","id":"m6","source":{"kind":"user"},"content":[{"type":"text","text":"Never sent"}]}]}}`,
	}
}

// dshSaidKinds are what those events say, in order: the prompts typed as the
// user's, what dsh wrote itself and the compaction summary folded as
// context, the image pasted, and the tool's result named by the call it
// answers — the tool/call line not said twice, a message a splice and a
// user/message both carry read once, a prompt only the splice carries read,
// and a canceled splice not read.
func dshSaidKinds() []string {
	return []string{
		"user/context: The working directory ch",
		"user/text: Fix the login bug",
		"assistant/thinking: The check is inverted.",
		"assistant/text: I'll read the file.",
		`assistant/tool_use/read: { "path": "/a" }`,
		"tool/tool_result/read: package login",
		"user/context: <system-reminder />",
		"assistant/text: Fixed: the check was inv",
		"user/context: A summary so far.",
		"user/image: [image]",
		"user/text: Thanks",
		"user/text: Typed while it worked",
	}
}

// A dsh session's conversation is read whole from its file, in order: the
// prompts typed, the model's reasoning, words and calls of tools, their
// results, and what dsh wrote itself folded as context.
func TestTranscriptDsh(t *testing.T) {
	file := filepath.Join(t.TempDir(), "session.v4.jsonl")
	writeLines(t, file, dshSaidLines()...)
	tr, err := TranscriptOf(Session{Agent: "dsh", ID: "dsh-a", Path: file})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := kinds(tr.Parts), dshSaidKinds(); !reflect.DeepEqual(got, want) {
		t.Fatalf("parts\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	// an assistant's part names the model its own line does (#2253)
	if got := tr.Parts[2].Model; got != "deepseek-v4-flash" {
		t.Fatalf("the model its line names: %q", got)
	}
}

// A packed dsh session is read through its zstd frames, which hold the
// events of more than one append: the conversation is the same as the
// file's lines say.
func TestTranscriptDshPacked(t *testing.T) {
	file := filepath.Join(t.TempDir(), "session.v4.jsonl.zstd")
	enc, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer enc.Close()
	lines := dshSaidLines()
	half := len(lines) / 2
	frame := func(ls []string) string {
		return string(enc.EncodeAll([]byte(strings.Join(ls, "\n")+"\n"), nil))
	}
	// two frames, as an append of dsh's writes them
	packed := frame(lines[:half]) + frame(lines[half:])
	if err := os.WriteFile(file, []byte(packed), 0o644); err != nil {
		t.Fatal(err)
	}
	tr, err := TranscriptOf(Session{Agent: "dsh", ID: "dsh-a", Path: file})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := kinds(tr.Parts), dshSaidKinds(); !reflect.DeepEqual(got, want) {
		t.Fatalf("packed parts\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// testdata/dsh is a version 3 session as dsh wrote it, where a message's
// content is one string and a reply carries no words (a version 3 reply is
// in assistant/delta chunks, which dsh never writes in the version 4 one):
// the session is marked readable on the Sessions page, its prompts are read,
// the line that isn't an event is passed over, its words are found by
// search, and the Markdown export has them.
func TestTranscriptDshFile(t *testing.T) {
	setupAgents(t)
	s := find(t, List(All), "dsh", "dsh-a")
	if !s.Transcript {
		t.Fatalf("the session isn't marked readable: %+v", s)
	}
	tr, err := TranscriptOf(s)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"user/context: Follow the house rules", "user/text: Tidy the build script"}
	if got := kinds(tr.Parts); !reflect.DeepEqual(got, want) {
		t.Fatalf("parts\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if _, found, err := SearchSession(context.Background(), s, NeedleOf("build")); err != nil || !found {
		t.Errorf("search: found %v, %v", found, err)
	}
	if _, found, _ := SearchSession(context.Background(), s, NeedleOf("never said")); found {
		t.Error("search found words never said")
	}
	var b strings.Builder
	if err := WriteMarkdown(&b, s, "DeepSeek Harness"); err != nil {
		t.Fatal(err)
	}
	for _, w := range []string{"Follow the house rules", "Tidy the"} {
		if !strings.Contains(b.String(), w) {
			t.Errorf("markdown has no %q:\n%s", w, b.String())
		}
	}
}
