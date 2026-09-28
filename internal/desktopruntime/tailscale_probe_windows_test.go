//go:build windows

package desktopruntime

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// A public probe that times out after an earlier success must leave pending
// state, like a fast failure, so the next status read cannot report Ready.
func TestFunnelTimeoutDoesNotResurrectReady(t *testing.T) {
	// Status reads local health directly; serve it from a private loopback server.
	health := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer health.Close()
	parsed, err := url.Parse(health.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		t.Fatal(err)
	}
	rt, system := newPublicAccessTestSystem(t, "none")
	rt.manifest.Port = port
	rt.manifest.LocalMCPURL = health.URL + "/mcp"
	if err := Save(filepath.Join(rt.root, "runtime.json"), rt.manifest); err != nil {
		t.Fatal(err)
	}
	if rt, err = loadTunnelRuntime(rt.root); err != nil {
		t.Fatal(err)
	}
	if err := system.configure(rt, true); err != nil {
		t.Fatal(err)
	}
	if rt, err = loadTunnelRuntime(rt.root); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(rt.root, "tailscale.exe")
	if before := inspectTailscaleRuntime(t.Context(), rt, binary, system.fake.client()); !before.Ready {
		t.Fatalf("fixture not verified: %+v", before)
	}
	restarts, mapping := system.restarts, cloneTailscaleServe(system.fake.config)
	hooks := system.hooks
	hooks.verifyOrigin = func(ctx context.Context, _ tunnelRuntime, _ string) error {
		<-ctx.Done()
		return ctx.Err()
	}
	deadline, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	failed, err := verifyConfiguredTailscale(deadline, rt.root, hooks)
	cancel()
	if err != nil || failed.Ready || failed.DiagnosticCode != "public_unreachable" {
		t.Fatalf("timeout result: %+v err=%v", failed, err)
	}
	state, err := loadTailscaleState(rt.root)
	if err != nil || !state.Pending || state.VerifiedAt != nil {
		t.Fatalf("timeout kept earlier verification: %+v %v", state, err)
	}
	if next := inspectTailscaleRuntime(t.Context(), rt, binary, system.fake.client()); next.Ready || next.DiagnosticCode != "verification_pending" {
		t.Fatalf("timeout resurrected readiness: %+v", next)
	}
	if system.restarts != restarts || !sameTailscaleServe(mapping, system.fake.config) {
		t.Fatal("verification changed lifecycle or mapping")
	}
	hooks.verifyOrigin = func(context.Context, tunnelRuntime, string) error { return nil }
	if recovered, err := verifyConfiguredTailscale(t.Context(), rt.root, hooks); err != nil || !recovered.Ready {
		t.Fatalf("successful reverification did not recover: %+v %v", recovered, err)
	}
}

func TestFunnelLocalCommitRetainsPendingWithoutPublicWait(t *testing.T) {
	runtime, system := newPublicAccessTestSystem(t, "none")
	hooks := system.hooks
	hooks.waitForPublic = false
	probes := 0
	hooks.verifyOrigin = func(context.Context, tunnelRuntime, string) error {
		probes++
		return errors.New("unexpected public wait")
	}
	binary := filepath.Join(runtime.root, "tailscale.exe")
	if err := configureTailscaleAccess(t.Context(), runtime, binary, true, hooks); err != nil {
		t.Fatal(err)
	}
	state, err := loadTailscaleState(runtime.root)
	if err != nil || state == nil || !state.Pending || state.VerifiedAt != nil || probes != 0 {
		t.Fatalf("local commit made a public claim: %+v %v probes=%d", state, err, probes)
	}
	current, err := loadTunnelRuntime(runtime.root)
	if err != nil {
		t.Fatal(err)
	}
	beforeRestart := system.restarts
	beforeConfig := cloneTailscaleServe(system.fake.config)
	if err := configureTailscaleAccess(t.Context(), current, binary, true, hooks); err != nil {
		t.Fatal(err)
	}
	if probes != 0 || system.restarts != beforeRestart || !sameTailscaleServe(beforeConfig, system.fake.config) {
		t.Fatal("reusing pending configuration repeated writes or restarted Core")
	}
	hooks.verifyOrigin = func(context.Context, tunnelRuntime, string) error {
		return &tailscaleTransientProbe{message: "DNS not ready"}
	}
	status, err := verifyConfiguredTailscale(t.Context(), runtime.root, hooks)
	if err != nil || status.Ready || !status.LocalReady || status.Phase != "Degraded" {
		t.Fatalf("propagation was reported as ready or destructive failure: %+v %v", status, err)
	}
	if !sameTailscaleServe(beforeConfig, system.fake.config) || system.restarts != beforeRestart {
		t.Fatal("public verification changed mapping or restarted Core")
	}
	hooks.verifyOrigin = func(context.Context, tunnelRuntime, string) error { return nil }
	status, err = verifyConfiguredTailscale(t.Context(), runtime.root, hooks)
	if err != nil || !status.Ready || status.VerifiedAt == nil {
		t.Fatalf("verified configuration was not committed: %+v %v", status, err)
	}
}
