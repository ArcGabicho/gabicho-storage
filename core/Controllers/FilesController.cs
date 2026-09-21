using System.Text.Json;
using GabichoStorage.Api.Auth;
using GabichoStorage.Api.Data;
using GabichoStorage.Api.Dtos;
using GabichoStorage.Api.Entities;
using GabichoStorage.Api.Options;
using GabichoStorage.Api.Services;
using Microsoft.AspNetCore.Mvc;
using Microsoft.AspNetCore.RateLimiting;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Options;

namespace GabichoStorage.Api.Controllers;

[ApiController]
[Route("api")]
public class FilesController(
    AppDbContext db,
    ILocalFileStorage storage,
    ITokenSigner signer,
    IOptions<StorageOptions> options,
    ILogger<FilesController> logger) : ControllerBase
{
    private readonly StorageOptions _options = options.Value;
    private const int SniffLength = 512;

    // --- Upload ---------------------------------------------------------

    [HttpPost("upload/{bucket}")]
    [ApiKeyAuth]
    [RequirePermission(Permission.Write)]
    [EnableRateLimiting("upload")]
    [Consumes("multipart/form-data")]
    public async Task<IActionResult> Upload(
        string bucket,
        [FromForm(Name = "file")] IFormFile? file,
        [FromForm(Name = "public")] string? isPublicRaw,
        [FromForm] string? metadata,
        CancellationToken ct)
    {
        var key = HttpContext.GetApiKey()!;

        var bucketEntity = await db.Buckets.FirstOrDefaultAsync(b => b.Name == bucket, ct);
        if (bucketEntity is null)
        {
            return NotFound(new ErrorResponse("bucket no encontrado"));
        }

        if (bucketEntity.Owner != key.UserId && !key.HasPermission(Permission.Admin))
        {
            return StatusCode(StatusCodes.Status403Forbidden, new ErrorResponse("no tenés permiso de escritura sobre este bucket"));
        }

        if (file is null || file.Length == 0)
        {
            return BadRequest(new ErrorResponse("falta el campo 'file' en el multipart/form-data"));
        }

        var sizeError = FilenameSanitizer.ValidateFileSize(file.Length, _options.MaxFileSizeBytes);
        if (sizeError is not null)
        {
            return StatusCode(StatusCodes.Status413PayloadTooLarge, new ErrorResponse(sizeError));
        }

        var mimeType = file.ContentType;
        if (!MimeValidator.IsAllowedMimeType(mimeType))
        {
            return StatusCode(StatusCodes.Status415UnsupportedMediaType, new ErrorResponse($"tipo de archivo no permitido: {mimeType}"));
        }

        var sniffed = await ReadSniffBytesAsync(file, ct);
        if (MimeValidator.LooksLikeHtml(sniffed))
        {
            return StatusCode(StatusCodes.Status415UnsupportedMediaType, new ErrorResponse("el contenido del archivo no coincide con el tipo declarado"));
        }

        var originalName = FilenameSanitizer.Sanitize(file.FileName);

        var isPublic = bucketEntity.IsPublic;
        if (!string.IsNullOrEmpty(isPublicRaw))
        {
            if (!bool.TryParse(isPublicRaw, out isPublic))
            {
                return BadRequest(new ErrorResponse("el campo 'public' debe ser true/false"));
            }
        }

        var metadataDict = new Dictionary<string, object?>();
        if (!string.IsNullOrEmpty(metadata))
        {
            try
            {
                metadataDict = JsonSerializer.Deserialize<Dictionary<string, object?>>(metadata) ?? [];
            }
            catch (JsonException)
            {
                return BadRequest(new ErrorResponse("el campo 'metadata' debe ser JSON válido"));
            }
        }

        var extension = MimeValidator.ExtensionForMimeType(mimeType!);
        string storedName, relativePath;
        await using (var stream = file.OpenReadStream())
        {
            (storedName, relativePath) = await storage.SaveAsync(bucketEntity.Name, stream, extension, ct);
        }

        var fileEntity = new FileObject
        {
            Id = Guid.NewGuid(),
            BucketId = bucketEntity.Id,
            Filename = storedName,
            OriginalName = originalName,
            MimeType = mimeType!,
            Size = file.Length,
            Path = relativePath,
            IsPublic = isPublic,
            CreatedAt = DateTimeOffset.UtcNow,
            Metadata = metadataDict,
        };

        db.Files.Add(fileEntity);
        try
        {
            await db.SaveChangesAsync(ct);
        }
        catch (Exception)
        {
            storage.DeleteFile(relativePath);
            throw;
        }

        logger.LogInformation("archivo subido {FileId} bucket={Bucket} size={Size} mime={MimeType} user={UserId}",
            fileEntity.Id, bucketEntity.Name, fileEntity.Size, fileEntity.MimeType, key.UserId);

        return StatusCode(StatusCodes.Status201Created, new UploadResponse(
            ToResponse(fileEntity), $"/api/{bucketEntity.Name}/{fileEntity.Filename}"));
    }

    private static async Task<byte[]> ReadSniffBytesAsync(IFormFile file, CancellationToken ct)
    {
        await using var stream = file.OpenReadStream();
        var buffer = new byte[Math.Min(SniffLength, file.Length)];
        var read = 0;
        while (read < buffer.Length)
        {
            var n = await stream.ReadAsync(buffer.AsMemory(read, buffer.Length - read), ct);
            if (n == 0)
            {
                break;
            }
            read += n;
        }
        return read == buffer.Length ? buffer : buffer[..read];
    }

    // --- Listado ----------------------------------------------------------

    [HttpGet("files/{bucket}")]
    [ApiKeyAuth]
    [RequirePermission(Permission.Read)]
    public async Task<IActionResult> ListFiles(string bucket, [FromQuery] int page = 1, [FromQuery(Name = "page_size")] int pageSize = 20, CancellationToken ct = default)
    {
        var key = HttpContext.GetApiKey()!;

        var bucketEntity = await db.Buckets.FirstOrDefaultAsync(b => b.Name == bucket, ct);
        if (bucketEntity is null)
        {
            return NotFound(new ErrorResponse("bucket no encontrado"));
        }

        if (!bucketEntity.IsPublic && bucketEntity.Owner != key.UserId && !key.HasPermission(Permission.Admin))
        {
            return StatusCode(StatusCodes.Status403Forbidden, new ErrorResponse("no tenés acceso a este bucket"));
        }

        if (page < 1) page = 1;
        if (pageSize < 1) pageSize = 20;
        if (pageSize > 100) pageSize = 100;

        var query = db.Files.Where(f => f.BucketId == bucketEntity.Id);
        var total = await query.LongCountAsync(ct);
        var files = await query
            .OrderByDescending(f => f.CreatedAt)
            .Skip((page - 1) * pageSize)
            .Take(pageSize)
            .ToListAsync(ct);

        var totalPages = (int)Math.Ceiling(total / (double)pageSize);

        return Ok(new FileListResponse(files.Select(ToResponse).ToList(), page, pageSize, total, totalPages));
    }

    // --- Borrado ------------------------------------------------------------

    [HttpDelete("{bucket}/{filename}")]
    [ApiKeyAuth]
    [RequirePermission(Permission.Write)]
    public async Task<IActionResult> DeleteFile(string bucket, string filename, CancellationToken ct)
    {
        var key = HttpContext.GetApiKey()!;

        var bucketEntity = await db.Buckets.FirstOrDefaultAsync(b => b.Name == bucket, ct);
        if (bucketEntity is null)
        {
            return NotFound(new ErrorResponse("bucket no encontrado"));
        }

        if (bucketEntity.Owner != key.UserId && !key.HasPermission(Permission.Admin))
        {
            return StatusCode(StatusCodes.Status403Forbidden, new ErrorResponse("no tenés permiso de escritura sobre este bucket"));
        }

        var fileEntity = await db.Files.FirstOrDefaultAsync(f => f.BucketId == bucketEntity.Id && f.Filename == filename, ct);
        if (fileEntity is null)
        {
            return NotFound(new ErrorResponse("archivo no encontrado"));
        }

        db.Files.Remove(fileEntity);
        await db.SaveChangesAsync(ct);

        storage.DeleteFile(fileEntity.Path);

        return NoContent();
    }

    // --- Descarga directa (pública si el archivo es público, o con API key) ---

    [HttpGet("{bucket}/{filename}")]
    public async Task<IActionResult> DownloadFile(string bucket, string filename, CancellationToken ct)
    {
        var bucketEntity = await db.Buckets.FirstOrDefaultAsync(b => b.Name == bucket, ct);
        if (bucketEntity is null)
        {
            return NotFound(new ErrorResponse("bucket no encontrado"));
        }

        var fileEntity = await db.Files.FirstOrDefaultAsync(f => f.BucketId == bucketEntity.Id && f.Filename == filename, ct);
        if (fileEntity is null)
        {
            return NotFound(new ErrorResponse("archivo no encontrado"));
        }

        if (!fileEntity.IsPublic)
        {
            var rawKey = HttpContext.ExtractRawApiKey();
            ApiKey? key = null;
            if (!string.IsNullOrEmpty(rawKey))
            {
                var authenticator = HttpContext.RequestServices.GetRequiredService<IApiKeyAuthenticator>();
                key = await authenticator.AuthenticateAsync(rawKey, ct);
                if (key is null)
                {
                    return Unauthorized(new ErrorResponse("API key inválida"));
                }
            }

            if (key is null || (bucketEntity.Owner != key.UserId && !key.HasPermission(Permission.Admin)))
            {
                return StatusCode(StatusCodes.Status403Forbidden, new ErrorResponse("archivo privado: se requiere una API key con acceso"));
            }
        }

        return ServeFile(fileEntity);
    }

    // --- Links presignados ---------------------------------------------------

    [HttpPost("presign/{bucket}/{filename}")]
    [ApiKeyAuth]
    [RequirePermission(Permission.Read)]
    public async Task<IActionResult> Presign(string bucket, string filename, CancellationToken ct)
    {
        var key = HttpContext.GetApiKey()!;

        var bucketEntity = await db.Buckets.FirstOrDefaultAsync(b => b.Name == bucket, ct);
        if (bucketEntity is null)
        {
            return NotFound(new ErrorResponse("bucket no encontrado"));
        }

        if (bucketEntity.Owner != key.UserId && !key.HasPermission(Permission.Admin))
        {
            return StatusCode(StatusCodes.Status403Forbidden, new ErrorResponse("no tenés acceso a este bucket"));
        }

        var exists = await db.Files.AnyAsync(f => f.BucketId == bucketEntity.Id && f.Filename == filename, ct);
        if (!exists)
        {
            return NotFound(new ErrorResponse("archivo no encontrado"));
        }

        var ttl = TimeSpan.FromMinutes(_options.TokenExpiryMinutes);
        var token = signer.GenerateDownloadToken(bucketEntity.Name, filename, ttl);

        return Ok(new PresignResponse(token, $"/api/download/{token}", (int)ttl.TotalSeconds));
    }

    [HttpGet("download/{token}")]
    public async Task<IActionResult> PresignedDownload(string token, CancellationToken ct)
    {
        var validated = signer.ValidateDownloadToken(token);
        if (validated is null)
        {
            return Unauthorized(new ErrorResponse("token inválido o expirado"));
        }

        var (bucketName, filename) = validated.Value;

        var bucketEntity = await db.Buckets.FirstOrDefaultAsync(b => b.Name == bucketName, ct);
        if (bucketEntity is null)
        {
            return NotFound(new ErrorResponse("bucket no encontrado"));
        }

        var fileEntity = await db.Files.FirstOrDefaultAsync(f => f.BucketId == bucketEntity.Id && f.Filename == filename, ct);
        if (fileEntity is null)
        {
            return NotFound(new ErrorResponse("archivo no encontrado"));
        }

        return ServeFile(fileEntity);
    }

    // ---------------------------------------------------------------------

    private PhysicalFileResult ServeFile(FileObject fileEntity)
    {
        Response.Headers.ContentDisposition = $"inline; filename=\"{fileEntity.OriginalName}\"";
        return PhysicalFile(storage.FullPath(fileEntity.Path), fileEntity.MimeType);
    }

    private static FileResponse ToResponse(FileObject f) => new(
        f.Id, f.BucketId, f.Filename, f.OriginalName, f.MimeType, f.Size, f.IsPublic, f.CreatedAt, f.Metadata);
}
