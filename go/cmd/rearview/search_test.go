package main

import (
	"strings"
	"testing"
)

func TestEmbedSearchAndScoreWithFakeEngine(t *testing.T) {
	cfg, dir := testEnv(t)
	withEngine(t, cfg, "")
	if _, _, err := execute(t, "import", card(t, dir), "--config", cfg); err != nil {
		t.Fatal(err)
	}
	out, _, err := execute(t, "process", "--embed", "--plain", "--config", cfg)
	if err != nil || !strings.Contains(out, "planned 3 new embed_frames jobs") {
		t.Fatalf("%v\n%s", err, out)
	}
	out, _, err = execute(t, "search", "anything", "--crops", "center", "--config", cfg)
	if err != nil || !strings.Contains(out, "keyframes searched in") || !strings.HasPrefix(out, "#") {
		t.Fatalf("search: %v\n%s", err, out)
	}
	out, _, err = execute(t, "search", "anything", "--json", "--limit", "2", "--config", cfg)
	if err != nil || !strings.Contains(out, `"rank": 1`) {
		t.Fatalf("search json: %v\n%s", err, out)
	}

	out, _, err = execute(t, "trips", "--score", "--config", cfg)
	if err != nil || !strings.Contains(out, "POINTS LOST") || !strings.Contains(out, "trend") {
		t.Fatalf("score: %v\n%s", err, out)
	}
	if _, _, err := execute(t, "report", "999", "--config", cfg); err == nil {
		t.Error("report for a missing event should fail")
	}
	if _, _, err := execute(t, "export", "999", "--blur", "--config", cfg); err == nil {
		t.Error("export for a missing event should fail")
	}
}

func TestSlug(t *testing.T) {
	if s := slug("A red truck, at NIGHT!"); s != "a-red-truck-at-night" {
		t.Errorf("slug %q", s)
	}
}

func TestEveryCommandHasHelp(t *testing.T) {
	root := newRootCmd(nil, nil)
	for _, c := range root.Commands() {
		if c.Short == "" {
			t.Errorf("%s has no short help", c.Name())
		}
		for _, s := range c.Commands() {
			if s.Short == "" {
				t.Errorf("%s %s has no short help", c.Name(), s.Name())
			}
		}
	}
}
