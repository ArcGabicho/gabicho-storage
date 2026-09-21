using System.Threading.RateLimiting;
using GabichoStorage.Api.Auth;
using GabichoStorage.Api.Data;
using GabichoStorage.Api.Dtos;
using GabichoStorage.Api.Middleware;
using GabichoStorage.Api.Options;
using GabichoStorage.Api.Services;
using Microsoft.AspNetCore.Http.Features;
using Microsoft.AspNetCore.RateLimiting;
using Microsoft.EntityFrameworkCore;

var builder = WebApplication.CreateBuilder(args);

var storageOptions = StorageOptions.Load();
builder.Services.AddSingleton(Microsoft.Extensions.Options.Options.Create(storageOptions));

// --- Logging estructurado en JSON (equivalente a slog.NewJSONHandler) ---
builder.Logging.ClearProviders();
builder.Logging.AddJsonConsole(o =>
{
    o.TimestampFormat = "yyyy-MM-ddTHH:mm:ss.fffZ";
    o.UseUtcTimestamp = true;
    o.JsonWriterOptions = new System.Text.Json.JsonWriterOptions { Indented = false };
});

// --- Base de datos ---
builder.Services.AddDbContext<AppDbContext>(o => o.UseSqlServer(storageOptions.ConnectionString()));

// --- Servicios de dominio ---
builder.Services.AddSingleton<ILocalFileStorage, LocalFileStorage>();
builder.Services.AddSingleton<ITokenSigner, TokenSigner>();
builder.Services.AddScoped<IApiKeyAuthenticator, ApiKeyAuthenticator>();

// --- Kestrel / multipart: límite con margen sobre MaxFileSizeBytes ---
// Si el límite de transporte fuera exactamente igual a MaxFileSizeBytes, un
// archivo más grande cortaría la conexión ANTES de que la request llegue al
// controller, y el cliente nunca vería el 413 con JSON prolijo que devuelve
// el propio endpoint de upload -- sólo un connection reset. El límite de
// Kestrel queda como techo duro contra bodies verdaderamente abusivos.
var effectiveBodyLimit = storageOptions.MaxFileSizeBytes + 1 * 1024 * 1024;
builder.Services.Configure<Microsoft.AspNetCore.Server.Kestrel.Core.KestrelServerOptions>(o =>
{
    o.Limits.MaxRequestBodySize = effectiveBodyLimit;
});
builder.Services.Configure<FormOptions>(o =>
{
    o.MultipartBodyLengthLimit = effectiveBodyLimit;
});

// --- CORS ---
const string CorsPolicyName = "configured-origins";
builder.Services.AddCors(o =>
{
    o.AddPolicy(CorsPolicyName, policy =>
    {
        var origins = storageOptions.CorsOrigins();
        if (origins.Length == 1 && origins[0] == "*")
        {
            policy.AllowAnyOrigin();
        }
        else
        {
            policy.WithOrigins(origins);
        }

        policy.WithMethods("GET", "POST", "DELETE", "OPTIONS")
            .WithHeaders("Content-Type", "Authorization", "X-API-Key", "X-Master-Key")
            .WithExposedHeaders("X-RateLimit-Limit", "X-RateLimit-Remaining", "X-RateLimit-Reset")
            .SetPreflightMaxAge(TimeSpan.FromSeconds(300));
    });
});

// --- Rate limiting: general sobre /api/* + uno más estricto para upload ---
builder.Services.AddRateLimiter(o =>
{
    // El default de ASP.NET Core es 503; el estándar para "excediste el
    // rate limit" (y lo que devolvía la versión en Go) es 429.
    o.RejectionStatusCode = StatusCodes.Status429TooManyRequests;

    o.OnRejected = async (context, ct) =>
    {
        context.HttpContext.Response.Headers.RetryAfter = context.Lease.TryGetMetadata(MetadataName.RetryAfter, out var retryAfter)
            ? ((int)retryAfter.TotalSeconds).ToString()
            : storageOptions.ApiRateLimitWindowSeconds.ToString();

        context.HttpContext.Response.ContentType = "application/json";
        await context.HttpContext.Response.WriteAsJsonAsync(
            new ErrorResponse("rate limit excedido, reintentá más tarde"), ct);
    };

    o.AddPolicy("api", httpContext => RateLimitPartition.GetFixedWindowLimiter(
        partitionKey: RateLimitKey(httpContext),
        factory: _ => new FixedWindowRateLimiterOptions
        {
            PermitLimit = storageOptions.ApiRateLimitMax,
            Window = TimeSpan.FromSeconds(storageOptions.ApiRateLimitWindowSeconds),
            QueueLimit = 0,
        }));

    o.AddPolicy("upload", httpContext => RateLimitPartition.GetFixedWindowLimiter(
        partitionKey: RateLimitKey(httpContext),
        factory: _ => new FixedWindowRateLimiterOptions
        {
            PermitLimit = storageOptions.UploadRateLimitMax,
            Window = TimeSpan.FromSeconds(storageOptions.UploadRateLimitWindowSeconds),
            QueueLimit = 0,
        }));
});

builder.Services.AddControllers().AddJsonOptions(o =>
{
    // snake_case (user_id, is_public, created_at, ...) para mantener el
    // mismo contrato que tenía la versión original en Go.
    o.JsonSerializerOptions.PropertyNamingPolicy = new SnakeCaseNamingPolicy();
    o.JsonSerializerOptions.DictionaryKeyPolicy = null; // no tocar las keys de "metadata", son datos de usuario
});
builder.Services.Configure<Microsoft.AspNetCore.Http.Json.JsonOptions>(o =>
{
    o.SerializerOptions.PropertyNamingPolicy = new SnakeCaseNamingPolicy();
});
builder.Services.AddOpenApi();

var app = builder.Build();

// --- Migraciones al arrancar ---
using (var scope = app.Services.CreateScope())
{
    var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
    db.Database.Migrate();
}

if (app.Environment.IsDevelopment())
{
    app.MapOpenApi();
}

// --- Manejo global de excepciones: nunca devolver detalles internos al
// cliente (stack traces, mensajes crudos de SQL/filesystem); el detalle
// completo va sólo a los logs del servidor. ---
app.UseExceptionHandler(errorApp =>
{
    errorApp.Run(async context =>
    {
        var feature = context.Features.Get<Microsoft.AspNetCore.Diagnostics.IExceptionHandlerFeature>();
        if (feature is not null)
        {
            var logger = context.RequestServices.GetRequiredService<ILoggerFactory>().CreateLogger("UnhandledException");
            logger.LogError(feature.Error, "excepción no manejada en {Path}", context.Request.Path);
        }

        context.Response.StatusCode = StatusCodes.Status500InternalServerError;
        context.Response.ContentType = "application/json";
        await context.Response.WriteAsJsonAsync(new ErrorResponse("error interno del servidor"));
    });
});

app.UseMiddleware<CloudflareRealIpMiddleware>();
app.UseMiddleware<RequestLoggingMiddleware>();
app.UseMiddleware<SecurityHeadersMiddleware>();

app.UseRouting();
app.UseCors(CorsPolicyName);
app.UseRateLimiter();

app.MapControllers().RequireRateLimiting("api");
app.MapGet("/health", () => Results.Ok(new { status = "ok" }));

app.Run();

/// <summary>
/// Identifica al llamante para el rate limiter: por la raw API key del
/// header si vino una (no hace falta validarla contra la base sólo para
/// particionar, alcanza con que sea consistente por llamador), o por IP si
/// no. Evita que todos los usuarios de un frontend que proxea requests
/// (ej. Next.js en Vercel) compartan el mismo balde de IP.
/// </summary>
static string RateLimitKey(HttpContext context)
{
    var rawKey = context.ExtractRawApiKey();
    if (!string.IsNullOrEmpty(rawKey))
    {
        return $"key:{rawKey}";
    }
    return $"ip:{context.Connection.RemoteIpAddress}";
}

public partial class Program;
