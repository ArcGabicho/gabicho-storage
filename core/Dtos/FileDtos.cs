namespace GabichoStorage.Api.Dtos;

public record FileResponse(
    Guid Id,
    Guid BucketId,
    string Filename,
    string OriginalName,
    string MimeType,
    long Size,
    bool IsPublic,
    DateTimeOffset CreatedAt,
    Dictionary<string, object?> Metadata);

public record UploadResponse(FileResponse File, string DownloadUrl);

public record FileListResponse(
    List<FileResponse> Files,
    int Page,
    int PageSize,
    long TotalCount,
    int TotalPages);

public record PresignResponse(string Token, string Url, int ExpiresIn);
