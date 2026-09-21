using GabichoStorage.Api.Dtos;
using Microsoft.AspNetCore.Mvc;
using Microsoft.AspNetCore.Mvc.Filters;

namespace GabichoStorage.Api.Auth;

/// <summary>
/// Exige que la API key autenticada (ya validada por
/// <see cref="ApiKeyAuthAttribute"/>, que debe correr antes) tenga el
/// permiso indicado. Equivalente a middleware.RequirePermission.
/// </summary>
public class RequirePermissionAttribute(string permission) : Attribute, IAsyncAuthorizationFilter, IOrderedFilter
{
    // Debe correr después de ApiKeyAuthAttribute (Order 10).
    public int Order => 20;

    public Task OnAuthorizationAsync(AuthorizationFilterContext context)
    {
        var key = context.HttpContext.GetApiKey();
        if (key is null)
        {
            context.Result = new UnauthorizedObjectResult(new ErrorResponse("no autenticado"));
            return Task.CompletedTask;
        }

        if (!key.HasPermission(permission))
        {
            context.Result = new ObjectResult(new ErrorResponse($"permiso insuficiente: se requiere '{permission}'"))
            {
                StatusCode = StatusCodes.Status403Forbidden,
            };
        }

        return Task.CompletedTask;
    }
}
