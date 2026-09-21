namespace GabichoStorage.Api.Entities;

public static class Permission
{
    public const string Read = "read";
    public const string Write = "write";
    public const string Admin = "admin";

    public static readonly HashSet<string> Valid = [Read, Write, Admin];
}

/// <summary>
/// Credencial emitida para un usuario. El secreto en texto plano nunca se
/// persiste: sólo se guarda su hash bcrypt (KeyHash).
/// </summary>
public class ApiKey
{
    public Guid Id { get; set; }

    public required string UserId { get; set; }

    public required string KeyHash { get; set; }

    /// <summary>Lista separada por coma, ej. "read,write". "admin" implica todos los demás.</summary>
    public required string Permissions { get; set; }

    public DateTimeOffset CreatedAt { get; set; }

    public DateTimeOffset? LastUsed { get; set; }

    /// <summary>
    /// Indica si esta key tiene el permiso solicitado (admin implica todos
    /// los demás).
    /// </summary>
    public bool HasPermission(string permission)
    {
        foreach (var p in Permissions.Split(',', StringSplitOptions.TrimEntries))
        {
            if (p == Permission.Admin || p == permission)
            {
                return true;
            }
        }
        return false;
    }
}
