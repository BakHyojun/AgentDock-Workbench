using System.IO;
using System.Security;
using AgentDock.ControlPanel;
internal static class TaskOwnerRegression
{
 internal static void Run()
 {
   var root=Path.Combine(Path.GetTempPath(),"AgentDock task-policy fixture");var sid="S-1-5-21-1-2-3-1001";
   var command=Path.Combine(root,"bin","agentdock-tray.exe");
   var xml=$"<Task xmlns=\"http://schemas.microsoft.com/windows/2004/02/mit/task\"><Principals><Principal><UserId>{sid}</UserId><LogonType>InteractiveToken</LogonType><RunLevel>HighestAvailable</RunLevel></Principal></Principals><Actions><Exec><Command>{SecurityElement.Escape(command)}</Command><Arguments>--run-core-task --runtime-root &quot;{SecurityElement.Escape(root)}&quot;</Arguments></Exec></Actions></Task>";
   TaskDefinitionPolicy.Validate(xml,"AgentDock",root,sid,value=>value);var assertions=1;
   foreach(var invalid in new[]{xml.Replace(sid,"S-1-5-18"),xml.Replace("InteractiveToken","Password"),xml.Replace("HighestAvailable","LeastPrivilege"),xml.Replace(command,Path.Combine(root,"not-agentdock.exe")),xml.Replace("--run-core-task","--background"),xml.Replace("</Arguments>"," --extra</Arguments>"),xml.Replace("</Actions>","<Exec><Command>other.exe</Command></Exec></Actions>"),xml.Replace("<Actions>","<Actions><ComHandler/>"),"<!DOCTYPE Task [<!ENTITY x SYSTEM 'file:///never'>]>"+xml}) {
     try {TaskDefinitionPolicy.Validate(invalid,"AgentDock",root,sid,value=>value);throw new Exception("foreign or malformed definition accepted");}
     catch(Exception ex) when(ex is InvalidOperationException or System.Xml.XmlException) {assertions++;}
   }
   foreach(var otherRoot in new[]{Path.Combine(root,"nested"),"relative"}) {
     try{TaskDefinitionPolicy.Validate(xml,"AgentDock",otherRoot,sid,value=>value);throw new Exception("other root accepted");}catch(InvalidOperationException){assertions++;}
   }
   foreach (var name in new[] { "AgentDock", "AgentDock-Acceptance-fixture", "Custom AgentDock" }) {
     TaskDefinitionPolicy.Validate(xml, name, root, sid, value => value); assertions++;
     TaskDefinitionPolicy.Validate(xml.Replace("HighestAvailable", "LeastPrivilege"), name, root, sid, value => value, allowStandardTask: true); assertions++;
   }
   foreach (var name in new[] { "", "..", "Other/AgentDock", "Other\\AgentDock", "AgentDock\nOther" }) {
     try { TaskDefinitionPolicy.Validate(xml, name, root, sid, value => value, allowStandardTask: true); throw new Exception("invalid task path accepted"); }
     catch (InvalidOperationException) { assertions++; }
   }
   foreach (var invalid in new[] { xml.Replace(sid,"S-1-5-18"), xml.Replace("--run-core-task", "--background"), xml.Replace("InteractiveToken", "Password"), xml.Replace("HighestAvailable", "UnknownLevel") }) {
     try { TaskDefinitionPolicy.Validate(invalid,"AgentDock",root,sid,value=>value,allowStandardTask:true); throw new Exception("standard restoration broadened task ownership"); }
     catch (InvalidOperationException) { assertions++; }
   }
   var omittedLevel = xml.Replace("<RunLevel>HighestAvailable</RunLevel>", "");
   TaskDefinitionPolicy.Validate(omittedLevel, "AgentDock", root, sid, value => value, allowStandardTask: true); assertions++;
   try { TaskDefinitionPolicy.Validate(omittedLevel,"AgentDock",root,sid,value=>value); throw new Exception("omitted level became elevated"); }
   catch (InvalidOperationException) { assertions++; }
   foreach (var allowLegacy in new[] { false, true }) {
     try { TaskDefinitionPolicy.Validate(xml.Replace("--run-core-task", "--task-core-host"), "AgentDock", root, sid, value => value, allowLegacyAction: allowLegacy); throw new Exception("unsupported historical host action accepted"); }
     catch (InvalidOperationException) { assertions++; }
   }
   var legacyCore = xml.Replace(command,Path.Combine(root,"bin","agentdock.exe")).Replace("--run-core-task", "service launch-core");
   try { TaskDefinitionPolicy.Validate(legacyCore,"AgentDock",root,sid,value=>value); throw new Exception("legacy action accepted as a new definition"); }
   catch (InvalidOperationException) { assertions++; }
   TaskDefinitionPolicy.Validate(legacyCore,"AgentDock",root,sid,value=>value,allowLegacyAction:true); assertions++;
   foreach (var invalid in new[] { legacyCore.Replace(sid,"S-1-5-18"),legacyCore.Replace("</Arguments>"," --extra</Arguments>"),legacyCore.Replace("InteractiveToken","Password") }) {
     try { TaskDefinitionPolicy.Validate(invalid,"AgentDock",root,sid,value=>value,allowLegacyAction:true); throw new Exception("legacy recovery weakened ownership"); }
     catch (InvalidOperationException) { assertions++; }
   }
   TaskDefinitionPolicy.ValidateLauncher(command, root); assertions++;
   foreach (var launcher in new[] { Path.Combine(root, "other.exe"), command + ".other", "agentdock-tray.exe" }) {
     try { TaskDefinitionPolicy.ValidateLauncher(launcher, root); throw new Exception("unowned launcher accepted"); }
     catch (InvalidOperationException) { assertions++; }
   }
   TaskLegacyPowerShellPolicy.Run();
   Console.WriteLine($"Task definition ownership: {assertions} assertions passed; no task, elevation, service or UI operation executed.");
 }
}
