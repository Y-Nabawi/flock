package models

import "testing"

// TestParseSchemeID covers the three accepted prefixes and the fall-
// through case (a plain catalog id is not a scheme — return ok=false so
// callers fall back to catalog lookup).
func TestParseSchemeID(t *testing.T) {
	cases := []struct {
		in       string
		wantOK   bool
		wantType string
		wantRepo string
		wantFile string
		wantName string
		wantPath string
	}{
		{in: "hf:Qwen/Qwen3-72B-AWQ", wantOK: true, wantType: "huggingface", wantRepo: "Qwen/Qwen3-72B-AWQ"},
		{in: "hf:bartowski/Phi-3-mini-GGUF:Phi-3-mini-4k-instruct.Q4_K_M.gguf", wantOK: true, wantType: "huggingface", wantRepo: "bartowski/Phi-3-mini-GGUF", wantFile: "Phi-3-mini-4k-instruct.Q4_K_M.gguf"},
		{in: "hf:invalid-no-slash", wantOK: false},
		{in: "hf:", wantOK: false},
		{in: "ollama:phi3", wantOK: true, wantType: "ollama", wantName: "phi3"},
		{in: "ollama:phi3:mini", wantOK: true, wantType: "ollama", wantName: "phi3:mini"},
		{in: "ollama:", wantOK: false},
		{in: "file:/tmp/x.gguf", wantOK: true, wantType: "file", wantPath: "/tmp/x.gguf"},
		{in: "file:./relative.gguf", wantOK: true, wantType: "file", wantPath: "./relative.gguf"},
		{in: "file:", wantOK: false},
		{in: "llama-3.2-3b", wantOK: false}, // plain catalog id
		{in: "", wantOK: false},
	}
	for _, c := range cases {
		e, ok := ParseSchemeID(c.in)
		if ok != c.wantOK {
			t.Errorf("%q: ok=%v want %v", c.in, ok, c.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if e.Source.Type != c.wantType {
			t.Errorf("%q: type=%q want %q", c.in, e.Source.Type, c.wantType)
		}
		if e.Source.Repo != c.wantRepo {
			t.Errorf("%q: repo=%q want %q", c.in, e.Source.Repo, c.wantRepo)
		}
		if e.Source.File != c.wantFile {
			t.Errorf("%q: file=%q want %q", c.in, e.Source.File, c.wantFile)
		}
		if e.Source.OllamaName != c.wantName {
			t.Errorf("%q: ollama_name=%q want %q", c.in, e.Source.OllamaName, c.wantName)
		}
		if e.Source.Path != c.wantPath {
			t.Errorf("%q: path=%q want %q", c.in, e.Source.Path, c.wantPath)
		}
		if e.ID != c.in {
			t.Errorf("%q: ID=%q want %q (full scheme-prefixed id should round-trip)", c.in, e.ID, c.in)
		}
	}
}
