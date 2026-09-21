using GabichoStorage.Api.Entities;

namespace GabichoStorage.Api.Services;

public interface IApiKeyAuthenticator
{
    /// <summary>
    /// Valida una raw API key (formato "sk_&lt;id&gt;_&lt;secret&gt;") contra la base.
    /// Devuelve null si falta, tiene formato inválido, no existe o el
    /// secreto no matchea -- nunca distingue el motivo en el valor de
    /// retorno (evita filtrar si un id existe o no).
    /// </summary>
    Task<ApiKey?> AuthenticateAsync(string? rawKey, CancellationToken ct = default);
}
