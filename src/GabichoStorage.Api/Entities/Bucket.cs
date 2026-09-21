namespace GabichoStorage.Api.Entities;

/// <summary>
/// Contenedor lógico de archivos, análogo a un bucket de S3/Firebase Storage.
/// Pertenece a un "owner" (el UserId de la API key que lo creó).
/// </summary>
public class Bucket
{
    public Guid Id { get; set; }

    public required string Name { get; set; }

    public required string Owner { get; set; }

    public bool IsPublic { get; set; }

    public DateTimeOffset CreatedAt { get; set; }

    public List<FileObject> Files { get; set; } = [];
}
