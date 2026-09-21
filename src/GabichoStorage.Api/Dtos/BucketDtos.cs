namespace GabichoStorage.Api.Dtos;

public record CreateBucketRequest(string Name, bool IsPublic);

public record BucketResponse(Guid Id, string Name, string Owner, bool IsPublic, DateTimeOffset CreatedAt);
