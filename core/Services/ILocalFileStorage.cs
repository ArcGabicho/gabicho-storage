namespace GabichoStorage.Api.Services;

/// <summary>
/// Backend de almacenamiento físico de archivos. Los archivos se organizan
/// en disco como &lt;root&gt;/&lt;bucket&gt;/&lt;filename-generado&gt;, separado de sus
/// metadatos en la base de datos.
/// </summary>
public interface ILocalFileStorage
{
    void EnsureBucketDirectory(string bucket);

    void RemoveBucketDirectory(string bucket);

    /// <summary>
    /// Persiste el contenido de un stream dentro del bucket, generando un
    /// nombre físico único (UUID + extensión) para evitar colisiones y
    /// ataques de path traversal. Devuelve el nombre físico generado y la
    /// ruta relativa a guardar en la base.
    /// </summary>
    Task<(string StoredName, string RelativePath)> SaveAsync(string bucket, Stream content, string extension, CancellationToken ct = default);

    string FullPath(string relativePath);

    void DeleteFile(string relativePath);
}
