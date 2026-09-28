using System.IO;
using System.Reflection;
using System.Security.Cryptography;
using System.Text;
using System.Text.Json;
using System.Xml.Linq;
using AgentDock.ControlPanel;

internal static class TaskBackupCompatibilityTests
{
    internal static void Run(Action<bool, string> check)
    {
        const string sid = "S-1-5-21-1-2-3-1000";
        var root = Path.Combine(Path.GetTempPath(), "agentdock-backup-schema-" + Guid.NewGuid().ToString("N"));
        var backup = Path.Combine(root, "recovery"); Directory.CreateDirectory(backup);
        var read = typeof(TaskAdminService).GetMethod("ReadBackup", BindingFlags.NonPublic | BindingFlags.Static)!;
        var validate = typeof(TaskAdminService).GetMethod("ValidateBackupOwnership", BindingFlags.NonPublic | BindingFlags.Static)!;
        var parse = typeof(TaskAdminService).GetMethod("Parse", BindingFlags.NonPublic | BindingFlags.Static)!;
        XNamespace ns = "http://schemas.microsoft.com/windows/2004/02/mit/task";
        var xml = new XElement(ns + "Task",
            new XElement(ns + "Principals", new XElement(ns + "Principal", new XElement(ns + "UserId", sid),
                new XElement(ns + "LogonType", "InteractiveToken"), new XElement(ns + "RunLevel", "HighestAvailable"))),
            new XElement(ns + "Actions", new XElement(ns + "Exec", new XElement(ns + "Command", Path.Combine(root, "bin", "agentdock-tray.exe")),
                new XElement(ns + "Arguments", $"--run-core-task --runtime-root \"{root}\"")))).ToString();
        object Request(string task, string directory, string user) => parse.Invoke(null, [new[] { "--task-admin", "restore", "--task-name", task,
            "--backup-directory", backup, "--runtime-root", directory, "--user-sid", user }])!;
        void Refused(Action action, string message)
        {
            var rejected = false;
            try { action(); }
            catch (TargetInvocationException error) when (error.InnerException is IOException or InvalidOperationException) { rejected = true; }
            check(rejected, message);
        }
        try
        {
            foreach (var schema in new[] { 1, 2 })
            {
                var state = new { SchemaVersion = schema, RuntimeRoot = root, TaskName = "AgentDock", UserSid = sid,
                    Exists = true, WasEnabled = true, WasRunning = false,
                    SecurityDescriptor = $"D:P(A;;FA;;;SY)(A;;FR;;;{sid})",
                    XmlDigest = schema == 2 ? Convert.ToHexString(SHA256.HashData(Encoding.UTF8.GetBytes(xml))) : "" };
                var statePath = Path.Combine(backup, "state.json"); var xmlPath = Path.Combine(backup, "task.xml");
                File.WriteAllText(statePath, JsonSerializer.Serialize(state)); File.WriteAllText(xmlPath, xml, Encoding.Unicode);
                var beforeState = File.ReadAllBytes(statePath); var beforeXml = File.ReadAllBytes(xmlPath);
                var value = read.Invoke(null, [backup])!;
                var stored = value.GetType().GetField("Item1")!.GetValue(value)!;
                var restoredXml = (string)value.GetType().GetField("Item2")!.GetValue(value)!;
                check(restoredXml == xml, "Exact legacy/new XML retained: " + schema);
                validate.Invoke(null, [stored, restoredXml, Request("AgentDock", root, sid)]);
                check(true, "Known schema has its original runtime/user binding: " + schema);
                foreach (var request in new[] { Request("OtherTask", root, sid), Request("AgentDock", root + "-other", sid), Request("AgentDock", root, "S-1-5-18") })
                    Refused(() => validate.Invoke(null, [stored, restoredXml, request]), "Foreign backup binding rejected: " + schema);
                check(beforeState.SequenceEqual(File.ReadAllBytes(statePath)) && beforeXml.SequenceEqual(File.ReadAllBytes(xmlPath)), "Validation never mutates recovery bytes: " + schema);
                if (schema == 2)
                {
                    File.WriteAllText(xmlPath, xml.Replace("HighestAvailable", "LeastPrivilege"), Encoding.Unicode);
                    Refused(() => read.Invoke(null, [backup]), "Schema 2 integrity mismatch rejected before scheduler actions");
                    check(File.Exists(statePath) && File.Exists(xmlPath), "Tampered recovery evidence preserved");
                }
            }
            foreach (var schema in new[] { 1, 2 })
                foreach (var field in new[] { "RuntimeRoot", "TaskName", "UserSid" })
                {
                    var metadata = new System.Text.Json.Nodes.JsonObject
                    {
                        ["SchemaVersion"] = schema, ["RuntimeRoot"] = root,
                        ["TaskName"] = "AgentDock", ["UserSid"] = sid, ["Exists"] = false
                    };
                    metadata[field] = null;
                    File.WriteAllText(Path.Combine(backup, "state.json"), metadata.ToJsonString());
                    Refused(() => read.Invoke(null, [backup]), "Null recovery owner rejected before any mutation: " + schema + "/" + field);
                }
            File.WriteAllText(Path.Combine(backup, "state.json"), JsonSerializer.Serialize(new { SchemaVersion = 1, Exists = false }));
            Refused(() => read.Invoke(null, [backup]), "Legacy absent-task backup requires original binding");
            File.WriteAllText(Path.Combine(backup, "state.json"), JsonSerializer.Serialize(new { SchemaVersion = 0, Exists = false }));
            var unbound = read.Invoke(null, [backup])!; var absent = unbound.GetType().GetField("Item1")!.GetValue(unbound)!;
            Refused(() => validate.Invoke(null, [absent, "", Request("AgentDock", root, sid)]), "An unbound absence record cannot authorize task removal");
            check(File.Exists(Path.Combine(backup, "state.json")), "Rejected unbound recovery input is preserved");
            foreach (var field in new[] { "RuntimeRoot", "TaskName", "UserSid" })
            {
                var metadata = new System.Text.Json.Nodes.JsonObject
                {
                    ["SchemaVersion"] = 1, ["RuntimeRoot"] = root,
                    ["TaskName"] = "AgentDock", ["UserSid"] = sid, ["Exists"] = false
                };
                metadata[field] = "";
                var path = Path.Combine(backup, "state.json");
                File.WriteAllText(path, metadata.ToJsonString());
                var original = File.ReadAllBytes(path);
                Refused(() => read.Invoke(null, [backup]), "Legacy adapter rejects missing original binding: " + field);
                check(original.SequenceEqual(File.ReadAllBytes(path)), "Legacy rejection preserves evidence: " + field);
            }
        }
        finally { Directory.Delete(root, true); }
    }
}
