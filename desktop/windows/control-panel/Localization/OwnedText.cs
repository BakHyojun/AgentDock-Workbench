using System.Security.Cryptography;
using System.Text;
using System.Text.Json;
using System.Text.Json.Serialization;

namespace AgentDock.ControlPanel;

public sealed class OwnedTextDescriptor
{
    [JsonPropertyName("schema_version")] public int SchemaVersion { get; set; }
    [JsonPropertyName("code")] public string Code { get; set; } = "";
    [JsonPropertyName("args")] public string[]? Args { get; set; }
    [JsonPropertyName("text_hash")] public string TextHash { get; set; } = "";
}

// Metadata accompanies an unchanged stored original. Unknown versions/codes,
// malformed arguments, user labels and stale hashes always show that original.
internal static class OwnedText
{
    private static readonly HashSet<string> Tools = new(StringComparer.Ordinal) {
        "agentdock_context", "workspace_context", "read_file", "list_dir", "search_text", "exec_command", "file_edit", "task_manage", "workspace_manage",
        "mcp_tool_search", "mcp_tool_list", "mcp_tool_inspect", "plugin_load", "session_observe", "session_act"
    };
    internal static string Render(JsonElement descriptor, string original, string tool, string labelSource, string status)
    {
        if (descriptor.ValueKind != JsonValueKind.Object) return original;
        try { return Render(descriptor.Deserialize<OwnedTextDescriptor>(), original, tool, labelSource, status); }
        catch (JsonException) { return original; }
    }
    internal static string Render(OwnedTextDescriptor? descriptor, string original, string tool, string labelSource, string status)
    {
        if (descriptor is not { SchemaVersion: 1 }) return original;
        if (descriptor.Code == "permission.update" && descriptor.Args is null)
            descriptor = new OwnedTextDescriptor { SchemaVersion = descriptor.SchemaVersion, Code = descriptor.Code, TextHash = descriptor.TextHash, Args = [] };
        if (string.IsNullOrEmpty(descriptor.Code) || descriptor.Args is null || descriptor.Args.Length > 6 ||
            descriptor.Args.Any(value => value is null || Encoding.UTF8.GetByteCount(value) > 1024) || labelSource == "user" ||
            !string.Equals(descriptor.TextHash, Convert.ToHexString(SHA256.HashData(Encoding.UTF8.GetBytes(original))), StringComparison.OrdinalIgnoreCase)) return original;
        string key;
        if (descriptor.Code == "tool." + tool && Tools.Contains(tool) && labelSource == "tool" && descriptor.Args.Length == 1)
            key = "OwnedTool_" + tool;
        else if (tool == "permission.update" && descriptor.Code == "permission.update" && descriptor.Args.Length == 0)
            key = "OwnedPermissionUpdate";
        else if (tool == "permission.update" && descriptor.Code == "permission.updated" && descriptor.Args.Length == 4 && status == "succeeded")
            key = "OwnedPermissionUpdated";
        else if (descriptor.Code.StartsWith("management.", StringComparison.Ordinal) && ManagementKey(descriptor, tool, status) is { } managementKey)
            key = managementKey;
        else return original;
        var format = UiText.Get(key);
        if (format == key) return original;
        try { return string.Format(System.Globalization.CultureInfo.CurrentCulture, format, descriptor.Args.Cast<object?>().ToArray()); }
        catch (FormatException) { return original; }
    }

    private static string? ManagementKey(OwnedTextDescriptor descriptor, string tool, string status)
    {
        var parts = tool.Split('.');
        var action = parts.Length == 2 && parts[0] is "conversation" or "task" &&
            parts[1] is "rename" or "pin" or "unpin" or "tags" or "archive" or "unarchive" or "trash" or "restore" or "delete" ? parts[1] : null;
        if (action is not null && descriptor.Code == "management.action." + action && descriptor.Args!.Length == 1)
            return "OwnedManagementAction_" + action;
        if (descriptor.Args!.Length != 0) return null;
        return (descriptor.Code, tool, status) switch
        {
            ("management.title.link_task", "conversation.link_task", _) => "ExecutionLinkExistingTask",
            ("management.title.set_current", "conversation.set_current", _) => "ExecutionSetCurrentTask",
            ("management.title.stop", "call.stop", _) => "ExecutionStop",
            ("management.updated", _, "succeeded") when action is not null => "OwnedManagement_updated",
            ("management.trashed", _, "succeeded") when action == "trash" => "OwnedManagement_trashed",
            ("management.deleted", _, "succeeded") when action == "delete" => "OwnedManagement_deleted",
            ("management.busy", _, "skipped" or "cancelled") when action is not null => "OwnedManagement_busy",
            ("management.request_cancelled", _, "skipped") when action is not null => "OwnedManagement_request_cancelled",
            ("management.linked", "conversation.link_task", "succeeded") => "OwnedManagement_linked",
            ("management.default_task_updated", "conversation.set_current", "succeeded") => "OwnedManagement_default_task_updated",
            ("management.stop_requested", "call.stop", "succeeded") => "OwnedManagement_stop_requested",
            ("management.process_exited", "call.stop", "succeeded") => "OwnedManagement_process_exited",
            ("management.cancel_requested", "call.stop", "succeeded") => "OwnedManagement_cancel_requested",
            ("management.instance_unavailable", "call.stop", "failed") => "OwnedManagement_instance_unavailable",
            _ => null
        };
    }

    internal static string ApprovalReason(string tool, string ruleId, string original)
    {
        // Rule explanations and independent reviewer responses are user data.
        if (ruleId.Length != 0) return original;
        var key = tool switch
        {
            "exec_command" => "OwnedApprovalReasonCommand",
            "session_act" => "OwnedApprovalReasonSession",
            "file_edit" => "OwnedApprovalReasonFile",
            _ => "OwnedApprovalReasonDefault"
        };
        return original == UiText.Original(key) ? UiText.Get(key) : original;
    }

    internal static string ApprovalScope(string original)
    {
        // Recognize the complete product template before translating any part.
        // Workspace/target paths and session IDs remain verbatim arguments.
        var lines = original.Split('\n');
        var translated = new List<string>();
        var index = 0;
        bool Prefix(string key)
        {
            var prefix = UiText.Original(key).Split("{0}")[0];
            if (index >= lines.Length || !lines[index].StartsWith(prefix, StringComparison.Ordinal)) return false;
            translated.Add(UiText.Format(key, lines[index++][prefix.Length..]));
            return true;
        }
        Prefix("OwnedApprovalScopeWorkspace");
        if (index >= lines.Length || lines[index++] != UiText.Original("OwnedApprovalScopeAccount")) return original;
        translated.Add(UiText.Get("OwnedApprovalScopeAccount"));
        Prefix("OwnedApprovalScopeSessions");
        Prefix("OwnedApprovalScopeTarget");
        if (index < lines.Length && lines[index] == UiText.Original("OwnedApprovalScopeExternal"))
        {
            translated.Add(UiText.Get("OwnedApprovalScopeExternal")); index++;
        }
        return index == lines.Length ? string.Join("\n", translated) : original;
    }
}
