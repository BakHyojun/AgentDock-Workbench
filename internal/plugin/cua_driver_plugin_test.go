package plugin

import (
	"path/filepath"
	"reflect"
	"testing"
)

// The shipped CUA plugin must stay a valid Agent Plugins package whose MCP
// member attaches to the user's cua-driver daemon, so element caches, sessions
// and elevation live in the daemon instead of a child that AgentDock recycles.
func TestShippedCuaDriverPluginAttachesToTheDaemon(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "plugins", "cua-driver"))
	if err != nil {
		t.Fatal(err)
	}
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	definition, err := store.Validate(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(definition.Diagnostics) != 0 || definition.Heavy ||
		!reflect.DeepEqual(definition.Skills, []string{"cua-desktop"}) ||
		!reflect.DeepEqual(definition.MCPServers, []string{"cua-driver"}) {
		t.Fatalf("shipped CUA plugin changed: %+v", definition)
	}
	configs, diagnostics := readMCP(root, definition.Name)
	server, ok := configs["cua-driver"]
	if len(diagnostics) != 0 || !ok || server.Name != "cua-driver" || server.Command != "cua-driver" ||
		!reflect.DeepEqual(server.Args, []string{"mcp", "--socket", `\\.\pipe\cua-driver`}) {
		t.Fatalf("CUA MCP member does not attach to the daemon pipe: %+v %v", server, diagnostics)
	}
}
