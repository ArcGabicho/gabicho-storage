using System.Text;

namespace GabichoStorage.Api.Services;

public static class MimeValidator
{
    /// <summary>
    /// Whitelist de tipos MIME aceptados para upload (imágenes y videos).
    /// Cualquier otro tipo es rechazado.
    /// </summary>
    public static readonly HashSet<string> AllowedMimeTypes = new(StringComparer.OrdinalIgnoreCase)
    {
        // Imágenes
        "image/jpeg",
        "image/png",
        "image/gif",
        "image/webp",
        "image/svg+xml",
        "image/avif",
        "image/heic",
        // Videos
        "video/mp4",
        "video/mpeg",
        "video/webm",
        "video/quicktime",
        "video/x-msvideo",
        "video/x-matroska",
    };

    /// <summary>
    /// Mapea cada tipo MIME permitido a UNA extensión fija. El nombre físico
    /// en disco siempre se genera a partir de esta tabla, nunca del nombre
    /// de archivo que mandó el cliente: el Content-Type del form y el
    /// nombre de archivo son dos campos independientes que un cliente puede
    /// declarar sin que se correspondan entre sí (ej: Content-Type
    /// "image/png" con Filename "shell.php").
    /// </summary>
    private static readonly Dictionary<string, string> ExtensionByMimeType = new(StringComparer.OrdinalIgnoreCase)
    {
        ["image/jpeg"] = ".jpg",
        ["image/png"] = ".png",
        ["image/gif"] = ".gif",
        ["image/webp"] = ".webp",
        ["image/svg+xml"] = ".svg",
        ["image/avif"] = ".avif",
        ["image/heic"] = ".heic",
        ["video/mp4"] = ".mp4",
        ["video/mpeg"] = ".mpeg",
        ["video/webm"] = ".webm",
        ["video/quicktime"] = ".mov",
        ["video/x-msvideo"] = ".avi",
        ["video/x-matroska"] = ".mkv",
    };

    public static bool IsAllowedMimeType(string? mimeType) =>
        mimeType is not null && AllowedMimeTypes.Contains(mimeType.Trim());

    /// <summary>
    /// Extensión física a usar para un tipo MIME ya validado con
    /// <see cref="IsAllowedMimeType"/>.
    /// </summary>
    public static string ExtensionForMimeType(string mimeType) =>
        ExtensionByMimeType.TryGetValue(mimeType.Trim(), out var ext) ? ext : "";

    /// <summary>
    /// Detección barata de HTML disfrazado de otro tipo (ej. un .html subido
    /// con Content-Type: image/png para intentar servirlo después como si
    /// fuera una imagen), a partir de los primeros bytes reales del
    /// archivo. No es una validación completa de magic bytes por formato
    /// -- esta whitelist incluye demasiados formatos de imagen/video para
    /// sniffear todos -- pero cierra el vector de ataque más importante:
    /// contenido HTML/script ejecutable por el browser escondido detrás de
    /// un Content-Type de imagen o video.
    /// </summary>
    public static bool LooksLikeHtml(ReadOnlySpan<byte> sniffedBytes)
    {
        // Mismas reglas de sniffing (simplificadas) que usan los browsers:
        // ignorar whitespace/BOM inicial y buscar los prefijos de tag HTML
        // más comunes, case-insensitive.
        var text = Encoding.ASCII.GetString(sniffedBytes).TrimStart('﻿', ' ', '\t', '\n', '\r');

        string[] htmlPrefixes =
        [
            "<!DOCTYPE HTML", "<HTML", "<HEAD", "<SCRIPT", "<IFRAME", "<BODY", "<TITLE",
        ];

        foreach (var prefix in htmlPrefixes)
        {
            if (text.Length >= prefix.Length &&
                text[..prefix.Length].Equals(prefix, StringComparison.OrdinalIgnoreCase))
            {
                return true;
            }
        }
        return false;
    }
}
