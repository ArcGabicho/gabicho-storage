using System.Diagnostics;
using GabichoStorage.Api.Auth;

namespace GabichoStorage.Api.Middleware;

/// <summary>
/// Registra cada request en formato estructurado (JSON vía el formatter de
/// consola configurado en Program.cs): método, path, status, latencia, IP y
/// user_id de la API key si aplica.
/// </summary>
public class RequestLoggingMiddleware(RequestDelegate next, ILogger<RequestLoggingMiddleware> logger)
{
    public async Task InvokeAsync(HttpContext context)
    {
        var stopwatch = Stopwatch.StartNew();

        await next(context);

        stopwatch.Stop();

        var status = context.Response.StatusCode;
        var userId = context.GetApiKey()?.UserId;

        var logLevel = status switch
        {
            >= 500 => LogLevel.Error,
            >= 400 => LogLevel.Warning,
            _ => LogLevel.Information,
        };

        logger.Log(logLevel,
            "request {Method} {Path} status={Status} latencyMs={LatencyMs} ip={Ip} userId={UserId}",
            context.Request.Method,
            context.Request.Path,
            status,
            stopwatch.Elapsed.TotalMilliseconds,
            context.Connection.RemoteIpAddress,
            userId ?? "-");
    }
}
