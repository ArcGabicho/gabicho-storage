using GabichoStorage.Api.Options;

namespace GabichoStorage.Tests.Unit;

/// <summary>
/// Estos tests manipulan variables de entorno del proceso: no pueden correr
/// en paralelo entre sí (xUnit no paraleliza tests dentro de la misma clase
/// por default, así que alcanza con evitar [Collection] compartidas).
/// </summary>
[Collection("EnvironmentVariables")]
public class StorageOptionsTests : IDisposable
{
    // Todas las variables que StorageOptions.Load() puede llegar a leer.
    // Se resetean a null en el constructor (no sólo en Dispose) porque
    // ApiWebApplicationFactory (usada por los tests de integración, misma
    // collection) las deja seteadas process-wide después de correr y nunca
    // las limpia -- sin este reset, un test de esta clase podía terminar
    // viendo, por ejemplo, el API_RATE_LIMIT_MAX que dejó puesto otra clase
    // de test, en vez del default esperado.
    private static readonly string[] AllOptionKeys =
    [
        "CORS_ORIGIN", "ENVIRONMENT",
        "DB_HOST", "DB_PORT", "DB_USER", "DB_PASSWORD", "DB_PASSWORD_FILE", "DB_NAME",
        "DB_ENCRYPT", "DB_TRUST_SERVER_CERTIFICATE",
        "STORAGE_PATH", "MAX_FILE_SIZE_MB",
        "TOKEN_EXPIRY_MINUTES", "SIGNING_SECRET", "SIGNING_SECRET_FILE",
        "ADMIN_MASTER_KEY", "ADMIN_MASTER_KEY_FILE",
        "API_RATE_LIMIT_MAX", "API_RATE_LIMIT_WINDOW_SECONDS",
        "UPLOAD_RATE_LIMIT_MAX", "UPLOAD_RATE_LIMIT_WINDOW_SECONDS",
    ];

    public StorageOptionsTests()
    {
        foreach (var key in AllOptionKeys)
        {
            Environment.SetEnvironmentVariable(key, null);
        }
    }

    private readonly List<string> _setVars = [];

    private void SetEnv(string key, string? value)
    {
        Environment.SetEnvironmentVariable(key, value);
        _setVars.Add(key);
    }

    private void SetRequiredEnv()
    {
        SetEnv("DB_PASSWORD", "secret");
        SetEnv("SIGNING_SECRET", "signing-secret");
        SetEnv("ADMIN_MASTER_KEY", "master-key");
    }

    public void Dispose()
    {
        foreach (var key in _setVars)
        {
            Environment.SetEnvironmentVariable(key, null);
        }
        GC.SuppressFinalize(this);
    }

    [Fact]
    public void Load_Succeeds_WithRequiredVarsSet()
    {
        SetRequiredEnv();

        var options = StorageOptions.Load();

        Assert.Equal("secret", options.DbPassword);
        Assert.Equal(500L * 1024 * 1024, options.MaxFileSizeBytes);
        Assert.Equal(15, options.TokenExpiryMinutes);
        Assert.Equal(300, options.ApiRateLimitMax);
    }

    [Theory]
    [InlineData("DB_PASSWORD")]
    [InlineData("SIGNING_SECRET")]
    [InlineData("ADMIN_MASTER_KEY")]
    public void Load_Throws_WhenRequiredVarMissing(string missingVar)
    {
        SetRequiredEnv();
        SetEnv(missingVar, "");

        Assert.Throws<InvalidOperationException>(() => StorageOptions.Load());
    }

    [Fact]
    public void Load_Throws_OnInvalidIntEnvVar()
    {
        SetRequiredEnv();
        SetEnv("MAX_FILE_SIZE_MB", "not-a-number");

        Assert.Throws<InvalidOperationException>(() => StorageOptions.Load());
    }

    [Fact]
    public void Load_ReadsSecretFromFile_ViaFileSuffix()
    {
        SetRequiredEnv();

        var dir = Directory.CreateTempSubdirectory();
        var path = Path.Combine(dir.FullName, "db_password");
        File.WriteAllText(path, "secret-from-file\n");
        SetEnv("DB_PASSWORD_FILE", path);

        var options = StorageOptions.Load();

        Assert.Equal("secret-from-file", options.DbPassword);
    }

    [Fact]
    public void Load_SecretFile_TakesPriorityOverPlainVar()
    {
        SetRequiredEnv();

        var dir = Directory.CreateTempSubdirectory();
        var path = Path.Combine(dir.FullName, "db_password");
        File.WriteAllText(path, "from-file-wins");
        SetEnv("DB_PASSWORD_FILE", path);

        var options = StorageOptions.Load();

        Assert.Equal("from-file-wins", options.DbPassword);
    }

    [Fact]
    public void Load_Throws_WhenSecretFileMissing()
    {
        SetRequiredEnv();
        SetEnv("DB_PASSWORD_FILE", "/no/existe/este/archivo");

        Assert.Throws<InvalidOperationException>(() => StorageOptions.Load());
    }

    [Fact]
    public void ConnectionString_EscapesSpecialCharacters()
    {
        var options = new StorageOptions
        {
            DbHost = "sqlserver",
            DbPort = "1433",
            DbUser = "sa",
            DbPassword = "p@ss;w=rd'1",
            DbName = "storage",
        };

        var connectionString = options.ConnectionString();
        var builder = new Microsoft.Data.SqlClient.SqlConnectionStringBuilder(connectionString);

        Assert.Equal("p@ss;w=rd'1", builder.Password);
        Assert.Equal("storage", builder.InitialCatalog);
    }

    [Theory]
    [InlineData("https://example.com", new[] { "https://example.com" })]
    [InlineData("https://a.com,https://b.com", new[] { "https://a.com", "https://b.com" })]
    [InlineData("https://a.com, https://b.com , https://c.com", new[] { "https://a.com", "https://b.com", "https://c.com" })]
    [InlineData("*", new[] { "*" })]
    [InlineData("", new[] { "*" })]
    public void CorsOrigins_ParsesCommaSeparatedList(string input, string[] expected)
    {
        var options = new StorageOptions { CorsOrigin = input };
        Assert.Equal(expected, options.CorsOrigins());
    }
}
