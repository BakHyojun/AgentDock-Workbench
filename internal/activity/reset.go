package activity

import (
	"context"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"
)

type ResetResult struct {
	LatestSeq             uint64 `json:"latest_seq"`
	RemovedSegments       int    `json:"removed_segments"`
	RemovedPayloads       int    `json:"removed_payloads"`
	ReclaimedPayloadBytes int64  `json:"reclaimed_payload_bytes"`
	RemainingPayloadBytes int64  `json:"remaining_payload_bytes"`
	ProtectedPayloadBytes int64  `json:"protected_payload_bytes"`
	CleanupIncomplete     bool   `json:"cleanup_incomplete"`
}

// Reset is a local maintenance operation, never a tool call. The runtime must
// exclude executions first. Cross-process locks use the existing payload ->
// journal order. Recent blobs retain publication grace, including reused hashes.
// A durable replay floor commits the reset BEFORE any old file is removed.
// Sequence IDs are never reused, so cached readers and SSE cursors recover.
func (s *Store) Reset(ctx context.Context, now time.Time) (ResetResult, error) {
	return s.resetUsing(ctx, now, os.Remove)
}

// Request-local deletion seam keeps partial cleanup tests deterministic without
// production fault switches or environment-dependent filesystem permissions.
func (s *Store) resetUsing(ctx context.Context, now time.Time, removeFile func(string) error) (ResetResult, error) {
	var result ResetResult
	releasePayload, err := s.lockPayload(ctx)
	if err != nil {
		return result, err
	}
	defer releasePayload()
	release, err := s.lock(ctx)
	if err != nil {
		return result, err
	}
	defer release()
	if err = rejectPayloadLink(s.root); err != nil {
		return result, err
	}
	files, err := s.segments()
	if err != nil {
		return result, err
	}
	state, err := s.state(files)
	if err != nil {
		return result, err
	}
	if state.Seq == ^uint64(0) {
		return result, errors.New("activity sequence exhausted")
	}
	management := filepath.Join(s.root, "call-management.json")
	if err = regularPath(management, false); err != nil {
		return result, err
	}
	if err = regularPath(filepath.Join(s.root, "payload-usage.json"), false); err != nil {
		return result, err
	}
	root := filepath.Join(s.root, "payloads")
	type blob struct {
		path      string
		bytes     int64
		protected bool
	}
	blobs := []blob{}
	if _, err = os.Lstat(root); err == nil {
		if err = rejectPayloadLink(root); err != nil {
			return result, err
		}
		directory, err := os.Open(root)
		if err != nil {
			return result, err
		}
		defer directory.Close()
		for {
			if err = ctx.Err(); err != nil {
				return result, err
			}
			entries, readErr := directory.ReadDir(256)
			for _, entry := range entries {
				// Never delete arbitrary files, subdirectories or linked targets.
				name := entry.Name()
				if filepath.Ext(name) != ".json" {
					continue
				}
				if len(name) != 69 {
					return result, errors.New("invalid payload name")
				}
				if _, err = hex.DecodeString(name[:64]); err != nil {
					return result, errors.New("invalid payload name")
				}
				info, err := entry.Info()
				if err != nil {
					return result, err
				}
				if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
					return result, errors.New("invalid payload entry")
				}
				if len(blobs) == 262144 {
					return result, errors.New("activity reset inventory exceeds safety limit")
				}
				protected := !info.ModTime().Before(now.Add(-payloadPublicationGrace))
				blobs = append(blobs, blob{filepath.Join(root, name), info.Size(), protected})
				result.RemainingPayloadBytes += info.Size()
				if protected {
					result.ProtectedPayloadBytes += info.Size()
				}
			}
			if readErr != nil {
				if errors.Is(readErr, io.EOF) {
					break
				}
				return ResetResult{}, readErr
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return result, err
	}
	if err = ctx.Err(); err != nil {
		return result, err
	}
	// Preflight the accounting write; rejected writes cannot delete history.
	if err = s.savePayloadUsageLocked(result.RemainingPayloadBytes); err != nil {
		return result, err
	}
	state.Seq++
	state.PrunedThrough = state.Seq
	if err = s.saveState(state); err != nil {
		return result, err
	}
	result.LatestSeq = state.Seq
	s.projection = nil
	close(s.changed)
	s.changed = make(chan struct{})
	// From here a reset has committed. Report cleanup separately rather than
	// claiming that a cancellation or failed delete restored the old records.
	remove := func(path string) bool {
		if ctx.Err() != nil {
			result.CleanupIncomplete = true
			return false
		}
		if err := regularPath(path, false); err != nil {
			result.CleanupIncomplete = true
			return false
		}
		if err := removeFile(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			result.CleanupIncomplete = true
			return false
		}
		return true
	}
	for _, path := range files {
		if remove(path) {
			result.RemovedSegments++
		}
	}
	remove(management)
	for _, blob := range blobs {
		if !blob.protected && remove(blob.path) {
			result.RemovedPayloads++
			result.ReclaimedPayloadBytes += blob.bytes
			result.RemainingPayloadBytes -= blob.bytes
		}
	}
	// Invalidate old reservations even if some deletions failed. If this atomic
	// write fails, the preflight accounting conservatively overcounts until GC.
	if err = s.savePayloadUsageLocked(result.RemainingPayloadBytes); err != nil {
		result.CleanupIncomplete = true
	}
	return result, nil
}
