using GabichoStorage.Api.Auth;
using GabichoStorage.Api.Data;
using GabichoStorage.Api.Dtos;
using GabichoStorage.Api.Entities;
using GabichoStorage.Api.Services;
using Microsoft.AspNetCore.Mvc;
using Microsoft.EntityFrameworkCore;

namespace GabichoStorage.Api.Controllers;

[ApiController]
[Route("api/buckets")]
[ApiKeyAuth]
public class BucketsController(AppDbContext db, ILocalFileStorage storage, ILogger<BucketsController> logger) : ControllerBase
{
    [HttpPost]
    [RequirePermission(Permission.Write)]
    public async Task<IActionResult> Create([FromBody] CreateBucketRequest request, CancellationToken ct)
    {
        var key = HttpContext.GetApiKey()!;

        if (!FilenameSanitizer.IsValidBucketName(request.Name))
        {
            return BadRequest(new ErrorResponse("nombre de bucket inválido: debe tener 3-63 caracteres, minúsculas, dígitos y guiones"));
        }

        if (await db.Buckets.AnyAsync(b => b.Name == request.Name, ct))
        {
            return Conflict(new ErrorResponse("ya existe un bucket con ese nombre"));
        }

        storage.EnsureBucketDirectory(request.Name);

        var bucket = new Bucket
        {
            Id = Guid.NewGuid(),
            Name = request.Name,
            Owner = key.UserId,
            IsPublic = request.IsPublic,
            CreatedAt = DateTimeOffset.UtcNow,
        };

        db.Buckets.Add(bucket);

        try
        {
            await db.SaveChangesAsync(ct);
        }
        catch (DbUpdateException)
        {
            // Condición de carrera: otro request creó el mismo nombre entre
            // el chequeo de arriba y este insert -- el índice único de la
            // columna Name es lo que realmente lo previene.
            if (await db.Buckets.AnyAsync(b => b.Name == request.Name, ct))
            {
                return Conflict(new ErrorResponse("ya existe un bucket con ese nombre"));
            }
            throw;
        }

        return StatusCode(StatusCodes.Status201Created, ToResponse(bucket));
    }

    [HttpGet]
    [RequirePermission(Permission.Read)]
    public async Task<IActionResult> List(CancellationToken ct)
    {
        var key = HttpContext.GetApiKey()!;

        var buckets = await db.Buckets
            .Where(b => b.Owner == key.UserId)
            .OrderByDescending(b => b.CreatedAt)
            .Select(b => ToResponse(b))
            .ToListAsync(ct);

        return Ok(new { buckets });
    }

    [HttpDelete("{id:guid}")]
    [RequirePermission(Permission.Write)]
    public async Task<IActionResult> Delete(Guid id, CancellationToken ct)
    {
        var key = HttpContext.GetApiKey()!;

        var bucket = await db.Buckets.FirstOrDefaultAsync(b => b.Id == id, ct);
        if (bucket is null)
        {
            return NotFound(new ErrorResponse("bucket no encontrado"));
        }

        if (bucket.Owner != key.UserId && !key.HasPermission(Permission.Admin))
        {
            return StatusCode(StatusCodes.Status403Forbidden, new ErrorResponse("no sos el dueño de este bucket"));
        }

        db.Buckets.Remove(bucket); // cascada a Files vía FK (OnDelete Cascade)
        await db.SaveChangesAsync(ct);

        storage.RemoveBucketDirectory(bucket.Name);

        logger.LogInformation("bucket eliminado {BucketId} {BucketName} owner={Owner} deletedBy={DeletedBy}",
            bucket.Id, bucket.Name, bucket.Owner, key.UserId);

        return NoContent();
    }

    private static BucketResponse ToResponse(Bucket b) => new(b.Id, b.Name, b.Owner, b.IsPublic, b.CreatedAt);
}
