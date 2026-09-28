using System.IO;
using System.Xml.Linq;
using AgentDock.ControlPanel;

internal static class TaskLegacyPowerShellPolicy
{
    internal static void Run()
    {
        // Match the existing Setup E2E's historical action, without scheduling
        // or starting it. Restoration must retain the original user's root.
        var root = Path.Combine(Path.GetTempPath(), "AgentDock legacy 한글 fixture");
        const string sid = "S-1-5-21-1-2-3-1001";
        var script = Path.Combine(root, "start-agentdock.ps1");
        var arguments = $"-NoLogo -NoProfile -NonInteractive -WindowStyle Hidden -ExecutionPolicy Bypass -File \"{script}\"";
        XNamespace ns = "http://schemas.microsoft.com/windows/2004/02/mit/task";
        string Xml(string command, string args, string user = sid, string directory = "") =>
            new XElement(ns + "Task",
                new XElement(ns + "Principals", new XElement(ns + "Principal",
                    new XElement(ns + "UserId", user), new XElement(ns + "LogonType", "InteractiveToken"))),
                new XElement(ns + "Actions", new XElement(ns + "Exec", new XElement(ns + "Command", command),
                    new XElement(ns + "Arguments", args), new XElement(ns + "WorkingDirectory", directory)))).ToString();
        var assertions = 0;
        void Validate(string xml, bool legacy) => TaskDefinitionPolicy.Validate(xml, "AgentDock", root, sid,
            value => value, allowStandardTask: true, allowLegacyAction: legacy);
        void Refused(string xml, bool legacy)
        {
            try { Validate(xml, legacy); }
            catch (InvalidOperationException) { assertions++; return; }
            throw new InvalidOperationException("Unowned or new PowerShell task was accepted.");
        }
        foreach (var command in new[] { "powershell.exe", Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.System), "WindowsPowerShell", "v1.0", "powershell.exe") })
        {
            var xml = Xml(command, arguments);
            Refused(xml, false);
            Validate(xml, true); assertions++;
            Validate(Xml(command, arguments, directory: root), true); assertions++;
            Refused(Xml(command, arguments, "S-1-5-18"), true);
            Refused(Xml(command, arguments, directory: root + "-other"), true);
            foreach (var changed in new[] {
                arguments + " extra", arguments.Replace(script, script + ".other"),
                arguments.Replace(script, Path.Combine(root + "-other", "start-agentdock.ps1")),
                arguments.Replace("-File", "-Command"), arguments.Replace("-NoProfile ", ""),
                arguments.Replace(script, "start-agentdock.ps1"), arguments + " -EncodedCommand AA==" })
                Refused(Xml(command, changed), true);
            Refused(xml.Replace("InteractiveToken", "Password"), true);
            Refused(xml.Replace("</Actions>", "<Exec><Command>other.exe</Command></Exec></Actions>"), true);
        }
        foreach (var command in new[] { "pwsh.exe", ".\\powershell.exe", Path.Combine(root, "powershell.exe"),
            Path.Combine(root, "WindowsPowerShell", "v1.0", "powershell.exe") })
            Refused(Xml(command, arguments), true);
        Console.WriteLine($"Legacy PowerShell ownership: {assertions} assertions passed; no task or process launched.");
    }
}
