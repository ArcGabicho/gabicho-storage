using System.Net;

namespace GabichoStorage.Api.Middleware;

/// <summary>
/// El servicio sólo se expone vía Cloudflare Tunnel: cloudflared corre en
/// el mismo host y es el único proceso que puede alcanzar el puerto
/// publicado (ver docker-compose.yml, bindeado a 127.0.0.1). Por eso es
/// seguro confiar incondicionalmente en el header CF-Connecting-IP que
/// Cloudflare siempre setea con la IP real del visitante -- sin esto,
/// HttpContext.Connection.RemoteIpAddress devolvería la IP interna de
/// Docker/loopback para TODAS las requests, rompiendo el rate limiting por
/// IP y el campo de IP en los logs.
/// </summary>
public class CloudflareRealIpMiddleware(RequestDelegate next)
{
    public async Task InvokeAsync(HttpContext context)
    {
        if (context.Request.Headers.TryGetValue("CF-Connecting-IP", out var value) &&
            IPAddress.TryParse(value.ToString(), out var ip))
        {
            context.Connection.RemoteIpAddress = ip;
        }

        await next(context);
    }
}
