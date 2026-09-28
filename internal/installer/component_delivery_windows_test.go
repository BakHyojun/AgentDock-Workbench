//go:build windows && amd64 && bundled_rg_integration

package installer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/uvwt/agentdock/internal/bundledrg"
)

// Only the existing payload copier is exercised. No installer, service,
// scheduled task or production runtime is started by these tests.
func componentPayload(t *testing.T, withComponent bool) string {
	t.Helper()
	root := t.TempDir()
	for _, name := range []string{"agentdock.exe", "agentdock-tray.exe", "agentdock-arbiter.exe"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("inert payload fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if !withComponent {
		return root
	}
	source := os.Getenv("AGENTDOCK_TEST_RG_BUNDLE")
	if source == "" {
		t.Fatal("verified component fixture is required")
	}
	target := filepath.Join(root, filepath.FromSlash(bundledrg.RelativeDir))
	if err := os.MkdirAll(target, 0700); err != nil {
		t.Fatal(err)
	}
	for _, file := range bundledrg.Specification().Files {
		data, err := os.ReadFile(filepath.Join(source, file.Path))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(target, file.Path), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(target, "manifest.json"), bundledrg.ManifestBytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestComponentDeliveryCopiesVerifiedGenerationPayload(t *testing.T) {
	payload := componentPayload(t, true)
	staging := t.TempDir()
	if err := copyWindowsGenerationPayload(payload, staging); err != nil {
		t.Fatal(err)
	}
	for _, root := range []string{payload, staging} {
		present, err := bundledrg.VerifyIfPresent(t.Context(), root)
		if err != nil || !present {
			t.Fatalf("bundle lost during delivery: present=%v err=%v", present, err)
		}
	}
	for _, name := range []string{"agentdock-core.exe", "agentdock-tray.exe", "agentdock-arbiter.exe"} {
		data, err := os.ReadFile(filepath.Join(staging, name))
		if err != nil || string(data) != "inert payload fixture" {
			t.Fatalf("original payload mapping changed: %s %v", name, err)
		}
	}
}

func TestComponentDeliveryRejectsPartialAndModifiedInput(t *testing.T) {
	for _, mutation := range []string{"manifest", "licence", "executable"} {
		t.Run(mutation, func(t *testing.T) {
			payload := componentPayload(t, true)
			component := filepath.Join(payload, filepath.FromSlash(bundledrg.RelativeDir))
			switch mutation {
			case "manifest":
				if err := os.Remove(filepath.Join(component, "manifest.json")); err != nil {
					t.Fatal(err)
				}
			case "licence":
				if err := os.Remove(filepath.Join(component, "LICENSE-MIT")); err != nil {
					t.Fatal(err)
				}
			case "executable":
				path := filepath.Join(component, "rg.exe")
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				data[len(data)-1] ^= 1
				if err := os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := copyWindowsGenerationPayload(payload, t.TempDir()); !errors.Is(err, bundledrg.ErrIntegrity) {
				t.Fatalf("corrupt component was accepted: %v", err)
			}
		})
	}
}

func TestComponentDeliveryPreservesLegacyAbsence(t *testing.T) {
	staging := t.TempDir()
	if err := copyWindowsGenerationPayload(componentPayload(t, false), staging); err != nil {
		t.Fatal(err)
	}
	present, err := bundledrg.VerifyIfPresent(context.Background(), staging)
	if err != nil || present {
		t.Fatalf("optional legacy component semantics changed: %v %v", present, err)
	}
}
