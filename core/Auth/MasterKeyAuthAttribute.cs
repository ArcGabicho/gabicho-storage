using System.Security.Cryptography;
using System.Text;
using GabichoStorage.Api.Dtos;
using GabichoStorage.Api.Options;
using Microsoft.AspNetCore.Mvc;
using Microsoft.AspNetCore.Mvc.Filters;
using Microsoft.Extensions.Options;

namespace GabichoStorage.Api.Auth;

/// <summary>
/// Protege endpoints de administración (gestión de API keys) exigiendo el
/// header X-Master-Key, comparado en tiempo constante contra
/// ADMIN_MASTER_KEY. Equivalente a middleware.MasterKeyAuth.
/// </summary>
public class MasterKeyAuthAttribute : Attribute, IAsyncAuthorizationFilter
{
    public Task OnAuthorizationAsync(AuthorizationFilterContext context)
    {
        var options = context.HttpContext.RequestServices.GetRequiredService<IOptions<StorageOptions>>().Value;
        var logger = context.HttpContext.RequestServices.GetRequiredService<ILogger<MasterKeyAuthAttribute>>();

        var provided = context.HttpContext.Request.Headers["X-Master-Key"].ToString();

        var providedBytes = Encoding.UTF8.GetBytes(provided);
        var masterBytes = Encoding.UTF8.GetBytes(options.MasterKey);

        // subtle.ConstantTimeCompare equivalente: si difieren en longitud ya
        // sabemos que no matchea, pero corremos igual la comparación de
        // tiempo constante para no filtrar nada por timing.
        var matches = providedBytes.Length == masterBytes.Length &&
                      CryptographicOperations.FixedTimeEquals(providedBytes, masterBytes);

        if (string.IsNullOrEmpty(provided) || !matches)
        {
            logger.LogWarning("intento de acceso con master key inválida desde {Ip}, path {Path}",
                context.HttpContext.Connection.RemoteIpAddress, context.HttpContext.Request.Path);
            context.Result = new UnauthorizedObjectResult(new ErrorResponse("master key inválida o faltante"));
        }

        return Task.CompletedTask;
    }
}
