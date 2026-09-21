using GabichoStorage.Api.Entities;

namespace GabichoStorage.Api.Auth;

public static class HttpContextExtensions
{
    private const string ApiKeyItemKey = "ApiKey";

    public static void SetApiKey(this HttpContext context, ApiKey key) => context.Items[ApiKeyItemKey] = key;

    /// <summary>
    /// Devuelve la API key autenticada de la request actual (seteada por
    /// <see cref="ApiKeyAuthAttribute"/> o, en la descarga con auth
    /// opcional, por el propio controller), o null si no hay ninguna.
    /// </summary>
    public static ApiKey? GetApiKey(this HttpContext context) => context.Items[ApiKeyItemKey] as ApiKey;

    /// <summary>
    /// Extrae la raw API key de la request: header X-API-Key, o
    /// "Authorization: Bearer &lt;key&gt;".
    /// </summary>
    public static string? ExtractRawApiKey(this HttpContext context)
    {
        if (context.Request.Headers.TryGetValue("X-API-Key", out var apiKeyHeader) &&
            !string.IsNullOrEmpty(apiKeyHeader))
        {
            return apiKeyHeader.ToString();
        }

        if (context.Request.Headers.TryGetValue("Authorization", out var authHeader))
        {
            const string prefix = "Bearer ";
            var value = authHeader.ToString();
            if (value.StartsWith(prefix, StringComparison.Ordinal))
            {
                return value[prefix.Length..];
            }
        }

        return null;
    }
}
