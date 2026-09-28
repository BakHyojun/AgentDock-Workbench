//go:build windows && amd64 && bundled_rg_integration

package selfupdate

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/uvwt/agentdock/internal/bundledrg"
)

func componentArchive(t *testing.T, mutation string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	add := func(name string, data []byte) {
		t.Helper()
		file, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	add("agentdock-tray.exe", []byte("inert tray"))
	add("agentdock.ico", []byte("inert icon"))
	add("unrelated-file.txt", []byte("must not be extracted"))
	if mutation != "absent" {
		source := os.Getenv("AGENTDOCK_TEST_RG_BUNDLE")
		if source == "" {
			t.Fatal("verified component fixture is required")
		}
		add(bundledrg.RelativeDir+"/manifest.json", bundledrg.ManifestBytes())
		for _, expected := range bundledrg.Specification().Files {
			if mutation == "missing-licence" && expected.Path == "LICENSE-MIT" {
				continue
			}
			data, err := os.ReadFile(filepath.Join(source, expected.Path))
			if err != nil {
				t.Fatal(err)
			}
			if mutation == "tampered" && expected.Path == "rg.exe" {
				data[len(data)-1] ^= 1
			}
			add(bundledrg.RelativeDir+"/"+expected.Path, data)
		}
		if mutation == "duplicate" {
			add(bundledrg.RelativeDir+"/manifest.json", bundledrg.ManifestBytes())
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestComponentArchiveVerifiedExtractionAndAllowlist(t *testing.T) {
	root, err := extractDesktopUpdateArchive(t.Context(), componentArchive(t, "valid"), t.TempDir(), "v1.1.7")
	if err != nil {
		t.Fatal(err)
	}
	present, err := bundledrg.VerifyIfPresent(t.Context(), root)
	if err != nil || !present {
		t.Fatalf("verified bundle lost: %v %v", present, err)
	}
	if _, err := os.Stat(filepath.Join(root, "unrelated-file.txt")); !os.IsNotExist(err) {
		t.Fatalf("archive allowlist widened: %v", err)
	}
}

func TestComponentArchiveRejectsIncompleteTamperedAndDuplicate(t *testing.T) {
	for _, mutation := range []string{"missing-licence", "tampered", "duplicate"} {
		t.Run(mutation, func(t *testing.T) {
			if root, err := extractDesktopUpdateArchive(t.Context(), componentArchive(t, mutation), t.TempDir(), "v1.1.7"); err == nil || root != "" {
				t.Fatalf("unverified archive accepted: %s %v", root, err)
			}
		})
	}
}

func TestComponentArchiveLegacyAndCancellation(t *testing.T) {
	archive := componentArchive(t, "absent")
	root, err := extractDesktopUpdateArchive(t.Context(), archive, t.TempDir(), "v1.1.7")
	if err != nil {
		t.Fatal(err)
	}
	present, err := bundledrg.VerifyIfPresent(t.Context(), root)
	if err != nil || present {
		t.Fatalf("legacy absence changed: %v %v", present, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := extractDesktopUpdateArchive(ctx, archive, t.TempDir(), "v1.1.7"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled extraction ignored: %v", err)
	}
}

// The single-pass Release payload used by local-archive updates must carry the
// same verified component into the generation staging directory.
func TestReleasePayloadCarriesVerifiedBundle(t *testing.T) {
	source := os.Getenv("AGENTDOCK_TEST_RG_BUNDLE")
	if source == "" {
		t.Fatal("verified component fixture is required")
	}
	for _, mutation := range []string{"valid", "tampered"} {
		t.Run(mutation, func(t *testing.T) {
			archive := makeWindowsReleaseZIP(t, func(writer *zip.Writer) {
				writeReleaseEntry(t, writer, bundledrg.RelativeDir+"/manifest.json", string(bundledrg.ManifestBytes()))
				for _, expected := range bundledrg.Specification().Files {
					data, err := os.ReadFile(filepath.Join(source, expected.Path))
					if err != nil {
						t.Fatal(err)
					}
					if mutation == "tampered" && expected.Path == "rg.exe" {
						data[len(data)-1] ^= 1
					}
					writeReleaseEntry(t, writer, bundledrg.RelativeDir+"/"+expected.Path, string(data))
				}
			})
			payload, err := extractWindowsReleasePayload(t.Context(), archive, t.TempDir(), "agentdock.exe", "v1.1.8100")
			if mutation == "tampered" {
				if !errors.Is(err, bundledrg.ErrIntegrity) {
					t.Fatalf("tampered bundle accepted: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			present, err := bundledrg.VerifyIfPresent(t.Context(), payload.DesktopPath)
			if err != nil || !present {
				t.Fatalf("verified bundle lost from the release payload: %v %v", present, err)
			}
		})
	}
}
