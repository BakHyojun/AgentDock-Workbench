using System.Text;

namespace AgentDock.ControlPanel;

public static class ExecutionTitleFormatter
{
    public static string Format(string tool, string label, string action = "")
    {
        label = string.Join(" ", label.Split((char[]?)null, StringSplitOptions.RemoveEmptyEntries));
        while (tool.Length > 0 && label.StartsWith(tool, StringComparison.Ordinal) &&
               (label.Length == tool.Length || " ·:-：".Contains(label[tool.Length])))
            label = label[tool.Length..].TrimStart(' ', '·', ':', '-', '：');
        if (tool == "file_edit" && label.StartsWith("EDIT_FILE", StringComparison.Ordinal))
            label = label[9..].TrimStart(' ', '·', ':', '-', '：');
        if (label.Length == 0) label = DefaultAction(tool, action);
        return tool.Length == 0 ? label : tool + " · " + label;
    }

    private static string DefaultAction(string tool, string action) => (tool, action) switch
    {
        ("task_manage", "create") => UiText.Get("ExecutionActionCreateTask"),
        ("task_manage", "resume") => UiText.Get("ExecutionActionResumeTask"),
        ("task_manage", "checkpoint") => UiText.Get("ExecutionActionCheckpoint"),
        ("task_manage", "complete") => UiText.Get("ExecutionActionCompleteTask"),
        ("task_manage", "list" or "get") => UiText.Get("ExecutionActionReadTask"),
        ("task_manage", "cancel") => UiText.Get("ActivityCancelTask"),
        ("task_manage", _) => UiText.Get("ExecutionManageTask"),
        ("file_edit", "add") => UiText.Get("ExecutionActionCreateFile"),
        ("file_edit", "delete") => UiText.Get("ExecutionActionDeleteFile"),
        ("file_edit", "move") => UiText.Get("ExecutionActionMoveFile"),
        ("file_edit", _) => UiText.Get("ExecutionTool_file_edit"),
        ("read_file", _) => UiText.Get("ExecutionTool_read_file"),
        ("list_dir", _) => UiText.Get("ExecutionTool_list_dir"),
        ("search_text", _) => UiText.Get("ExecutionTool_search_text"),
        ("agentdock_context", _) => UiText.Get("ExecutionActionContext"),
        ("exec_command", _) => UiText.Get("ExecutionActionCommand"),
        ("session_observe", _) => UiText.Get("ExecutionTool_session_observe"),
        ("session_act", _) => UiText.Get("ExecutionActionSession"),
        ("workspace_manage", _) => UiText.Get("ExecutionActionWorkspace"),
        ("device_manage", _) => UiText.Get("ExecutionActionDevice"),
        ("file_publish", _) => UiText.Get("ExecutionActionPublish"),
        ("mcp_search", _) => UiText.Get("ExecutionActionFindTool"),
        ("mcp_inspect", _) => UiText.Get("ExecutionActionInspectTool"),
        ("mcp_tool_call", _) => UiText.Get("ExecutionActionExternal"),
        ("plugin_load", _) => UiText.Get("ExecutionActionPlugin"),
        ("insertion_ack", _) => UiText.Get("ExecutionActionReceipt"),
        ("plugin_manage", _) => UiText.Get("ExecutionTool_plugin_manage"),
        _ => tool.Contains(':') || tool.Contains('.') ? UiText.Get("ExecutionActionExternal") : UiText.Get("ExecutionActionTool")
    };
}
