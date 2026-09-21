using System.Buffers.Text;
using System.Security.Cryptography;
using System.Text;
using GabichoStorage.Api.Options;
using Microsoft.Extensions.Options;

namespace GabichoStorage.Api.Services;

public class TokenSigner(IOptions<StorageOptions> options) : ITokenSigner
{
    private readonly byte[] _secret = Encoding.UTF8.GetBytes(options.Value.SigningSecret);

    public string GenerateDownloadToken(string bucket, string filename, TimeSpan ttl)
    {
        var expiry = DateTimeOffset.UtcNow.Add(ttl).ToUnixTimeSeconds();
        var payload = $"{bucket}|{filename}|{expiry}";
        var payloadB64 = Base64Url.EncodeToString(Encoding.UTF8.GetBytes(payload));
        var signature = Sign(payloadB64);
        return $"{payloadB64}.{signature}";
    }

    public (string Bucket, string Filename)? ValidateDownloadToken(string token)
    {
        var parts = token.Split('.', 2);
        if (parts.Length != 2)
        {
            return null;
        }
        var (payloadB64, signature) = (parts[0], parts[1]);

        var expectedSignature = Sign(payloadB64);
        if (!FixedTimeEquals(signature, expectedSignature))
        {
            return null;
        }

        byte[] payloadBytes;
        try
        {
            payloadBytes = Base64Url.DecodeFromChars(payloadB64);
        }
        catch (FormatException)
        {
            return null;
        }

        var fields = Encoding.UTF8.GetString(payloadBytes).Split('|', 3);
        if (fields.Length != 3)
        {
            return null;
        }

        if (!long.TryParse(fields[2], out var expiry))
        {
            return null;
        }
        if (DateTimeOffset.UtcNow.ToUnixTimeSeconds() > expiry)
        {
            return null;
        }

        return (fields[0], fields[1]);
    }

    private string Sign(string payloadB64)
    {
        var hash = HMACSHA256.HashData(_secret, Encoding.UTF8.GetBytes(payloadB64));
        return Base64Url.EncodeToString(hash);
    }

    /// <summary>
    /// Compara dos strings en tiempo constante para evitar timing attacks al
    /// validar la firma. Ambos se codifican a UTF-8 primero: CryptographicOperations.FixedTimeEquals
    /// exige spans de igual longitud, y strings de largo distinto ya
    /// implican que no matchean.
    /// </summary>
    private static bool FixedTimeEquals(string a, string b)
    {
        var aBytes = Encoding.UTF8.GetBytes(a);
        var bBytes = Encoding.UTF8.GetBytes(b);
        if (aBytes.Length != bBytes.Length)
        {
            return false;
        }
        return CryptographicOperations.FixedTimeEquals(aBytes, bBytes);
    }
}
