using System.Text.RegularExpressions;

namespace GabichoStorage.Api.Services;

public static partial class FilenameSanitizer
{
    [GeneratedRegex(@"^[a-z0-9]([a-z0-9-]{1,61}[a-z0-9])?$")]
    private static partial Regex BucketNameRegex();

    [GeneratedRegex(@"[^a-zA-Z0-9._-]")]
    private static partial Regex UnsafeFilenameCharsRegex();

    /// <summary>
    /// Exige nombres de bucket estilo DNS (minúsculas, dígitos y guiones,
    /// 3-63 caracteres), igual que S3, para evitar problemas al usarlos
    /// como segmentos de path o de URL.
    /// </summary>
    public static bool IsValidBucketName(string name) =>
        name.Length is >= 3 and <= 63 && BucketNameRegex().IsMatch(name);

    /// <summary>
    /// Limpia un nombre de archivo original para que sea seguro de usar
    /// como referencia (nunca se usa para el path físico real, sólo se
    /// guarda como metadata para mostrar): elimina cualquier componente de
    /// directorio, reemplaza caracteres no permitidos y evita nombres
    /// vacíos.
    /// </summary>
    public static string Sanitize(string? name)
    {
        var baseName = Path.GetFileName(name ?? "");
        var sanitized = UnsafeFilenameCharsRegex().Replace(baseName, "_");
        sanitized = sanitized.TrimStart('.');
        return string.IsNullOrEmpty(sanitized) ? "file" : sanitized;
    }

    /// <summary>Rechaza archivos que excedan el límite configurado.</summary>
    public static string? ValidateFileSize(long size, long maxSize)
    {
        if (size <= 0)
        {
            return "el archivo está vacío";
        }
        if (size > maxSize)
        {
            return $"el archivo excede el tamaño máximo permitido ({maxSize} bytes)";
        }
        return null;
    }
}
