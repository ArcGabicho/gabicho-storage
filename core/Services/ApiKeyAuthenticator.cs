using GabichoStorage.Api.Data;
using GabichoStorage.Api.Entities;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;

namespace GabichoStorage.Api.Services;

public class ApiKeyAuthenticator(AppDbContext db, IServiceScopeFactory scopeFactory, ILogger<ApiKeyAuthenticator> logger) : IApiKeyAuthenticator
{
    public async Task<ApiKey?> AuthenticateAsync(string? rawKey, CancellationToken ct = default)
    {
        if (string.IsNullOrEmpty(rawKey))
        {
            return null;
        }

        var id = ApiKeyGenerator.ParseId(rawKey);
        if (id is null)
        {
            return null;
        }

        var key = await db.ApiKeys.AsNoTracking().FirstOrDefaultAsync(k => k.Id == id, ct);
        if (key is null)
        {
            return null;
        }

        if (!ApiKeyGenerator.Verify(rawKey, key.KeyHash))
        {
            return null;
        }

        // Fire-and-forget, igual que el "go func(){...}()" de la versión Go:
        // no debe bloquear ni fallar la request por esto.
        _ = TouchLastUsedAsync(key.Id);

        return key;
    }

    private async Task TouchLastUsedAsync(Guid id)
    {
        try
        {
            // No se puede reusar el DbContext inyectado (scoped a esta
            // request): para cuando termine esta tarea en background, ya
            // pudo haber sido dispuesto. Se crea un scope propio, igual que
            // el "go func(){...}()" de la versión Go corre en su propia
            // goroutine con su propia conexión del pool.
            using var scope = scopeFactory.CreateScope();
            var scopedDb = scope.ServiceProvider.GetRequiredService<AppDbContext>();
            await scopedDb.ApiKeys
                .Where(k => k.Id == id)
                .ExecuteUpdateAsync(setters => setters.SetProperty(k => k.LastUsed, DateTimeOffset.UtcNow));
        }
        catch (Exception ex)
        {
            logger.LogWarning(ex, "no se pudo actualizar last_used para la API key {Id}", id);
        }
    }
}
