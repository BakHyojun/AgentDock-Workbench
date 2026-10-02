package activity

import (
	"encoding/json"
	"testing"
)

func TestManagementLegacyReadCompatibilityDoesNotWriteOrTrustUserText(t *testing.T) {
	store, err := New(t.TempDir(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	event := Event{Binding: Binding{CallID: "call_112233445566778899aabbccddeeff00"}, Kind: "call.completed", ToolName: "permission.update", Title: "修改执行权限", Status: "succeeded", Summary: "scope=global scope_id= mode=rules revision=2；操作系统权限未改变。"}
	if _, err := store.Append(t.Context(), event); err != nil {
		t.Fatal(err)
	}
	before, err := store.Query(t.Context(), Query{CallID: event.CallID})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(before)
	for _, updates := range []bool{false, true} {
		page, err := store.Calls(t.Context(), CallQuery{Updates: updates, Limit: 10})
		if err != nil || len(page.Calls) != 1 || page.Calls[0].TitleText == nil || page.Calls[0].SummaryText == nil {
			t.Fatalf("list compatibility: %+v %v", page, err)
		}
	}
	call, err := store.Call(t.Context(), event.CallID)
	if err != nil || call.TitleText == nil || call.SummaryText == nil || call.Title != event.Title || call.Summary != event.Summary {
		t.Fatalf("detail: %+v %v", call, err)
	}
	after, err := store.Query(t.Context(), Query{CallID: event.CallID})
	encoded, _ := json.Marshal(after)
	if err != nil || string(raw) != string(encoded) || after.Events[0].TitleText != nil {
		t.Fatal("compatibility rewrote old journal", err)
	}
	for _, modified := range []Event{
		{ToolName: "third:permission.update", Title: event.Title, Summary: event.Summary, Status: "succeeded"},
		{ToolName: event.ToolName, Title: event.Title, Summary: event.Summary, Status: "succeeded", LabelSource: "user"},
		{ToolName: event.ToolName, Title: event.Title, Summary: "unknown failure 原文", Status: "failed"},
	} {
		described := DescribeManagement(modified)
		if described.SummaryText != nil {
			t.Fatal("unknown/user/external summary trusted", described)
		}
		if (modified.ToolName != event.ToolName || modified.LabelSource != "") && described.TitleText != nil {
			t.Fatal("user/external title trusted")
		}
	}
	future := &LocalizedText{SchemaVersion: 2, Code: "future", TextHash: "stale"}
	event.TitleText = future
	if DescribeManagement(event).TitleText != future {
		t.Fatal("future metadata replaced")
	}
}

func TestManagementMessagesRequireToolStatusAndExactText(t *testing.T) {
	for _, tc := range []struct{ tool, status, raw, code string }{
		{"conversation.trash", "succeeded", "已移入回收站，源码、仓库和工作区未改动。", "trashed"},
		{"task.delete", "succeeded", "管理对象已永久删除；项目文件未改动，执行与审批审计按独立保留规则保存。", "deleted"},
		{"conversation.trash", "skipped", "仍有关联的运行项或待审批请求，请先停止或处理审批。项目文件未改动。", "busy"},
		{"conversation.link_task", "succeeded", "关联仅影响管理关系，不改写历史调用的 task_id。", "linked"},
		{"call.stop", "succeeded", "已确认命令进程退出。", "process_exited"},
		{"call.stop", "failed", "当前服务没有该运行实例，请核对已记录调用的实际状态。", "instance_unavailable"},
	} {
		got := ManagementMessage(tc.tool, tc.status, tc.raw)
		if got == nil || got.Code != "management."+tc.code || !got.valid(tc.raw) {
			t.Fatalf("message: %+v", tc)
		}
		if ManagementMessage("third:"+tc.tool, tc.status, tc.raw) != nil || ManagementMessage(tc.tool, "unknown", tc.raw) != nil || ManagementMessage(tc.tool, tc.status, tc.raw+" user") != nil {
			t.Fatalf("unsafe matching: %+v", tc)
		}
	}
}
