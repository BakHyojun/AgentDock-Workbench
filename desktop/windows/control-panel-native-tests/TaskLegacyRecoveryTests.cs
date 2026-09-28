using System.IO;
using System.Reflection;
using System.Security.Principal;
using System.Text;
using System.Text.Json;
using System.Text.Json.Nodes;
using System.Xml.Linq;
using AgentDock.ControlPanel;

// Test doubles record only scheduler method invocations. No task or executable
// is started by these policy checks; all recovery paths use a new temporary root.
public sealed class RecoveryResumeProbeTask
{
    public bool Enabled { get; set; }
    public int RunCount { get; private set; }
    public void Run(object? parameters) => RunCount++;
}
public sealed class RecoveryResumeProbeRoot
{
    public RecoveryResumeProbeTask Task { get; } = new();
    public int RegistrationFlags { get; private set; }
    public object[] GetTasks(int flags) => [];
    public RecoveryResumeProbeTask RegisterTask(string name, string xml, int flags, string user, object? password, int logonType, string? security)
    {
        RegistrationFlags = flags;
        return Task;
    }
}

internal static class TaskLegacyResumePolicy
{
    internal static void Run(Action<bool,string> check)
    {
        using var identity = WindowsIdentity.GetCurrent();
        var sid = identity.User!.Value;
        var root = Path.Combine(Path.GetTempPath(), "agentdock-legacy-resume-" + Guid.NewGuid().ToString("N"));
        var backup = Path.Combine(root, "recovery");
        Directory.CreateDirectory(backup);
        var parse = typeof(TaskAdminService).GetMethod("Parse", BindingFlags.NonPublic | BindingFlags.Static)!;
        var restore = typeof(TaskAdminService).GetMethod("RestoreBackup", BindingFlags.NonPublic | BindingFlags.Static)!;
        XNamespace ns = "http://schemas.microsoft.com/windows/2004/02/mit/task";
        var xml = new XElement(ns+"Task",
            new XElement(ns+"Principals", new XElement(ns+"Principal", new XElement(ns+"UserId",sid), new XElement(ns+"LogonType","InteractiveToken"), new XElement(ns+"RunLevel","HighestAvailable"))),
            new XElement(ns+"Actions", new XElement(ns+"Exec", new XElement(ns+"Command",Path.Combine(root,"bin","agentdock-tray.exe")), new XElement(ns+"Arguments",TaskAdminService.ElevatedCoreArguments(root))))).ToString();
        try
        {
            foreach (var schema in new[] {1,2})
            {
                var state = new {SchemaVersion=schema, RuntimeRoot=root, TaskName="AgentDock", UserSid=sid, Exists=true, WasEnabled=false, WasRunning=true,
                    SecurityDescriptor=$"D:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;FA;;;{sid})",
                    XmlDigest=schema==2 ? Convert.ToHexString(System.Security.Cryptography.SHA256.HashData(Encoding.UTF8.GetBytes(xml))) : ""};
                var statePath=Path.Combine(backup,"state.json"); var xmlPath=Path.Combine(backup,"task.xml");
                File.WriteAllText(statePath,JsonSerializer.Serialize(state),new UTF8Encoding(false));
                File.WriteAllText(xmlPath,xml,Encoding.Unicode);
                var beforeState=File.ReadAllBytes(statePath); var beforeXml=File.ReadAllBytes(xmlPath);
                var request=parse.Invoke(null,[new[]{"--task-admin","restore","--task-name","AgentDock","--backup-directory",backup,"--runtime-root",root,"--user-sid",sid}])!;
                var probe=new RecoveryResumeProbeRoot();
                restore.Invoke(null,[probe,request]);
                check(probe.Task.RunCount==(schema==1 ? 0 : 1), "Schema 1 leaves resume to its original outer coordinator; schema 2 keeps upstream resume: " + schema);
                check(!probe.Task.Enabled,"Original enabled state retained after restore: " + schema);
                check(probe.RegistrationFlags==(6|0x10|0x20),"Restoration must not inject principal ACEs or fire registration triggers: " + schema);
                check(beforeState.SequenceEqual(File.ReadAllBytes(statePath)) && beforeXml.SequenceEqual(File.ReadAllBytes(xmlPath)),"Resume policy never rewrites recovery input: " + schema);
            }
        }
        finally {Directory.Delete(root,true);}
    }
}

internal static partial class Program
{
    private static void RunLegacyBoundRecovery(dynamic service, dynamic folder, WindowsIdentity identity, string root, string output)
    {
        var name="AgentDock-Acceptance-"+Guid.NewGuid().ToString("N");
        var directory=Path.Combine(root,"bound-schema1");
        var recovery=Path.Combine(directory,"recovery");
        Directory.CreateDirectory(recovery);
        dynamic definition=service.NewTask(0);
        definition.Principal.UserId=identity.User!.Value;
        definition.Principal.LogonType=3;
        definition.Principal.RunLevel=1;
        definition.Settings.Enabled=false;
        dynamic action=definition.Actions.Create(0);
        action.Path=Path.Combine(directory,"bin","agentdock.exe");
        action.Arguments=$"service launch-core --runtime-root \"{directory}\"";
        try
        {
            dynamic original=folder.RegisterTaskDefinition(name,definition,6|0x10|0x20,identity.User.Value,null,3,
                $"D:P(D;;FW;;;AN)(A;;FA;;;SY)(A;;FA;;;BA)(A;;FR;;;{identity.User.Value})");
            original.Enabled=false;
            string xml=original.Xml;
            string security=original.GetSecurityDescriptor(4);
            // Exact field set and text encodings written by the preserved
            // 1.1.16102 TaskAdmin, with an isolated task name/root/current SID.
            var state=new {SchemaVersion=1,RuntimeRoot=directory,TaskName=name,UserSid=identity.User.Value,
                Exists=true,WasEnabled=false,WasRunning=false,SecurityDescriptor=security};
            var statePath=Path.Combine(recovery,"state.json"); var xmlPath=Path.Combine(recovery,"task.xml");
            File.WriteAllText(statePath,JsonSerializer.Serialize(state),new UTF8Encoding(false));
            File.WriteAllText(xmlPath,xml,Encoding.Unicode);
            var originalState=File.ReadAllBytes(statePath); var originalXml=File.ReadAllBytes(xmlPath);
            RemoveFixtureTask(folder,name);
            Native("restore",name,directory,recovery,identity);
            Check(TaskAdminService.VerifyRestoredBackup(name,recovery)==TaskSecurityMatch.Exact,"Real COM restores schema-1 XML and explicit-deny DACL exactly");
            dynamic restored=folder.GetTask(name);
            Check(!(bool)restored.Enabled && Convert.ToInt32(restored.State)!=4,"Legacy task remains disabled and never starts an executable");
            Check(originalState.SequenceEqual(File.ReadAllBytes(statePath)) && originalXml.SequenceEqual(File.ReadAllBytes(xmlPath)),"Real legacy restoration preserves original JSON/XML bytes");
            string restoredXml=restored.Xml;
            foreach(var field in new[]{"RuntimeRoot","TaskName","UserSid"})
            {
                var changed=JsonNode.Parse(originalState)!.AsObject();
                changed[field]=field switch {"RuntimeRoot"=>directory+"-other","TaskName"=>name+"-other",_=>"S-1-5-18"};
                File.WriteAllText(statePath,changed.ToJsonString());
                var rejected=TaskAdminService.Run(["--task-admin","restore","--task-name",name,"--backup-directory",recovery,"--runtime-root",directory,"--user-sid",identity.User.Value]);
                Check(rejected!=0,"Foreign legacy recovery binding refused before mutation: "+field);
                dynamic kept=folder.GetTask(name);
                Check((string)kept.Xml==restoredXml && (string)kept.GetSecurityDescriptor(4)==security,"Foreign input cannot delete or replace the live fixture task: "+field);
            }
            File.WriteAllBytes(statePath,originalState);
            File.Copy(statePath,Path.Combine(output,"bound-schema1-state.json"));
            File.Copy(xmlPath,Path.Combine(output,"bound-schema1-task.xml"));
            Evidence.Add(new{scenario="bound_schema1_restore",task_name=name,runtime_root=directory,restored=true,foreign_binding_rejections=3,original_bytes_preserved=true,security_comparison="Exact",executable_started=false});
        }
        finally {RemoveFixtureTask(folder,name);}
    }
}
