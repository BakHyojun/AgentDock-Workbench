package activity

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func resetFixture(t *testing.T, options Options) (*Store, *Payload, uint64, time.Time) {
	t.Helper()
	s := testStore(t, options)
	p := s.CapturePayload(t.Context(), map[string]any{"output": strings.Repeat("history", 1000)}, "complete", NewRedactor())
	if p.Ref == "" {
		t.Fatal(p)
	}
	savePayloadCall(t, s, "call_reset", p)
	if _, err := s.Append(t.Context(), Event{Binding: Binding{CallID: "call_reset"}, Kind: "call.completed", Status: "succeeded"}); err != nil {
		t.Fatal(err)
	}
	page, err := s.Calls(t.Context(), CallQuery{View: "all"})
	if err != nil {
		t.Fatal(err)
	}
	return s, p, page.LatestSeq, time.Now().Add(payloadPublicationGrace + time.Second)
}

func TestActivityResetReclaimsHistoryAndPreservesMonotonicReplay(t *testing.T) {
	s, p, before, now := resetFixture(t, Options{SegmentBytes: 512})
	if err := s.ManageCall(t.Context(), "call_reset", MetadataChange{Action: "archive"}); err != nil {
		t.Fatal(err)
	}
	other, err := New(s.root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = other.Calls(t.Context(), CallQuery{View: "all"}); err != nil {
		t.Fatal(err)
	}
	changed := s.Changed()
	result, err := s.Reset(t.Context(), now)
	if err != nil || result.CleanupIncomplete || result.ReclaimedPayloadBytes != p.Bytes || result.RemainingPayloadBytes != 0 || result.LatestSeq <= before {
		t.Fatalf("reset: %+v %v", result, err)
	}
	select {
	case <-changed:
	default:
		t.Fatal("reset did not notify observers")
	}
	for _, store := range []*Store{s, other} {
		page, err := store.Calls(t.Context(), CallQuery{After: before, Updates: true, View: "all"})
		if err != nil || len(page.Calls) != 0 || !page.Gap || page.LatestSeq != result.LatestSeq {
			t.Fatalf("cached history returned after reset: %+v %v", page, err)
		}
		if _, err := store.Call(t.Context(), "call_reset"); !errors.Is(err, ErrCallNotFound) {
			t.Fatal("old detail reopened", err)
		}
	}
	if _, err := os.Stat(filepath.Join(s.root, "call-management.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("old management overlay survived", err)
	}
	newEvent, err := other.Append(t.Context(), Event{Binding: Binding{CallID: "call_after_reset"}, Kind: "call.created", Status: "created"})
	if err != nil || newEvent.Seq <= result.LatestSeq {
		t.Fatal("sequence reused", newEvent, err)
	}
	fresh, err := New(s.root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	page, err := fresh.Query(t.Context(), Query{After: before})
	if err != nil || !page.Gap || len(page.Events) != 1 || page.Events[0].CallID != "call_after_reset" {
		t.Fatal("replay floor failed", page, err)
	}
	data, err := os.ReadFile(filepath.Join(s.root, "payload-usage.json"))
	if err != nil {
		t.Fatal(err)
	}
	var usage struct {
		Bytes int64 `json:"bytes"`
	}
	if err := json.Unmarshal(data, &usage); err != nil || usage.Bytes != 0 {
		t.Fatal("quota was not reclaimed", string(data), err)
	}
}

func TestActivityResetProtectsPublicationAndReusedHashes(t *testing.T) {
	s, p, _, _ := resetFixture(t, Options{})
	path := filepath.Join(s.root, "payloads", p.Ref+".json")
	now := time.Now()
	old := now.Add(-2 * payloadPublicationGrace)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	other, err := New(s.root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	reused := other.CapturePayload(t.Context(), map[string]any{"output": strings.Repeat("history", 1000)}, "complete", NewRedactor())
	if reused.Ref != p.Ref {
		t.Fatal("fixture did not reuse hash")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.Reset(t.Context(), info.ModTime().Add(payloadPublicationGrace))
	if err != nil || result.RemainingPayloadBytes != p.Bytes || result.ProtectedPayloadBytes != p.Bytes {
		t.Fatal("boundary publication deleted", result, err)
	}
	// Publish after another Store resets: its captured source must still exist.
	savePayloadCall(t, other, "call_published_after_reset", reused)
	if _, err := other.ReadCallPayload(t.Context(), "call_published_after_reset", "response", 0, 100); err != nil {
		t.Fatal(err)
	}
	result, err = s.Reset(t.Context(), info.ModTime().Add(payloadPublicationGrace+time.Nanosecond))
	if err != nil || result.RemainingPayloadBytes != 0 || result.ReclaimedPayloadBytes != p.Bytes {
		t.Fatal("expired grace not reclaimed", result, err)
	}
}

func TestActivityResetRejectedWritesAndCancellationPreserveHistory(t *testing.T) {
	for _, failure := range []string{"cancelled", "usage", "sequence"} {
		t.Run(failure, func(t *testing.T) {
			s, p, _, now := resetFixture(t, Options{})
			ctx := t.Context()
			if failure == "cancelled" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			} else {
				path := filepath.Join(s.root, map[string]string{"usage": "payload-usage.json", "sequence": "sequence.json"}[failure])
				if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
					t.Fatal(err)
				}
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := s.Reset(ctx, now); err == nil {
				t.Fatal("rejected reset reported success")
			}
			if _, err := os.Stat(filepath.Join(s.root, "payloads", p.Ref+".json")); err != nil {
				t.Fatal("rejected reset deleted blob", err)
			}
			files, err := s.segments()
			if err != nil || len(files) == 0 {
				t.Fatal("rejected reset deleted journal", files, err)
			}
		})
	}
}

func TestActivityResetPartialCleanupNeverResurrectsOldRecords(t *testing.T) {
	s, p, _, now := resetFixture(t, Options{SegmentBytes: 512, Segments: 8})
	result, err := s.resetUsing(t.Context(), now, func(path string) error {
		if strings.HasSuffix(path, ".jsonl") || strings.Contains(path, "payloads") {
			return os.ErrPermission
		}
		return os.Remove(path)
	})
	if err != nil || !result.CleanupIncomplete || result.RemainingPayloadBytes != p.Bytes || result.ReclaimedPayloadBytes != 0 {
		t.Fatal(result, err)
	}
	// Rotation over undeleted old segments must never lower the reset floor.
	for i := 0; i < 20; i++ {
		if _, err := s.Append(t.Context(), Event{Kind: "activity.test", Summary: strings.Repeat("x", 400)}); err != nil {
			t.Fatal(err)
		}
	}
	fresh, err := New(s.root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fresh.Call(t.Context(), "call_reset"); !errors.Is(err, ErrCallNotFound) {
		t.Fatal("partial cleanup resurrected history", err)
	}
	result, err = s.Reset(t.Context(), now)
	if err != nil || result.CleanupIncomplete || result.RemainingPayloadBytes != 0 {
		t.Fatal("cleanup retry failed", result, err)
	}
}

func TestActivityResetRejectsLinkedPayload(t *testing.T) {
	s, _, _, now := resetFixture(t, Options{})
	out := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(out, []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(s.root, "payloads", strings.Repeat("a", 64)+".json")
	if err := os.Symlink(out, link); err != nil {
		t.Skip("fixture requires symlink support:", err)
	}
	if _, err := s.Reset(t.Context(), now); err == nil {
		t.Fatal("linked payload accepted")
	}
	data, err := os.ReadFile(out)
	if err != nil || string(data) != "outside" {
		t.Fatal("linked target changed", err)
	}
}

func TestActivityResetPreservesUnrelatedFilesAndHandlesEmptyRetry(t *testing.T) {
	s, _, _, now := resetFixture(t, Options{})
	sentinel := filepath.Join(s.root, "user-note.txt")
	if err := os.WriteFile(sentinel, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Reset(t.Context(), now); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(sentinel)
	if err != nil || string(data) != "keep" {
		t.Fatal("reset crossed its scope", err)
	}
	result, err := s.Reset(t.Context(), now)
	if err != nil || result.RemainingPayloadBytes != 0 || result.RemovedSegments != 0 || result.CleanupIncomplete {
		t.Fatal("empty reset retry failed", result, err)
	}
}

func TestActivityResetCancellationAfterCommitReportsPartialCleanup(t *testing.T) {
	s, p, before, now := resetFixture(t, Options{SegmentBytes: 512})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result, err := s.resetUsing(ctx, now, func(path string) error { cancel(); return os.Remove(path) })
	if err != nil || !result.CleanupIncomplete || result.LatestSeq <= before || result.RemainingPayloadBytes != p.Bytes {
		t.Fatal("committed reset was disguised as a rollback", result, err)
	}
	fresh, err := New(s.root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fresh.Call(t.Context(), "call_reset"); !errors.Is(err, ErrCallNotFound) {
		t.Fatal("cancelled cleanup resurrected history", err)
	}
}
