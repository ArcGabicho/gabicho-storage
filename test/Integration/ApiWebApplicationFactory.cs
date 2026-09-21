using GabichoStorage.Api.Data;
using Microsoft.AspNetCore.Hosting;
using Microsoft.AspNetCore.Mvc.Testing;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;

namespace GabichoStorage.Tests.Integration;

/// <summary>
/// Levanta la app completa (misma que main(), vía el Program.cs real) contra
/// una base de datos fresca en la instancia de SQL Server indicada por
/// TEST_DB_HOST/TEST_DB_PORT/TEST_DB_USER/TEST_DB_PASSWORD, y un storage en
/// un directorio temporal. Cada instancia de este fixture crea su PROPIA
/// base (nombre random), así que no hace falta limpiar entre tests: alcanza
/// con usar nombres de bucket/usuario únicos dentro de la misma instancia
/// compartida por una clase de tests (ver <see cref="TestDatabase"/>).
/// </summary>
public class ApiWebApplicationFactory : WebApplicationFactory<Program>
{
    public string StoragePath { get; } = Directory.CreateTempSubdirectory("gabicho-storage-tests-").FullName;
    public string MasterKey { get; } = "test-master-key";
    public string SigningSecret { get; } = "test-signing-secret";

    // Configurables DESPUÉS de construir la instancia (WebApplicationFactory
    // no arranca el host hasta el primer CreateClient()/Server, así que
    // alcanza con setearlas antes de eso). xUnit exige que un fixture tenga
    // un único constructor público -- y no considera valores default de C#
    // al resolver sus argumentos -- así que la única forma de parametrizar
    // este fixture (ver RateLimitingTests, que necesita límites bajos) sin
    // romper IClassFixture<T> es vía propiedades, no argumentos de ctor.
    public int ApiRateLimitMax { get; set; } = 100_000;
    public int UploadRateLimitMax { get; set; } = 100_000;
    public long MaxFileSizeMb { get; set; } = 5;

    private readonly string _databaseName = $"storage_test_{Guid.NewGuid():N}";

    protected override void ConfigureWebHost(IWebHostBuilder builder)
    {
        Environment.SetEnvironmentVariable("DB_HOST", TestDatabase.Host);
        Environment.SetEnvironmentVariable("DB_PORT", TestDatabase.Port);
        Environment.SetEnvironmentVariable("DB_USER", TestDatabase.User);
        Environment.SetEnvironmentVariable("DB_PASSWORD", TestDatabase.Password);
        Environment.SetEnvironmentVariable("DB_NAME", _databaseName);
        Environment.SetEnvironmentVariable("DB_ENCRYPT", "false");
        Environment.SetEnvironmentVariable("DB_TRUST_SERVER_CERTIFICATE", "true");
        Environment.SetEnvironmentVariable("STORAGE_PATH", StoragePath);
        Environment.SetEnvironmentVariable("SIGNING_SECRET", SigningSecret);
        Environment.SetEnvironmentVariable("ADMIN_MASTER_KEY", MasterKey);
        Environment.SetEnvironmentVariable("CORS_ORIGIN", "https://mi-app.vercel.app");
        Environment.SetEnvironmentVariable("MAX_FILE_SIZE_MB", MaxFileSizeMb.ToString());
        Environment.SetEnvironmentVariable("TOKEN_EXPIRY_MINUTES", "15");
        Environment.SetEnvironmentVariable("API_RATE_LIMIT_MAX", ApiRateLimitMax.ToString());
        Environment.SetEnvironmentVariable("API_RATE_LIMIT_WINDOW_SECONDS", "60");
        Environment.SetEnvironmentVariable("UPLOAD_RATE_LIMIT_MAX", UploadRateLimitMax.ToString());
        Environment.SetEnvironmentVariable("UPLOAD_RATE_LIMIT_WINDOW_SECONDS", "60");

        builder.UseEnvironment("Testing");
    }

    private static readonly string[] EnvKeysToClear =
    [
        "DB_HOST", "DB_PORT", "DB_USER", "DB_PASSWORD", "DB_NAME",
        "DB_ENCRYPT", "DB_TRUST_SERVER_CERTIFICATE", "STORAGE_PATH",
        "SIGNING_SECRET", "ADMIN_MASTER_KEY", "CORS_ORIGIN",
        "MAX_FILE_SIZE_MB", "TOKEN_EXPIRY_MINUTES",
        "API_RATE_LIMIT_MAX", "API_RATE_LIMIT_WINDOW_SECONDS",
        "UPLOAD_RATE_LIMIT_MAX", "UPLOAD_RATE_LIMIT_WINDOW_SECONDS",
    ];

    protected override void Dispose(bool disposing)
    {
        if (disposing)
        {
            // Tiene que ir ANTES de base.Dispose(): eso tira abajo el host y
            // Services deja de ser utilizable.
            try
            {
                using var scope = Services.CreateScope();
                var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
                db.Database.EnsureDeleted();
            }
            catch
            {
                // best-effort: si falla la limpieza no debe tumbar la corrida de tests.
            }
        }

        base.Dispose(disposing);

        if (disposing)
        {
            try
            {
                Directory.Delete(StoragePath, recursive: true);
            }
            catch
            {
                // ídem.
            }

            // No dejar las variables seteadas para otros tests que corran
            // después en el mismo proceso (ver StorageOptionsTests, que se
            // topó con esto durante el desarrollo: veía el
            // API_RATE_LIMIT_MAX que esta clase dejaba puesto).
            foreach (var key in EnvKeysToClear)
            {
                Environment.SetEnvironmentVariable(key, null);
            }
        }
    }
}
