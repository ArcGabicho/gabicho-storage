using System.Buffers.Text;
using System.Security.Cryptography;

namespace GabichoStorage.Api.Services;

/// <summary>
/// Genera y verifica API keys con formato "sk_&lt;id&gt;_&lt;secret&gt;". El
/// id permite un lookup O(1) en base de datos sin necesitar comparar bcrypt
/// contra todas las keys existentes; sólo el secreto (nunca el id, que no es
/// sensible) se hashea y verifica con bcrypt, tanto porque es la parte
/// realmente confidencial como para no exceder el límite de 72 bytes de
/// entrada que impone bcrypt (la key completa "sk_&lt;uuid&gt;_&lt;secret&gt;"
/// lo supera).
/// </summary>
public static class ApiKeyGenerator
{
    private const string Prefix = "sk";

    public static (string RawKey, string Hash) Generate(Guid id)
    {
        var secretBytes = RandomNumberGenerator.GetBytes(32);
        var secret = Base64Url.EncodeToString(secretBytes);

        var rawKey = $"{Prefix}_{id}_{secret}";
        var hash = BCrypt.Net.BCrypt.HashPassword(secret);

        return (rawKey, hash);
    }

    /// <summary>
    /// Extrae el id embebido en una raw key sin verificarla, para poder
    /// localizar el registro correspondiente en base de datos.
    /// </summary>
    public static Guid? ParseId(string rawKey)
    {
        var parts = rawKey.Split('_', 3);
        if (parts.Length != 3 || parts[0] != Prefix)
        {
            return null;
        }
        return Guid.TryParse(parts[1], out var id) ? id : null;
    }

    /// <summary>Compara una raw key contra el hash bcrypt de su secreto.</summary>
    public static bool Verify(string rawKey, string hash)
    {
        var parts = rawKey.Split('_', 3);
        if (parts.Length != 3 || parts[0] != Prefix)
        {
            return false;
        }
        var secret = parts[2];
        try
        {
            return BCrypt.Net.BCrypt.Verify(secret, hash);
        }
        catch (BCrypt.Net.SaltParseException)
        {
            return false;
        }
    }
}
