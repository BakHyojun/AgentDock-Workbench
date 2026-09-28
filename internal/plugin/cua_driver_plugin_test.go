package plugin

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

// A cua-driver update leaves the old daemon running; the agent must hand the
// restart to the user and reconnect, never spawn a private daemon.
func TestShippedCuaDriverSkillHandsDaemonRecoveryToTheUser(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "plugins", "cua-driver", "skills", "cua-desktop", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	skill := strings.Join(strings.Fields(string(data)), " ")
	for _, want := range []string{
		"Do not start, stop or replace cua-driver processes or daemons through a shell",
		"`cua-driver stop` and then `cua-driver autostart kick`",
		"`{\"action\":\"refresh\",\"name\":\"cua-driver\"}`",
	} {
		if !strings.Contains(skill, want) {
			t.Errorf("CUA skill lost daemon recovery guidance: %s", want)
		}
	}
}
