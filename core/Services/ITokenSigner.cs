namespace GabichoStorage.Api.Services;

/// <summary>
/// Firma y valida tokens de descarga presignados de forma stateless (sin
/// persistirlos en base de datos): el propio token incluye el bucket, el
/// archivo y su expiración, autenticados con HMAC-SHA256 contra un secreto
/// del servidor.
/// </summary>
public interface ITokenSigner
{
    string GenerateDownloadToken(string bucket, string filename, TimeSpan ttl);

    /// <summary>
    /// Valida la firma y expiración de un token. Devuelve (bucket, filename)
    /// si es válido, o null si la firma es inválida, está mal formado o
    /// expiró.
    /// </summary>
    (string Bucket, string Filename)? ValidateDownloadToken(string token);
}
