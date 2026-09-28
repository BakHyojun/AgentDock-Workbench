using System.Diagnostics;
using System.IO;
using System.Runtime.InteropServices;
using System.Security.Principal;
using System.Security.AccessControl;
using System.Security.Cryptography;
using System.Text;
using System.Text.Json;

namespace AgentDock.ControlPanel;

internal static class TaskAdminService
{
    private const string DefaultTaskName = "AgentDock";
    private const int TaskActionExec = 0;
    private const int TaskTriggerLogon = 9;
    private const int TaskCreateOrUpdate = 6;
    private const int TaskDontAddPrincipalAce = 0x10;
    private const int TaskIgnoreRegistrationTriggers = 0x20;
    private const int TaskLogonInteractiveToken = 3;
    private const int TaskRunLevelHighest = 1;
    private const int TaskInstancesIgnoreNew = 2;
    private const int DaclSecurityInformation = 0x4;

    internal static int Run(string[] arguments)
    {
        try
        {
            var request = Parse(arguments);
            EnsureAdministrator();
            RequireRuntimeRoot(request);
            if (!TaskDefinitionPolicy.IsValidTaskName(request.TaskName)) throw new InvalidOperationException("task_owner_mismatch: invalid managed task name.");
            using var identity = WindowsIdentity.GetCurrent();
            var currentSid = identity.User?.Value ?? throw new InvalidOperationException("task_owner_mismatch: current SID is missing.");
            if (request.UserSid.Length > 0) EnsureSameWindowsUser(request.UserSid);
            var ownerPath = Path.Combine(request.RuntimeRoot, "credential-owner-sid.txt");
            if (File.Exists(ownerPath) && !string.Equals(File.ReadAllText(ownerPath).Trim(), currentSid, StringComparison.OrdinalIgnoreCase))
                throw new InvalidOperationException("task_owner_mismatch: credentials belong to another Windows user.");
            request = request with { RuntimeRoot = Path.GetFullPath(request.RuntimeRoot), UserSid = currentSid };
            if (request.Action == "prepare-elevated")
                TaskDefinitionPolicy.ValidateLauncher(request.LauncherPath, request.RuntimeRoot);
            using var scheduler = new SchedulerSession();
            ValidateExistingTask(scheduler.Root, request);
            switch (request.Action)
            {
                case "prepare-elevated":
                    RequireBackupDirectory(request);
                    RequireRuntimeRoot(request);
                    SaveBackup(scheduler.Root, request);
                    try
                    {
                        RemoveTask(scheduler.Root, request.TaskName);
                        StopInstalledCore(request.RuntimeRoot);
                        CreateElevatedTask(scheduler.Service, scheduler.Root, request);
                    }
                    catch
                    {
                        RestoreBackup(scheduler.Root, request);
                        throw;
                    }
                    break;
                case "prepare-standard":
                    RequireBackupDirectory(request);
                    RequireRuntimeRoot(request);
                    SaveBackup(scheduler.Root, request);
                    try
                    {
                        RemoveTask(scheduler.Root, request.TaskName);
                        StopInstalledCore(request.RuntimeRoot);
                    }
                    catch
                    {
                        RestoreBackup(scheduler.Root, request);
                        throw;
                    }
                    break;
                case "restore":
                    RequireBackupDirectory(request);
                    RestoreBackup(scheduler.Root, request);
                    break;
                case "remove":
                    RequireRuntimeRoot(request);
                    RemoveTask(scheduler.Root, request.TaskName);
                    StopInstalledCore(request.RuntimeRoot);
                    break;
                case "set-enabled":
                    SetTaskEnabled(scheduler.Root, request.TaskName, request.Enabled);
                    break;
                default:
                    throw new InvalidOperationException(UiText.Format("UnsupportedTaskAdminAction", request.Action));
            }
            return 0;
        }
        catch (Exception ex)
        {
            Console.Error.WriteLine(ex.Message);
            return 1;
        }
    }

    private static TaskAdminRequest Parse(string[] arguments)
    {
        var action = ReadArgument(arguments, "--task-admin");
        if (action is not ("prepare-elevated" or "prepare-standard" or "restore" or "remove" or "set-enabled"))
        {
            throw new InvalidOperationException(UiText.Get("InvalidTaskAdminAction"));
        }
        return new TaskAdminRequest(
            action,
            NormalizeTaskName(ReadArgument(arguments, "--task-name", required: false)),
            ReadArgument(arguments, "--backup-directory", required: false),
            ReadArgument(arguments, "--launcher-path", required: false),
            ReadArgument(arguments, "--runtime-root", required: false),
            ReadArgument(arguments, "--user-sid", required: false),
            ReadArgument(arguments, "--user-name", required: false),
            ReadOptionalBoolArgument(arguments, "--enabled"));
    }

    private static string NormalizeTaskName(string value)
    {
        value = value.Trim();
        if (value.Length == 0)
        {
            return DefaultTaskName;
        }
        if (!TaskDefinitionPolicy.IsValidTaskName(value))
        {
            throw new InvalidOperationException(UiText.Format("MissingAdminArgument", "--task-name"));
        }
        return value;
    }

    private static string ReadArgument(string[] arguments, string name, bool required = true)
    {
        for (var index = 0; index < arguments.Length - 1; index++)
        {
            if (string.Equals(arguments[index], name, StringComparison.OrdinalIgnoreCase))
            {
                var value = arguments[index + 1];
                if (!string.IsNullOrWhiteSpace(value))
                {
                    return value;
                }
                break;
            }
        }
        if (required)
        {
            throw new InvalidOperationException(UiText.Format("MissingAdminArgument", name));
        }
        return "";
    }

    private static void EnsureAdministrator()
    {
        using var identity = WindowsIdentity.GetCurrent();
        var principal = new WindowsPrincipal(identity);
        if (!principal.IsInRole(WindowsBuiltInRole.Administrator))
        {
            throw new InvalidOperationException(UiText.Get("TaskAdminRequiresElevation"));
        }
    }

    private static void EnsureSameWindowsUser(string expectedSid)
    {
        if (string.IsNullOrWhiteSpace(expectedSid))
        {
            throw new InvalidOperationException(UiText.Get("ElevatedModeMissingSid"));
        }
        using var identity = WindowsIdentity.GetCurrent();
        if (!string.Equals(identity.User?.Value, expectedSid, StringComparison.OrdinalIgnoreCase))
        {
            throw new InvalidOperationException(UiText.Get("ElevatedModeRequiresCurrentUserUac"));
        }
    }

    private static bool? ReadOptionalBoolArgument(string[] arguments, string name)
    {
        var value = ReadArgument(arguments, name, required: false);
        if (string.IsNullOrWhiteSpace(value))
        {
            return null;
        }
        if (!bool.TryParse(value, out var parsed))
        {
            throw new InvalidOperationException(UiText.Format("AdminArgumentBoolean", name));
        }
        return parsed;
    }

    private static void SetTaskEnabled(dynamic root, string taskName, bool? enabled)
    {
        if (enabled is null)
        {
            throw new InvalidOperationException(UiText.Get("TaskEnabledStateRequired"));
        }
        dynamic? task = FindTask(root, taskName);
        if (task is null)
        {
            throw new InvalidOperationException(UiText.Get("ScheduledTaskMissing"));
        }
        task.Enabled = enabled.Value;
    }

    private static void RequireBackupDirectory(TaskAdminRequest request)
    {
        if (string.IsNullOrWhiteSpace(request.BackupDirectory))
        {
            throw new InvalidOperationException(UiText.Get("TaskBackupDirectoryRequired"));
        }
    }

    private static void RequireRuntimeRoot(TaskAdminRequest request)
    {
        if (!Path.IsPathFullyQualified(request.RuntimeRoot))
        {
            throw new InvalidOperationException(UiText.Get("RuntimeDirectoryRequired"));
        }
    }

    private static void StopInstalledCore(string runtimeRoot)
    {
        var expectedPaths = InstalledCorePaths(runtimeRoot);
        var deadline = DateTime.UtcNow.AddSeconds(15);

        // Task Scheduler may terminate only the stable CUI parent while a generation Core is still alive.
        // Match by absolute path and cover both the stable legacy entry and active/fallback generations.
        while (true)
        {
            var foundTarget = false;
            foreach (var processName in new[] { "agentdock", "agentdock-core" })
            {
                foreach (var process in Process.GetProcessesByName(processName))
                {
                    using (process)
                    {
                        string? processPath;
                        try
                        {
                            processPath = process.MainModule?.FileName;
                        }
                        catch (System.ComponentModel.Win32Exception)
                        {
                            continue;
                        }
                        catch (InvalidOperationException)
                        {
                            continue;
                        }

                        if (string.IsNullOrWhiteSpace(processPath) ||
                            !expectedPaths.Contains(Path.GetFullPath(processPath)))
                        {
                            continue;
                        }

                        foundTarget = true;
                        try
                        {
                            process.Kill(entireProcessTree: true);
                        }
                        catch (InvalidOperationException)
                        {
                            // 进程可能在枚举后自行退出，下一轮会重新确认。
                        }
                    }
                }
            }

            if (!foundTarget)
            {
                return;
            }
            if (DateTime.UtcNow >= deadline)
            {
                throw new InvalidOperationException(UiText.Format("StopCoreFailed", string.Join(", ", expectedPaths)));
            }
            Thread.Sleep(250);
        }
    }

    private static HashSet<string> InstalledCorePaths(string runtimeRoot)
    {
        var root = Path.GetFullPath(runtimeRoot);
        var paths = new HashSet<string>(StringComparer.OrdinalIgnoreCase)
        {
            Path.GetFullPath(Path.Combine(root, "bin", "agentdock.exe"))
        };
        var activePath = Path.Combine(root, "active-version.json");
        try
        {
            if (!File.Exists(activePath))
            {
                return paths;
            }
            using var document = JsonDocument.Parse(File.ReadAllText(activePath));
            foreach (var propertyName in new[] { "active_version", "fallback_version" })
            {
                if (!document.RootElement.TryGetProperty(propertyName, out var value) || value.ValueKind != JsonValueKind.String)
                {
                    continue;
                }
                var version = value.GetString()?.Trim().TrimStart('v');
                if (string.IsNullOrWhiteSpace(version) || !System.Text.RegularExpressions.Regex.IsMatch(version, @"^\d+\.\d+\.\d+$"))
                {
                    continue;
                }
                paths.Add(Path.GetFullPath(Path.Combine(root, "versions", "v" + version, "agentdock-core.exe")));
            }
        }
        catch (Exception ex) when (ex is IOException or UnauthorizedAccessException or JsonException)
        {
            // The stable legacy entry remains a safe cleanup target even when generation state is unreadable.
        }
        return paths;
    }

    private static void SaveBackup(dynamic root, TaskAdminRequest request)
    {
        var taskName = request.TaskName; var backupDirectory = request.BackupDirectory;
        Directory.CreateDirectory(backupDirectory);
        if (File.Exists(Path.Combine(backupDirectory, "state.json")))
            throw new IOException("不得覆盖仍有恢复价值的计划任务备份。");
        dynamic? task = FindTask(root, taskName);
        var state = new TaskBackupState { SchemaVersion = 2, RuntimeRoot = request.RuntimeRoot, TaskName = request.TaskName, UserSid = request.UserSid };
        if (task is not null)
        {
            state.Exists = true;
            state.WasEnabled = task.Enabled;
            state.WasRunning = Convert.ToInt32(task.State) == 4;
            state.SecurityDescriptor = task.GetSecurityDescriptor(DaclSecurityInformation);
            if (string.IsNullOrWhiteSpace(state.SecurityDescriptor)) throw new IOException("未取得计划任务权限描述符，未开始变更。");
            _ = new RawSecurityDescriptor(state.SecurityDescriptor);
            var xml = (string)task.Xml;
            _ = ReadTaskUserId(xml);
            state.XmlDigest = Convert.ToHexString(SHA256.HashData(Encoding.UTF8.GetBytes(xml)));
            RecoveryFiles.WriteText(Path.Combine(backupDirectory, "task.xml"), xml, Encoding.Unicode);
        }
        RecoveryFiles.WriteText(
            Path.Combine(backupDirectory, "state.json"),
            JsonSerializer.Serialize(state));
    }

    private static (TaskBackupState State, string Xml, string UserId) ReadBackup(string backupDirectory)
    {
        var statePath = Path.Combine(backupDirectory, "state.json");
        if (!File.Exists(statePath) || new FileInfo(statePath).Length > 65536)
            throw new InvalidOperationException(UiText.Format("TaskBackupStateMissing", statePath));
        var json = File.ReadAllText(statePath);
        using var document = JsonDocument.Parse(json);
        if (!document.RootElement.TryGetProperty("Exists", out var exists) || exists.ValueKind is not (JsonValueKind.True or JsonValueKind.False))
            throw new IOException("计划任务备份缺少明确的原始存在状态。");
        var state = JsonSerializer.Deserialize<TaskBackupState>(json)
            ?? throw new InvalidOperationException(UiText.Get("TaskBackupStateReadFailed"));
        if (state.RuntimeRoot is null || state.TaskName is null || state.UserSid is null)
            throw new IOException("task_backup_invalid: null ownership fields are not valid recovery metadata.");
        // Preserve the upstream read-only schema-0 absence record. It cannot
        // authorize restoration without ValidateBackupOwnership below.
        if (state.SchemaVersion is not (0 or 1 or 2)) throw new IOException("Unsupported task backup format.");
        if (state.SchemaVersion == 1) ValidateLegacyRecoveryState(state);
        if (!state.Exists) return (state, "", "");
        if (state.SchemaVersion == 0 || string.IsNullOrWhiteSpace(state.SecurityDescriptor))
            throw new IOException("旧备份缺少可验证的任务安全信息；请保留恢复材料并使用匹配版本处理。");
        var xmlPath = Path.Combine(backupDirectory, "task.xml");
        if (!File.Exists(xmlPath) || new FileInfo(xmlPath).Length > 1048576)
            throw new InvalidOperationException(UiText.Format("TaskBackupXmlMissing", xmlPath));
        var xml = File.ReadAllText(xmlPath);
        var userId = ReadTaskUserId(xml);
        if (state.SchemaVersion == 2 && !string.Equals(state.XmlDigest,
            Convert.ToHexString(SHA256.HashData(Encoding.UTF8.GetBytes(xml))), StringComparison.Ordinal))
            throw new IOException("计划任务 XML 与备份完整性记录不符。");
        if (!string.IsNullOrWhiteSpace(state.SecurityDescriptor)) _ = new RawSecurityDescriptor(state.SecurityDescriptor);
        return (state, xml, userId);
    }

    // Earlier fork builds wrote bound schema-1 records. This read-only adapter is confined
    // to explicitly supplied recovery input; new backups always use schema 2.
    // It neither upgrades the original bytes nor accepts anonymous absence.
    private static void ValidateLegacyRecoveryState(TaskBackupState state)
    {
        if (!Path.IsPathFullyQualified(state.RuntimeRoot) || !TaskDefinitionPolicy.IsValidTaskName(state.TaskName) ||
            string.IsNullOrWhiteSpace(state.UserSid))
            throw new IOException("task_backup_invalid: legacy backup omitted its original runtime or user binding.");
    }

    private static void RestoreBackup(dynamic root, TaskAdminRequest request)
    {
        // Validate every restoration input before stopping or deleting anything.
        var taskName = request.TaskName; var backupDirectory = request.BackupDirectory;
        var (state, xml, userId) = ReadBackup(backupDirectory);
        ValidateBackupOwnership(state, xml, request);
        ValidateExistingTask(root, request);
        StopInstalledCore(request.RuntimeRoot);
        RemoveTask(root, taskName);
        if (!state.Exists) return;
        dynamic task = root.RegisterTask(
            taskName,
            xml,
            TaskCreateOrUpdate | TaskDontAddPrincipalAce | TaskIgnoreRegistrationTriggers,
            userId,
            null,
            TaskLogonInteractiveToken,
            string.IsNullOrWhiteSpace(state.SecurityDescriptor) ? null : state.SecurityDescriptor);
        task.Enabled = state.WasEnabled;
        // Registration received the complete original DACL and explicitly
        // disabled principal-ACE insertion. Do not rewrite it a second time.
        // The schema-1 writer's coordinator resumes only after restoring its
        // source files and manifest. Preserve that order; schema 2 retains the
        // upstream resume behavior without introducing another runtime owner.
        if (state.WasRunning && state.SchemaVersion == 2)
        {
            task.Enabled = true;
            task.Run(null);
            task.Enabled = state.WasEnabled;
        }
    }

    internal static void VerifyCurrentTask(string taskName, bool elevated, bool enabled)
    {
        using var scheduler = new SchedulerSession();
        dynamic? task = FindTask(scheduler.Root, taskName);
        if (!elevated)
        {
            if (task is not null) throw new IOException("普通权限转换后仍存在高权限计划任务。");
            return;
        }
        if (task is null || (bool)task.Enabled != enabled || Convert.ToInt32(task.Definition.Principal.RunLevel) != TaskRunLevelHighest)
            throw new IOException("高权限计划任务未达到预期状态。");
    }

    internal static TaskSecurityMatch VerifyRestoredBackup(string taskName, string backupDirectory)
    {
        var (state, xml, userId) = ReadBackup(backupDirectory);
        using var scheduler = new SchedulerSession();
        dynamic? task = FindTask(scheduler.Root, taskName);
        if (!state.Exists)
        {
            if (task is not null) throw new IOException("原本不存在的计划任务未移除。");
            return TaskSecurityMatch.Exact;
        }
        if (task is null || (bool)task.Enabled != state.WasEnabled ||
            !string.Equals(ReadTaskUserId((string)task.Xml), userId, StringComparison.OrdinalIgnoreCase))
            throw new IOException("计划任务恢复后的身份或启用状态不符。");
        var expectedDefinition = System.Xml.Linq.XDocument.Parse(xml);
        var actualDefinition = System.Xml.Linq.XDocument.Parse((string)task.Xml);
        foreach (var section in new[] { "Principals", "Triggers", "Settings", "Actions" })
        {
            var expectedSection = expectedDefinition.Root?.Elements().SingleOrDefault(element => element.Name.LocalName == section);
            var actualSection = actualDefinition.Root?.Elements().SingleOrDefault(element => element.Name.LocalName == section);
            if (!System.Xml.Linq.XNode.DeepEquals(expectedSection, actualSection))
                throw new IOException($"计划任务恢复后的 {section} 定义不符，保留恢复材料。");
        }
        if (!string.IsNullOrWhiteSpace(state.SecurityDescriptor))
        {
            var comparison = TaskSecurityDescriptor.Compare(state.SecurityDescriptor, (string)task.GetSecurityDescriptor(DaclSecurityInformation));
            if (comparison == TaskSecurityMatch.Mismatch) throw new IOException("计划任务恢复后的权限不符，保留原始描述符和恢复材料。");
            return comparison;
        }
        return TaskSecurityMatch.Exact;
    }

    private static void ValidateBackupOwnership(TaskBackupState state, string xml, TaskAdminRequest request)
    {
        // Schema 1 is the previous downstream format. Keep its original binding
        // checks; schema 2 adds upstream XML integrity and DACL verification.
        var bound = state.RuntimeRoot.Length > 0 || state.TaskName.Length > 0 || state.UserSid.Length > 0;
        if (bound && (state.TaskName != request.TaskName ||
            !TaskDefinitionPolicy.Same(state.RuntimeRoot, request.RuntimeRoot) ||
            !string.Equals(state.UserSid, request.UserSid, StringComparison.OrdinalIgnoreCase)))
            throw new InvalidOperationException("task_backup_invalid: backup belongs to another runtime or Windows user.");
        if (!bound && !state.Exists)
            throw new InvalidOperationException("task_backup_invalid: an absent-task backup must identify its original runtime and user.");
        if (state.Exists)
            TaskDefinitionPolicy.Validate(xml, request.TaskName, request.RuntimeRoot, request.UserSid, ResolveUserSid, allowStandardTask: true, allowLegacyAction: true);
    }

    private static string ReadTaskUserId(string xml)
    {
        var document = System.Xml.Linq.XDocument.Parse(xml);
        var userId = document.Descendants()
            .FirstOrDefault(element => element.Name.LocalName == "UserId")?.Value;
        if (string.IsNullOrWhiteSpace(userId))
        {
            throw new InvalidOperationException(UiText.Get("TaskBackupUserMissing"));
        }
        return userId;
    }

    private static string ResolveUserSid(string value) => value.StartsWith("S-1-", StringComparison.OrdinalIgnoreCase)
        ? new SecurityIdentifier(value).Value : ((SecurityIdentifier)new NTAccount(value).Translate(typeof(SecurityIdentifier))).Value;

    private static void ValidateExistingTask(dynamic root, TaskAdminRequest request)
    {
        dynamic? task = FindTask(root, request.TaskName);
        if (task is not null)
            TaskDefinitionPolicy.Validate((string)task.Xml, request.TaskName, request.RuntimeRoot, request.UserSid, ResolveUserSid, allowStandardTask: true, allowLegacyAction: true);
    }

    private static void RemoveTask(dynamic root, string taskName)
    {
        dynamic? task = FindTask(root, taskName);
        if (task is null)
        {
            return;
        }
        try
        {
            task.Stop(0);
        }
        catch (COMException)
        {
            if (Convert.ToInt32(task.State) is 2 or 4) throw;
        }
        var deadline = DateTime.UtcNow.AddSeconds(15);
        while (Convert.ToInt32(task.GetInstances(0).Count) > 0)
        {
            if (DateTime.UtcNow >= deadline) throw new InvalidOperationException("task_stop_incomplete: original task instance is still running.");
            Thread.Sleep(50);
        }
        root.DeleteTask(taskName, 0);
    }

    private static dynamic? FindTask(dynamic root, string taskName)
    {
        dynamic tasks = root.GetTasks(0);
        foreach (dynamic task in tasks)
        {
            if (string.Equals((string)task.Name, taskName, StringComparison.OrdinalIgnoreCase))
            {
                return task;
            }
        }
        return null;
    }

    private static void CreateElevatedTask(dynamic service, dynamic root, TaskAdminRequest request)
    {
        if (string.IsNullOrWhiteSpace(request.LauncherPath) ||
            string.IsNullOrWhiteSpace(request.RuntimeRoot) ||
            string.IsNullOrWhiteSpace(request.UserSid) ||
            string.IsNullOrWhiteSpace(request.UserName))
        {
            throw new InvalidOperationException(UiText.Get("ElevatedTaskArgumentsIncomplete"));
        }
        dynamic definition = service.NewTask(0);
        definition.RegistrationInfo.Description = "AgentDock privileged core service for the current desktop user.";
        definition.Settings.Enabled = false;
        definition.Settings.DisallowStartIfOnBatteries = false;
        definition.Settings.StopIfGoingOnBatteries = false;
        definition.Settings.StartWhenAvailable = true;
        definition.Settings.RestartCount = 3;
        definition.Settings.RestartInterval = "PT1M";
        definition.Settings.ExecutionTimeLimit = "PT0S";
        definition.Settings.MultipleInstances = TaskInstancesIgnoreNew;

        dynamic principal = definition.Principal;
        principal.UserId = request.UserSid;
        principal.LogonType = TaskLogonInteractiveToken;
        principal.RunLevel = TaskRunLevelHighest;

        dynamic trigger = definition.Triggers.Create(TaskTriggerLogon);
        trigger.UserId = request.UserName;

        dynamic action = definition.Actions.Create(TaskActionExec);
        action.Path = Path.GetFullPath(request.LauncherPath);
        action.Arguments = ElevatedCoreArguments(request.RuntimeRoot);

        dynamic task = root.RegisterTaskDefinition(
            request.TaskName,
            definition,
            TaskCreateOrUpdate,
            request.UserSid,
            null,
            TaskLogonInteractiveToken,
            null);
        task.Enabled = false;
        task.SetSecurityDescriptor(
            $"D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GA;;;{request.UserSid})",
            0);
    }

    internal static string ElevatedCoreArguments(string runtimeRoot) => $"--run-core-task --runtime-root \"{Path.GetFullPath(runtimeRoot)}\"";

    private sealed class SchedulerSession : IDisposable
    {
        internal SchedulerSession()
        {
            var schedulerType = Type.GetTypeFromProgID("Schedule.Service")
                ?? throw new InvalidOperationException(UiText.Get("TaskSchedulerComUnavailable"));
            Service = Activator.CreateInstance(schedulerType)
                ?? throw new InvalidOperationException(UiText.Get("TaskSchedulerConnectFailed"));
            Service.Connect();
            Root = Service.GetFolder("\\");
        }

        internal dynamic Service { get; }
        internal dynamic Root { get; }

        public void Dispose()
        {
            if (Marshal.IsComObject(Root))
            {
                Marshal.FinalReleaseComObject(Root);
            }
            if (Marshal.IsComObject(Service))
            {
                Marshal.FinalReleaseComObject(Service);
            }
        }
    }

    private sealed record TaskAdminRequest(
        string Action,
        string TaskName,
        string BackupDirectory,
        string LauncherPath,
        string RuntimeRoot,
        string UserSid,
        string UserName,
        bool? Enabled);

    private sealed class TaskBackupState
    {
        public int SchemaVersion { get; set; }
        public string RuntimeRoot { get; set; } = "";
        public string TaskName { get; set; } = "";
        public string UserSid { get; set; } = "";
        public string XmlDigest { get; set; } = "";
        public bool Exists { get; set; }
        public bool WasEnabled { get; set; }
        public bool WasRunning { get; set; }
        public string SecurityDescriptor { get; set; } = "";
    }
}
