package activity

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestHistoryPayloadBoundaryUsesRedactedFormattedBytes(t *testing.T) {
	for _, size := range []int{MaxHistoryFullPayloadBytes - 1, MaxHistoryFullPayloadBytes, MaxHistoryFullPayloadBytes + 1} {
		s := testStore(t, Options{})
		p := s.CaptureHistoryPayload(t.Context(), strings.Repeat("x", size-2), "complete", NewRedactor(), false)
		if p.Bytes != int64(size) {
			t.Fatalf("size=%d: descriptor bytes=%d", size, p.Bytes)
		}
		if size <= MaxHistoryFullPayloadBytes {
			if p.State != "complete" || p.Ref == "" {
				t.Fatalf("boundary detail not saved: %+v", p)
			}
		} else if p.State != "preview_only" || p.Ref != "" || !p.Truncated || len(p.Preview) != PayloadPreviewBytes {
			t.Fatalf("over-boundary detail: %+v", p)
		}
	}
	s := testStore(t, Options{})
	// Credential and binary fields are reduced before applying the history cap.
	secret := strings.Repeat("secret", 50000)
	p := s.CaptureHistoryPayload(t.Context(), map[string]any{"password": secret, "content": []any{map[string]any{"type": "image", "data": secret}}}, "complete", NewRedactor(), false)
	if p.State != "complete" || p.Ref == "" || strings.Contains(p.Preview, "secret") {
		t.Fatalf("threshold was applied before redaction: %+v", p)
	}
	// Compact JSON fits; indentation expands it past the history threshold.
	value := map[string]any{"text": strings.Repeat("x", MaxHistoryFullPayloadBytes-14)}
	compact, _ := json.Marshal(value)
	if len(compact) > MaxHistoryFullPayloadBytes {
		t.Fatal("invalid compact boundary fixture")
	}
	p = s.CaptureHistoryPayload(t.Context(), value, "complete", NewRedactor(), false)
	if p.State != "preview_only" || p.Bytes <= MaxHistoryFullPayloadBytes {
		t.Fatalf("formatted bytes did not determine policy: %+v", p)
	}
}

func TestHistoryPreviewSkipsBlobIOAndSurvivesReplay(t *testing.T) {
	s := testStore(t, Options{})
	// A blocked blob directory and corrupt ledger must not affect optional preview.
	if err := os.WriteFile(filepath.Join(s.root, "payloads"), []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	usage := filepath.Join(s.root, "payload-usage.json")
	if err := os.WriteFile(usage, []byte("corrupt ledger"), 0600); err != nil {
		t.Fatal(err)
	}
	value := map[string]any{"password": "private-fixture", "text": strings.Repeat("中🙂\n", 300000)}
	for i := 0; i < 4; i++ {
		p := s.CaptureHistoryPayload(t.Context(), value, "complete", NewRedactor(), false)
		if p.State != "preview_only" || p.Ref != "" || p.Bytes < 2<<20 || p.Lines <= 0 || !utf8.ValidString(p.Preview) || len(p.Preview) > PayloadPreviewBytes || strings.Contains(p.Preview, "private-fixture") {
			t.Fatalf("large preview invalid: %+v", p)
		}
		savePayloadCall(t, s, "call_preview", p)
	}
	data, err := os.ReadFile(usage)
	if err != nil || string(data) != "corrupt ledger" {
		t.Fatal("preview capture touched accounting", err)
	}
	replay, err := New(s.root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	page, err := replay.ReadCallPayload(t.Context(), "call_preview", "response", 0, 32768)
	if err != nil || page.Payload.State != "preview_only" || page.Text == "" || page.HasMore {
		t.Fatalf("preview did not survive journal replay: %+v %v", page, err)
	}
	if _, err := replay.ReadCallPayload(t.Context(), "call_preview", "response", 1, 32768); err == nil {
		t.Fatal("preview fabricated additional pages")
	}
}

func TestHistoryDebugAndContinuationKeepFullCaptureLimits(t *testing.T) {
	s := testStore(t, Options{})
	value := strings.Repeat("x", MaxHistoryFullPayloadBytes+1)
	debug := s.CaptureHistoryPayload(t.Context(), value, "complete", NewRedactor(), true)
	if debug.State != "complete" || debug.Ref == "" {
		t.Fatalf("debug did not capture full result: %+v", debug)
	}
	savePayloadCall(t, s, "call_debug", debug)
	preview := s.CaptureHistoryPayload(t.Context(), value, "complete", NewRedactor(), false)
	if preview.State != "preview_only" || preview.Ref != "" {
		t.Fatal("normal mode reused a previously captured large blob")
	}
	source := s.CapturePayload(t.Context(), value, "partial", NewRedactor())
	if source.Ref != debug.Ref || source.State != "partial" {
		t.Fatal("normal history policy changed continuation capture")
	}
	page, err := s.ReadCallPayload(t.Context(), "call_debug", "response", 0, 32768)
	if err != nil || page.Text == "" || !page.HasMore {
		t.Fatal("switching policy removed old debug detail", err)
	}
	oversize := strings.Repeat("x", MaxPayloadBytes)
	for _, fullDebug := range []bool{false, true} {
		p := s.CaptureHistoryPayload(t.Context(), oversize, "complete", NewRedactor(), fullDebug)
		if p.State != "not_stored" || p.Ref != "" {
			t.Fatal("history capture bypassed 16 MiB safety limit")
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if p := s.CaptureHistoryPayload(ctx, value, "complete", NewRedactor(), false); p.State != "not_stored" {
		t.Fatal("cancelled capture reported successful preview")
	}
}
