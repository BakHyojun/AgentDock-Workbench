package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
)

// Setup provisions the shipped plugins with this command after a committed
// install; a repeated run reports them as current instead of reinstalling.
func TestPluginBootstrapProvisionsShippedPlugins(t *testing.T) {
	bundle, err := filepath.Abs(filepath.Join("..", "..", "plugins"))
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	actions := func() map[string]string {
		t.Helper()
		var stdout bytes.Buffer
		if err := run(context.Background(), []string{"plugin", "bootstrap", "--bundle", bundle, "--home", home}, &stdout, &bytes.Buffer{}); err != nil {
			t.Fatalf("bootstrap failed: %v %s", err, stdout.String())
		}
		var output struct {
			BundledPlugins []struct {
				Name   string `json:"name"`
				Action string `json:"action"`
			} `json:"bundled_plugins"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
			t.Fatalf("bootstrap output is not JSON: %q", stdout.String())
		}
		result := map[string]string{}
		for _, item := range output.BundledPlugins {
			result[item.Name] = item.Action
		}
		return result
	}
	if got := actions(); got["cua-driver"] != "installed" {
		t.Fatalf("first bootstrap = %v", got)
	}
	if got := actions(); got["cua-driver"] != "current" {
		t.Fatalf("repeated bootstrap = %v", got)
	}
	if err := run(context.Background(), []string{"plugin", "bootstrap", "--bundle", "plugins", "--home", home}, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
		t.Fatal("relative bundle path accepted")
	}
}
