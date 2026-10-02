package httpx

import (
	"encoding/json"
	"testing"

	"github.com/uvwt/agentdock/internal/activity"
)

func TestManagementLocalizationUsesActualHTTPAndReservedPersistence(t *testing.T) {
	f := newExecutionHTTPFixture(t)
	current, status := f.request(t, "GET", "/internal/runtime/permissions/effective", nil)
	if status != 200 {
		t.Fatal(status, current)
	}
	policy := current["policy"].(map[string]any)
	result, status := f.request(t, "POST", "/internal/runtime/permissions", map[string]any{"scope": "global", "mode": "readonly", "expected_revision": policy["revision"]})
	if status != 200 {
		t.Fatal(status, result)
	}
	page, status := f.request(t, "GET", "/internal/runtime/calls?unattributed=true&view=all", nil)
	if status != 200 {
		t.Fatal(status, page)
	}
	found := false
	for _, raw := range page["calls"].([]any) {
		call := raw.(map[string]any)
		if call["tool_name"] != "permission.update" {
			continue
		}
		found = true
		if call["title"] != "修改执行权限" || call["title_text"] == nil || call["summary_text"] == nil {
			t.Fatal("HTTP missed reserved descriptors", call)
		}
		id := call["call_id"].(string)
		journal, err := f.runtime.ActivityJournal().Query(t.Context(), activity.Query{CallID: id})
		if err != nil || len(journal.Events) != 3 || journal.Events[0].TitleText == nil || journal.Events[2].SummaryText == nil {
			t.Fatal("not persisted", journal, err)
		}
	}
	if !found {
		t.Fatal("no permission update call")
	}

	id := "call_aabbccddeeff00112233445566778899"
	raw := activity.Event{Binding: activity.Binding{CallID: id}, Kind: "call.completed", ToolName: "permission.update", Title: "修改执行权限", Status: "succeeded", Summary: "scope=global scope_id= mode=rules revision=1；操作系统权限未改变。"}
	if _, err := f.runtime.ActivityJournal().Append(t.Context(), raw); err != nil {
		t.Fatal(err)
	}
	before, err := f.runtime.ActivityJournal().Query(t.Context(), activity.Query{CallID: id})
	if err != nil {
		t.Fatal(err)
	}
	original, _ := json.Marshal(before)
	detail, status := f.request(t, "GET", "/internal/runtime/calls/"+id, nil)
	if status != 200 || detail["title"] != raw.Title || detail["title_text"] == nil || detail["summary_text"] == nil {
		t.Fatal("old HTTP detail", status, detail)
	}
	after, err := f.runtime.ActivityJournal().Query(t.Context(), activity.Query{CallID: id})
	encoded, _ := json.Marshal(after)
	if err != nil || string(original) != string(encoded) {
		t.Fatal("HTTP compatibility rewrote history", err)
	}
}
