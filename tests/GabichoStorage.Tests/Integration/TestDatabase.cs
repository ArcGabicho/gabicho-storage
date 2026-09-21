namespace GabichoStorage.Tests.Integration;

/// <summary>
/// Configuración de la instancia de SQL Server contra la que corren los
/// tests de integración. Se levanta aparte (ver README.md, sección
/// "Tests"), no la maneja este proyecto.
/// </summary>
public static class TestDatabase
{
    public static string? Host => Environment.GetEnvironmentVariable("TEST_DB_HOST");
    public static string Port => Environment.GetEnvironmentVariable("TEST_DB_PORT") ?? "1433";
    public static string User => Environment.GetEnvironmentVariable("TEST_DB_USER") ?? "sa";
    public static string? Password => Environment.GetEnvironmentVariable("TEST_DB_PASSWORD");

    public static bool IsAvailable => !string.IsNullOrEmpty(Host) && !string.IsNullOrEmpty(Password);

    public const string SkipReason = "TEST_DB_HOST/TEST_DB_PASSWORD no seteadas: saltando test de integración con SQL Server";
}
