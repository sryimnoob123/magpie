package sessions

import (
	"bytes"
	"encoding/json"
)

// DeepSeek Harness's conversation, read from a session's file (the format is
// described at dsh.go): the prompts typed, what the model said — its words,
// its reasoning and the calls of a tool — and what each tool gave back. A
// user/message dsh wrote itself, an instruction file, the runtime context or
// a subagent's report, is shown folded as context, never as something the
// user said, so distillation reads the prompts alone (memdistill's
// transcript keeps text parts); a compacted session's summary is folded the
// same way.
//
// An agent/inbox/spliced line carries the messages dsh is about to put into
// a running turn — a prompt typed while the agent was busy, the runtime
// context — and those are read when nothing else has said them yet: a
// message is read once, by its id, whichever of the two lines carries it
// first, and a splice dsh canceled is not read at all. A session seeded
// from another shows the events it inherited, as a fork's do.
//
// A packed session (session.v<N>.jsonl.zstd) is read through its frames as
// any packed session is (openLines): a transcript is read when it is asked
// for, whole, so nothing is carried between reads. A version 3 file's
// replies are not read: that format's assistant/message carries no words,
// only its assistant/delta chunks do.

// dshSaid are the events a conversation is read from, tested before a line
// is decoded.
var dshSaid = [][]byte{[]byte(`"user/message"`), []byte(`"assistant/message"`), []byte(`"tool/result"`), []byte(`"compaction/summary"`), []byte(`"agent/inbox/spliced"`)}

// dshBlock is one content block a message holds: words, a reasoning, a call
// of a tool or a pasted image.
type dshBlock struct {
	Type      string `json:"type"`
	Text      string `json:"text"`
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// dshLine is an event line a conversation is read from.
type dshLine struct {
	Type string `json:"type"`
	Data struct {
		Content json.RawMessage `json:"content"`
		Summary json.RawMessage `json:"summary"`
		ID      string          `json:"id"`
		Outcome string          `json:"outcome"`
		Source  struct {
			Kind string `json:"kind"`
			Name string `json:"name"`
		} `json:"source"`
		Inserted []struct {
			Content json.RawMessage `json:"content"`
			Role    string          `json:"role"`
			ID      string          `json:"id"`
			Source  struct {
				Kind string `json:"kind"`
			} `json:"source"`
		} `json:"inserted"`
		Message struct {
			Content    json.RawMessage `json:"content"`
			ToolCallID string          `json:"toolCallId"`
			Source     struct {
				Model string `json:"model"`
			} `json:"source"`
		} `json:"message"`
	} `json:"data"`
}

// dshTranscript tells add what a dsh session said, in the order of its file,
// decoding only the lines keep says yes to.
func dshTranscript(path string, keep func([]byte) bool, add func(bool, Part) bool) error {
	names := map[string]string{} // a tool call's name, by its id
	read := map[string]bool{}    // a message read already, by its id
	_, err := scanAt(path, 0, nil, func(b []byte, _, _ int64) bool {
		said := false
		for _, k := range dshSaid {
			if bytes.Contains(b, k) {
				said = true
				break
			}
		}
		if !said || (keep != nil && !keep(b)) {
			return true
		}
		var l dshLine
		if json.Unmarshal(b, &l) != nil {
			return true
		}
		for _, p := range dshParts(l, names, read) {
			if !add(p.Role != "assistant", p) {
				return false
			}
		}
		return true
	})
	return err
}

// dshParts are what one event said.
func dshParts(l dshLine, names map[string]string, read map[string]bool) []Part {
	switch l.Type {
	case "user/message":
		if l.Data.ID != "" {
			if read[l.Data.ID] {
				return nil
			}
			read[l.Data.ID] = true
		}
		return dshText(l.Data.Source.Kind, dshBlocks(l.Data.Content))
	case "agent/inbox/spliced":
		if l.Data.Outcome == "canceled" {
			return nil
		}
		var parts []Part
		for _, ins := range l.Data.Inserted {
			if ins.ID != "" {
				if read[ins.ID] {
					continue
				}
				read[ins.ID] = true
			}
			parts = append(parts, dshText(ins.Source.Kind, dshBlocks(ins.Content))...)
		}
		return parts
	case "compaction/summary":
		// what a compacted session's earlier turns were folded into
		var parts []Part
		for _, c := range dshBlocks(l.Data.Summary) {
			if c.Type == "text" {
				parts = append(parts, Part{Role: "user", Kind: "context", Text: c.Text})
			}
		}
		return parts
	case "assistant/message":
		var parts []Part
		model := l.Data.Message.Source.Model
		for _, c := range dshBlocks(l.Data.Message.Content) {
			switch c.Type {
			case "text":
				parts = append(parts, Part{Role: "assistant", Kind: "text", Text: c.Text, Model: model})
			case "reasoning":
				parts = append(parts, Part{Role: "assistant", Kind: "thinking", Text: c.Text, Model: model})
			case "tool-call":
				names[c.ID] = c.Name
				parts = append(parts, Part{Role: "assistant", Kind: "tool_use", Name: c.Name,
					Text: pretty(json.RawMessage(c.Arguments)), Model: model})
			case "image":
				parts = append(parts, Part{Role: "assistant", Kind: "image", Text: "[image]", Model: model})
			}
		}
		return parts
	case "tool/result":
		m := l.Data.Message
		return []Part{{Role: "tool", Kind: "tool_result", Name: names[m.ToolCallID], Text: resultText(m.Content)}}
	}
	return nil
}

// dshText is what a message dsh wrote said: the words of a prompt typed its
// caller's, and what dsh put in itself — an instruction file, the runtime
// context, a subagent's report, a skill typed as /<name> — folded.
func dshText(kind string, blocks []dshBlock) []Part {
	var parts []Part
	for _, c := range blocks {
		switch {
		case c.Type == "text" && kind == "user":
			parts = append(parts, textPart("user", c.Text))
		case c.Type == "text":
			parts = append(parts, Part{Role: "user", Kind: "context", Text: c.Text})
		case c.Type == "image":
			parts = append(parts, Part{Role: "user", Kind: "image", Text: "[image]"})
		}
	}
	return parts
}

// dshBlocks are a message's content blocks; a message whose content is one
// string holds those words alone.
func dshBlocks(raw json.RawMessage) []dshBlock {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return []dshBlock{{Type: "text", Text: s}}
	}
	var blocks []dshBlock
	if json.Unmarshal(raw, &blocks) != nil {
		return nil
	}
	return blocks
}
