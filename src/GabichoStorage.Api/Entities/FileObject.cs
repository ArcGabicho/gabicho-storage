namespace GabichoStorage.Api.Entities;

/// <summary>
/// Metadata de un archivo almacenado dentro de un bucket. El contenido
/// binario vive en el filesystem local (ver Services/ILocalFileStorage),
/// en la ruta indicada por <see cref="Path"/>.
/// </summary>
public class FileObject
{
    public Guid Id { get; set; }

    public Guid BucketId { get; set; }

    public Bucket? Bucket { get; set; }

    /// <summary>Nombre físico en disco (UUID + extensión derivada del MimeType).</summary>
    public required string Filename { get; set; }

    /// <summary>Nombre original declarado por el cliente, sólo para mostrar.</summary>
    public required string OriginalName { get; set; }

    public required string MimeType { get; set; }

    public long Size { get; set; }

    /// <summary>Ruta relativa al root de storage, ej. "mi-bucket/uuid.png".</summary>
    public required string Path { get; set; }

    public bool IsPublic { get; set; }

    public DateTimeOffset CreatedAt { get; set; }

    /// <summary>Metadata arbitraria asociada al archivo, persistida como JSON.</summary>
    public Dictionary<string, object?> Metadata { get; set; } = [];
}
