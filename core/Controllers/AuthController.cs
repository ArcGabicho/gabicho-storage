using GabichoStorage.Api.Auth;
using GabichoStorage.Api.Data;
using GabichoStorage.Api.Dtos;
using GabichoStorage.Api.Entities;
using GabichoStorage.Api.Services;
using Microsoft.AspNetCore.Mvc;
using Microsoft.EntityFrameworkCore;

namespace GabichoStorage.Api.Controllers;

/// <summary>
/// Administración de API keys, protegida por master key (no por API key
/// normal): es el mecanismo de bootstrap para emitir las primeras keys.
/// </summary>
[ApiController]
[Route("api/auth/keys")]
[MasterKeyAuth]
public class AuthController(AppDbContext db, ILogger<AuthController> logger) : ControllerBase
{
    private const int MaxUserIdLength = 255;

    [HttpPost]
    public async Task<IActionResult> Create([FromBody] CreateApiKeyRequest request, CancellationToken ct)
    {
        var userId = request.UserId?.Trim() ?? "";
        if (userId.Length == 0)
        {
            return BadRequest(new ErrorResponse("user_id es obligatorio"));
        }
        if (userId.Length > MaxUserIdLength)
        {
            return BadRequest(new ErrorResponse("user_id demasiado largo"));
        }

        var permissions = string.IsNullOrWhiteSpace(request.Permissions)
            ? $"{Permission.Read},{Permission.Write}"
            : request.Permissions;

        foreach (var p in permissions.Split(',', StringSplitOptions.TrimEntries))
        {
            if (!Permission.Valid.Contains(p))
            {
                return BadRequest(new ErrorResponse($"permiso inválido: {p} (válidos: read, write, admin)"));
            }
        }

        var id = Guid.NewGuid();
        var (rawKey, hash) = ApiKeyGenerator.Generate(id);

        var apiKey = new ApiKey
        {
            Id = id,
            UserId = userId,
            KeyHash = hash,
            Permissions = permissions,
            CreatedAt = DateTimeOffset.UtcNow,
        };

        db.ApiKeys.Add(apiKey);
        await db.SaveChangesAsync(ct);

        logger.LogInformation("API key creada {Id} user_id={UserId} permissions={Permissions} ip={Ip}",
            apiKey.Id, apiKey.UserId, apiKey.Permissions, HttpContext.Connection.RemoteIpAddress);

        return StatusCode(StatusCodes.Status201Created, new CreateApiKeyResponse(
            apiKey.Id, apiKey.UserId, rawKey, apiKey.Permissions, apiKey.CreatedAt));
    }

    [HttpGet]
    public async Task<IActionResult> List([FromQuery(Name = "user_id")] string? userId, CancellationToken ct)
    {
        var query = db.ApiKeys.AsQueryable();
        if (!string.IsNullOrEmpty(userId))
        {
            query = query.Where(k => k.UserId == userId);
        }

        var keys = await query
            .OrderByDescending(k => k.CreatedAt)
            .Select(k => new ApiKeyResponse(k.Id, k.UserId, k.Permissions, k.CreatedAt, k.LastUsed))
            .ToListAsync(ct);

        return Ok(new { apiKeys = keys });
    }

    [HttpDelete("{keyId:guid}")]
    public async Task<IActionResult> Revoke(Guid keyId, CancellationToken ct)
    {
        var key = await db.ApiKeys.FirstOrDefaultAsync(k => k.Id == keyId, ct);
        if (key is null)
        {
            return NotFound(new ErrorResponse("API key no encontrada"));
        }

        db.ApiKeys.Remove(key);
        await db.SaveChangesAsync(ct);

        logger.LogInformation("API key revocada {Id} ip={Ip}", keyId, HttpContext.Connection.RemoteIpAddress);

        return NoContent();
    }
}
