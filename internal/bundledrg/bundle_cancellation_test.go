package bundledrg

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

// Cancellation occurs after Open's initial check, when its manifest reader
// consults the context. No timing race, sleeps or external process is needed.
type manifestCancellationContext struct {
	context.Context
	cancel context.CancelFunc
	checks atomic.Int32
}

func (ctx *manifestCancellationContext) Err() error {
	if ctx.checks.Add(1) == 2 {
		ctx.cancel()
	}
	return ctx.Context.Err()
}

func TestManifestReadCancellationRemainsCancellation(t *testing.T) {
	root := filepath.Join(t.TempDir(), "share", "agentdock", "bin")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(root, "manifest.json")
	if err := os.WriteFile(manifest, ManifestBytes(), 0600); err != nil {
		t.Fatal(err)
	}
	parent, cancel := context.WithCancel(t.Context())
	defer cancel()
	ctx := &manifestCancellationContext{Context: parent, cancel: cancel}
	verified, err := Open(ctx, root)
	if verified != nil {
		verified.Close()
		t.Fatal("cancelled bundle returned an executable")
	}
	if !errors.Is(err, context.Canceled) || errors.Is(err, ErrIntegrity) {
		t.Fatalf("manifest cancellation became an integrity failure: %v", err)
	}
	// On Windows the read handle forbids writes until it is closed.
	file, err := os.OpenFile(manifest, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("cancelled manifest read leaked a handle: %v", err)
	}
	file.Close()
}
