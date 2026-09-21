using GabichoStorage.Api.Options;
using Microsoft.Extensions.Options;

namespace GabichoStorage.Api.Services;

public class LocalFileStorage : ILocalFileStorage
{
    public string RootPath { get; }

    public LocalFileStorage(IOptions<StorageOptions> options)
    {
        RootPath = options.Value.StoragePath;
        Directory.CreateDirectory(RootPath);
    }

    public void EnsureBucketDirectory(string bucket) => Directory.CreateDirectory(BucketDir(bucket));

    public void RemoveBucketDirectory(string bucket)
    {
        var dir = BucketDir(bucket);
        if (Directory.Exists(dir))
        {
            Directory.Delete(dir, recursive: true);
        }
    }

    public async Task<(string StoredName, string RelativePath)> SaveAsync(string bucket, Stream content, string extension, CancellationToken ct = default)
    {
        EnsureBucketDirectory(bucket);

        var storedName = $"{Guid.NewGuid()}{extension}";
        var relativePath = Path.Combine(bucket, storedName);
        var fullPath = Path.Combine(RootPath, relativePath);

        // FileMode.CreateNew: falla si ya existiera (no debería, el nombre
        // es un GUID nuevo), en vez de pisar contenido silenciosamente.
        await using var dst = new FileStream(fullPath, FileMode.CreateNew, FileAccess.Write);
        await content.CopyToAsync(dst, ct);

        return (storedName, relativePath);
    }

    public string FullPath(string relativePath) => Path.Combine(RootPath, relativePath);

    public void DeleteFile(string relativePath)
    {
        var path = FullPath(relativePath);
        if (File.Exists(path))
        {
            File.Delete(path);
        }
    }

    private string BucketDir(string bucket) => Path.Combine(RootPath, bucket);
}
