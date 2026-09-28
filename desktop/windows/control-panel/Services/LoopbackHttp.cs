using System.Net.Http;

namespace AgentDock.ControlPanel;

// Core requests go only to 127.0.0.1 and carry the local Bearer token. They
// must never use the system proxy or follow a redirect elsewhere.
internal static class LoopbackHttp
{
    internal static SocketsHttpHandler CreateHandler() => new()
    {
        UseProxy = false,
        AllowAutoRedirect = false,
        ConnectTimeout = TimeSpan.FromSeconds(5),
        PooledConnectionLifetime = TimeSpan.FromMinutes(5)
    };
}
