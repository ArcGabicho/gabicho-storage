using Microsoft.Data.SqlClient;

namespace GabichoStorage.Api.Options;

/// <summary>
/// Configuración runtime del servicio, cargada desde variables de entorno
/// (con soporte opcional de un archivo .env para desarrollo local, vía
/// DotNetEnv). Equivalente a config.Config de la versión en Go.
/// </summary>
public class StorageOptions
{
    // Servidor
    public string CorsOrigin { get; set; } = "*";
    public string Environment { get; set; } = "production";

    // Base de datos
    public string DbHost { get; set; } = "localhost";
    public string DbPort { get; set; } = "1433";
    public string DbUser { get; set; } = "sa";
    public string DbPassword { get; set; } = "";
    public string DbName { get; set; } = "storage";
    public bool DbEncrypt { get; set; } = true;
    public bool DbTrustServerCertificate { get; set; } = true;

    // Almacenamiento
    public string StoragePath { get; set; } = "/data/storage";
    public long MaxFileSizeBytes { get; set; }

    // Seguridad
    public int TokenExpiryMinutes { get; set; } = 15;
    public string SigningSecret { get; set; } = "";
    public string MasterKey { get; set; } = "";

    // Rate limiting: ApiRateLimit* aplica a toda /api/* (protección general
    // anti-abuso). UploadRateLimit* es un límite adicional y más estricto
    // sólo para /api/upload/{bucket}, clave por API key (no por IP).
    public int ApiRateLimitMax { get; set; } = 300;
    public int ApiRateLimitWindowSeconds { get; set; } = 60;
    public int UploadRateLimitMax { get; set; } = 30;
    public int UploadRateLimitWindowSeconds { get; set; } = 60;

    public string[] CorsOrigins()
    {
        var origins = CorsOrigin
            .Split(',', StringSplitOptions.RemoveEmptyEntries | StringSplitOptions.TrimEntries);
        return origins.Length > 0 ? origins : ["*"];
    }

    /// <summary>
    /// Connection string de SQL Server, construida con SqlConnectionStringBuilder
    /// (escapa correctamente cualquier caracter especial en el password, a
    /// diferencia de interpolar un string a mano).
    /// </summary>
    public string ConnectionString()
    {
        var builder = new SqlConnectionStringBuilder
        {
            DataSource = $"{DbHost},{DbPort}",
            InitialCatalog = DbName,
            UserID = DbUser,
            Password = DbPassword,
            Encrypt = DbEncrypt,
            TrustServerCertificate = DbTrustServerCertificate,
        };
        return builder.ConnectionString;
    }

    /// <summary>
    /// Carga la configuración desde el entorno. Si existe un archivo .env
    /// en el directorio de trabajo, sus valores se cargan primero (sin
    /// sobrescribir variables ya presentes en el entorno real).
    /// </summary>
    public static StorageOptions Load()
    {
        DotNetEnv.Env.TraversePath().NoEnvVars().Load();

        var options = new StorageOptions
        {
            CorsOrigin = GetEnv("CORS_ORIGIN", "*"),
            Environment = GetEnv("ENVIRONMENT", "production"),

            DbHost = GetEnv("DB_HOST", "localhost"),
            DbPort = GetEnv("DB_PORT", "1433"),
            DbUser = GetEnv("DB_USER", "sa"),
            DbName = GetEnv("DB_NAME", "storage"),
            DbEncrypt = GetEnvBool("DB_ENCRYPT", true),
            DbTrustServerCertificate = GetEnvBool("DB_TRUST_SERVER_CERTIFICATE", true),

            StoragePath = GetEnv("STORAGE_PATH", "/data/storage"),
        };

        // Secretos: soportan la convención "_FILE" (Docker secrets) con
        // fallback a la variable de entorno directa. Nunca deben tener un
        // valor por defecto.
        options.DbPassword = GetSecret("DB_PASSWORD");
        options.SigningSecret = GetSecret("SIGNING_SECRET");
        options.MasterKey = GetSecret("ADMIN_MASTER_KEY");

        var maxFileSizeMb = GetEnvInt("MAX_FILE_SIZE_MB", 500);
        options.MaxFileSizeBytes = maxFileSizeMb * 1024L * 1024L;

        options.TokenExpiryMinutes = GetEnvInt("TOKEN_EXPIRY_MINUTES", 15);
        options.ApiRateLimitMax = GetEnvInt("API_RATE_LIMIT_MAX", 300);
        options.ApiRateLimitWindowSeconds = GetEnvInt("API_RATE_LIMIT_WINDOW_SECONDS", 60);
        options.UploadRateLimitMax = GetEnvInt("UPLOAD_RATE_LIMIT_MAX", 30);
        options.UploadRateLimitWindowSeconds = GetEnvInt("UPLOAD_RATE_LIMIT_WINDOW_SECONDS", 60);

        if (string.IsNullOrEmpty(options.DbPassword))
        {
            throw new InvalidOperationException("DB_PASSWORD (o DB_PASSWORD_FILE) es obligatorio");
        }
        if (string.IsNullOrEmpty(options.SigningSecret))
        {
            throw new InvalidOperationException("SIGNING_SECRET (o SIGNING_SECRET_FILE) es obligatorio (usado para firmar tokens presignados)");
        }
        if (string.IsNullOrEmpty(options.MasterKey))
        {
            throw new InvalidOperationException("ADMIN_MASTER_KEY (o ADMIN_MASTER_KEY_FILE) es obligatorio (usado para administrar API keys)");
        }

        return options;
    }

    private static string GetEnv(string key, string fallback)
    {
        var value = System.Environment.GetEnvironmentVariable(key);
        return string.IsNullOrEmpty(value) ? fallback : value;
    }

    private static bool GetEnvBool(string key, bool fallback)
    {
        var value = System.Environment.GetEnvironmentVariable(key);
        return string.IsNullOrEmpty(value) ? fallback : bool.Parse(value);
    }

    private static int GetEnvInt(string key, int fallback)
    {
        var value = System.Environment.GetEnvironmentVariable(key);
        if (string.IsNullOrEmpty(value))
        {
            return fallback;
        }
        if (!int.TryParse(value, out var parsed))
        {
            throw new InvalidOperationException($"{key} inválido: {value}");
        }
        return parsed;
    }

    /// <summary>
    /// Resuelve un valor sensible con la convención "_FILE" que usan las
    /// imágenes oficiales de Docker (ej. POSTGRES_PASSWORD_FILE): si
    /// KEY_FILE está seteada, lee el secreto desde ese archivo -- pensado
    /// para Docker secrets, montados en /run/secrets/&lt;nombre&gt; y por lo
    /// tanto NUNCA visibles vía `docker inspect` ni `docker compose config`
    /// (a diferencia de un valor puesto directo en `environment:`). Si no
    /// está seteada, cae a la variable de entorno KEY directa.
    /// </summary>
    private static string GetSecret(string key)
    {
        var filePath = System.Environment.GetEnvironmentVariable($"{key}_FILE");
        if (!string.IsNullOrEmpty(filePath))
        {
            try
            {
                return File.ReadAllText(filePath).Trim();
            }
            catch (Exception ex)
            {
                throw new InvalidOperationException($"leyendo {key}_FILE ({filePath}): {ex.Message}", ex);
            }
        }
        return GetEnv(key, "");
    }
}
