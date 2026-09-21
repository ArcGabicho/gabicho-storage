using GabichoStorage.Api.Dtos;
using GabichoStorage.Api.Services;
using Microsoft.AspNetCore.Mvc;
using Microsoft.AspNetCore.Mvc.Filters;

namespace GabichoStorage.Api.Auth;

/// <summary>
/// Exige una API key válida (header X-API-Key, o
/// "Authorization: Bearer &lt;key&gt;") y la deja disponible vía
/// HttpContext.GetApiKey() para el resto de la request (el propio action y
/// filtros posteriores como <see cref="RequirePermissionAttribute"/>).
/// </summary>
public class ApiKeyAuthAttribute : Attribute, IAsyncAuthorizationFilter, IOrderedFilter
{
    // Debe correr antes que RequirePermissionAttribute, que depende de que
    // HttpContext.GetApiKey() ya esté seteado.
    public int Order => 10;

    public async Task OnAuthorizationAsync(AuthorizationFilterContext context)
    {
        var rawKey = context.HttpContext.ExtractRawApiKey();
        if (string.IsNullOrEmpty(rawKey))
        {
            context.Result = new UnauthorizedObjectResult(new ErrorResponse("falta la API key (header X-API-Key)"));
            return;
        }

        var authenticator = context.HttpContext.RequestServices.GetRequiredService<IApiKeyAuthenticator>();
        var key = await authenticator.AuthenticateAsync(rawKey, context.HttpContext.RequestAborted);
        if (key is null)
        {
            context.Result = new UnauthorizedObjectResult(new ErrorResponse("API key inválida"));
            return;
        }

        context.HttpContext.SetApiKey(key);
    }
}
