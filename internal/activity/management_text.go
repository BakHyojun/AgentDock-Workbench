package activity

import (
	"regexp"
	"strings"
)

var ownedPermissionSummary = regexp.MustCompile(`^scope=(global|workspace|conversation) scope_id=([A-Za-z0-9_-]*) mode=([a-z_]+) revision=([0-9]+)；操作系统权限未改变。$`)

var managementActions = map[string]bool{
	"rename": true, "pin": true, "unpin": true, "tags": true, "archive": true,
	"unarchive": true, "trash": true, "restore": true, "delete": true,
}

func managementAction(tool string) (string, string, bool) {
	kind, action, found := strings.Cut(tool, ".")
	return kind, action, found && (kind == "conversation" || kind == "task") && managementActions[action]
}

// DescribeManagement adds optional, hash-bound presentation metadata only for
// exact product-owned management messages. Originals and unknown errors remain
// unchanged. The same recognizer serves persistence and the old-record boundary.
func DescribeManagement(event Event) Event {
	if event.LabelSource != "" || event.Label != "" {
		return event
	}
	if event.TitleText == nil {
		switch {
		case event.ToolName == "permission.update" && event.Title == "修改执行权限":
			event.TitleText = NewLocalizedText("permission.update", event.Title)
		case event.ToolName == "conversation.link_task" && event.Title == "关联已有任务":
			event.TitleText = NewLocalizedText("management.title.link_task", event.Title)
		case event.ToolName == "conversation.set_current" && event.Title == "设置对话当前任务":
			event.TitleText = NewLocalizedText("management.title.set_current", event.Title)
		case event.ToolName == "call.stop" && event.Title == "停止执行":
			event.TitleText = NewLocalizedText("management.title.stop", event.Title)
		default:
			kind, action, ok := managementAction(event.ToolName)
			prefix := action + " · "
			if ok && strings.HasPrefix(event.Title, prefix) {
				id := strings.TrimPrefix(event.Title, prefix)
				binding := Binding{ConversationID: id}
				if kind == "task" {
					binding = Binding{TaskID: id}
				}
				if id != "" && binding.Validate() == nil {
					event.TitleText = NewLocalizedText("management.action."+action, event.Title, id)
				}
			}
		}
	}
	if event.SummaryText == nil {
		event.SummaryText = ManagementMessage(event.ToolName, event.Status, event.Summary)
	}
	return event
}

// ManagementMessage also describes batch outcome messages without converting
// arbitrary diagnostics or changing skipped/failed outcomes into success.
func ManagementMessage(tool, status, raw string) *LocalizedText {
	if tool == "permission.update" && status == "succeeded" {
		if match := ownedPermissionSummary.FindStringSubmatch(raw); match != nil {
			return NewLocalizedText("permission.updated", raw, match[1], match[2], match[3], match[4])
		}
	}
	code := ""
	if _, action, ok := managementAction(tool); ok {
		switch {
		case status == "succeeded" && raw == "管理数据已更新。":
			code = "updated"
		case status == "succeeded" && action == "trash" && raw == "已移入回收站，源码、仓库和工作区未改动。":
			code = "trashed"
		case status == "succeeded" && action == "delete" && raw == "管理对象已永久删除；项目文件未改动，执行与审批审计按独立保留规则保存。":
			code = "deleted"
		case (status == "skipped" || status == "cancelled") && raw == "仍有关联的运行项或待审批请求，请先停止或处理审批。项目文件未改动。":
			code = "busy"
		case status == "skipped" && raw == "请求已取消，该对象未处理。":
			code = "request_cancelled"
		}
	}
	if tool == "conversation.link_task" && status == "succeeded" && raw == "关联仅影响管理关系，不改写历史调用的 task_id。" {
		code = "linked"
	}
	if tool == "conversation.set_current" && status == "succeeded" && raw == "已更新后续调用的默认任务。运行项和审批快照保持不变。" {
		code = "default_task_updated"
	}
	if tool == "call.stop" {
		switch {
		case status == "succeeded" && raw == "已发送停止请求，等待进程退出。":
			code = "stop_requested"
		case status == "succeeded" && raw == "已确认命令进程退出。":
			code = "process_exited"
		case status == "succeeded" && raw == "已发送取消请求，最终副作用结果以原调用状态为准。":
			code = "cancel_requested"
		case status == "failed" && raw == "当前服务没有该运行实例，请核对已记录调用的实际状态。":
			code = "instance_unavailable"
		}
	}
	if code == "" {
		return nil
	}
	return NewLocalizedText("management."+code, raw)
}
