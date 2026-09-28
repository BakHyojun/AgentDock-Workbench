package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeBundledFixture(t *testing.T, root, name, version string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Join(dir, "skills", name+"-guide"), 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"` + name + `","version":"` + version + `","description":"bundled fixture"}`
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	skill := "---\nname: " + name + "-guide\ndescription: Bundled fixture guide.\n---\n# Guide\n"
	if err := os.WriteFile(filepath.Join(dir, "skills", name+"-guide", "SKILL.md"), []byte(skill), 0o600); err != nil {
		t.Fatal(err)
	}
}

func provisioned(t *testing.T, store *Store, bundle string) map[string]BundledResult {
	t.Helper()
	results, err := store.ProvisionBundled(bundle)
	if err != nil {
		t.Fatalf("provision failed: %v %+v", err, results)
	}
	byName := map[string]BundledResult{}
	for _, result := range results {
		byName[result.Name] = result
	}
	return byName
}

func TestBundledPluginsFollowTheUserChoice(t *testing.T) {
	bundle := t.TempDir()
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	writeBundledFixture(t, bundle, "desk", "1.0.0")

	if got := provisioned(t, store, bundle)["desk"]; got.Action != BundledInstalled {
		t.Fatalf("first provision = %+v", got)
	}
	if definition, err := store.Get("desk"); err != nil || !definition.Enabled || definition.Version != "1.0.0" {
		t.Fatalf("bundled plugin not installed enabled: %+v %v", definition, err)
	}
	if got := provisioned(t, store, bundle)["desk"]; got.Action != BundledCurrent {
		t.Fatalf("unchanged bundle = %+v", got)
	}

	// A newer bundle updates the files but keeps the user's switch.
	if _, err := store.SetEnabled("desk", false); err != nil {
		t.Fatal(err)
	}
	writeBundledFixture(t, bundle, "desk", "1.1.0")
	if got := provisioned(t, store, bundle)["desk"]; got.Action != BundledUpdated {
		t.Fatalf("newer bundle = %+v", got)
	}
	if definition, err := store.Get("desk"); err != nil || definition.Enabled || definition.Version != "1.1.0" {
		t.Fatalf("update lost the disabled state or version: %+v %v", definition, err)
	}

	// Once the user removes a plugin Setup provided, Setup leaves it removed.
	if err := store.Remove("desk"); err != nil {
		t.Fatal(err)
	}
	if got := provisioned(t, store, bundle)["desk"]; got.Action != BundledUserRemoved {
		t.Fatalf("removed plugin = %+v", got)
	}
	if _, err := store.Get("desk"); err == nil {
		t.Fatal("Setup reinstalled a plugin the user removed")
	}
}

func TestBundledPluginNeverReplacesAUserInstalledNamesake(t *testing.T) {
	user := t.TempDir()
	writeBundledFixture(t, user, "desk", "0.9.0")
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Install(filepath.Join(user, "desk"), false); err != nil {
		t.Fatal(err)
	}
	bundle := t.TempDir()
	writeBundledFixture(t, bundle, "desk", "1.0.0")
	if got := provisioned(t, store, bundle)["desk"]; got.Action != BundledUserOwned {
		t.Fatalf("user namesake = %+v", got)
	}
	if definition, err := store.Get("desk"); err != nil || definition.Version != "0.9.0" {
		t.Fatalf("user plugin replaced: %+v %v", definition, err)
	}
}

func TestBundledPluginFailureDoesNotStopOthers(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if results, err := store.ProvisionBundled(filepath.Join(t.TempDir(), "absent")); err != nil || len(results) != 0 {
		t.Fatalf("payload without plugins = %+v %v", results, err)
	}
	bundle := t.TempDir()
	writeBundledFixture(t, bundle, "desk", "1.0.0")
	if err := os.MkdirAll(filepath.Join(bundle, "broken"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundle, "broken", "plugin.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	results, err := store.ProvisionBundled(bundle)
	if err == nil {
		t.Fatal("broken bundled plugin was not reported")
	}
	actions := map[string]string{}
	for _, result := range results {
		actions[result.Name] = result.Action
	}
	if actions["broken"] != BundledFailed || actions["desk"] != BundledInstalled {
		t.Fatalf("bundle results = %+v", results)
	}
	if _, err := store.Get("desk"); err != nil {
		t.Fatal("a broken sibling blocked a valid bundled plugin")
	}
	data, err := os.ReadFile(filepath.Join(store.root, bundledRecordFile))
	if err != nil || !strings.Contains(string(data), `"desk"`) || strings.Contains(string(data), `"broken"`) {
		t.Fatalf("bundled record = %s %v", data, err)
	}
}
