package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestActivityDebugPreferenceDefaultsAndPersists(t *testing.T) {
	for _, schema := range []int{1, 2} {
		home := t.TempDir()
		path := filepath.Join(home, "display-settings.json")
		if err := os.WriteFile(path, fmt.Appendf(nil, `{"schema_version":%d,"revision":7,"chatgpt_mcp_ui_enabled":true}`, schema), 0600); err != nil {
			t.Fatal(err)
		}
		s := NewDisplayPreferences(home, false)
		if s.Snapshot().ActivityFullPayloadDebug || s.Snapshot().Warning != "" {
			t.Fatal("legacy settings enabled debug capture")
		}
		called := 0
		s.Subscribe(func() { called++ })
		debug := true
		next, err := s.Update(t.Context(), DisplayChange{ExpectedRevision: 7, ActivityFullPayloadDebug: &debug})
		if err != nil || !next.ActivityFullPayloadDebug || next.Revision != 8 || called != 1 {
			t.Fatalf("debug save failed: %+v %v", next, err)
		}
		if !NewDisplayPreferences(home, false).Snapshot().ActivityFullPayloadDebug {
			t.Fatal("debug setting did not survive restart")
		}
		if _, err := s.Update(t.Context(), DisplayChange{ExpectedRevision: 8, ActivityFullPayloadDebug: &debug}); err != nil || called != 1 {
			t.Fatal("unchanged debug setting caused another notification", err)
		}
		debug = false
		if _, err := s.Update(t.Context(), DisplayChange{ExpectedRevision: 7, ActivityFullPayloadDebug: &debug}); !errors.Is(err, ErrDisplayRevision) {
			t.Fatal("stale debug write accepted", err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if _, err := s.Update(ctx, DisplayChange{ExpectedRevision: 8, ActivityFullPayloadDebug: &debug}); !errors.Is(err, context.Canceled) || !s.Snapshot().ActivityFullPayloadDebug {
			t.Fatal("cancelled save changed debug policy", err)
		}
		if _, err := s.Update(t.Context(), DisplayChange{ExpectedRevision: 8, ActivityFullPayloadDebug: &debug}); err != nil || NewDisplayPreferences(home, true).Snapshot().ActivityFullPayloadDebug {
			t.Fatal("debug disable did not persist", err)
		}
	}
}

func TestActivityDebugPreferenceRejectsCorruptAndFailedWrites(t *testing.T) {
	for _, value := range []string{`null`, `"true"`, `1`, `{}`} {
		home := t.TempDir()
		path := filepath.Join(home, "display-settings.json")
		data := fmt.Appendf(nil, `{"schema_version":2,"revision":1,"chatgpt_mcp_ui_enabled":true,"activity_full_payload_debug":%s}`, value)
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		s := NewDisplayPreferences(home, true)
		if s.Snapshot().ActivityFullPayloadDebug || s.Snapshot().Warning == "" {
			t.Fatalf("invalid debug value accepted: %s", value)
		}
		debug := true
		if _, err := s.Update(t.Context(), DisplayChange{ExpectedRevision: 1, ActivityFullPayloadDebug: &debug}); err == nil {
			t.Fatal("corrupt preference was overwritten")
		}
		actual, err := os.ReadFile(path)
		if err != nil || string(actual) != string(data) {
			t.Fatal("original corrupt preference was not preserved", err)
		}
	}
	home := t.TempDir()
	s := NewDisplayPreferences(home, true)
	if err := os.Mkdir(filepath.Join(home, "display-settings.json"), 0700); err != nil {
		t.Fatal(err)
	}
	debug := true
	if _, err := s.Update(t.Context(), DisplayChange{ExpectedRevision: 1, ActivityFullPayloadDebug: &debug}); err == nil || s.Snapshot().ActivityFullPayloadDebug || s.Snapshot().Revision != 1 {
		t.Fatal("failed write enabled debug capture", err)
	}
}
