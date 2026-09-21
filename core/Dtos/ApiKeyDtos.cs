namespace GabichoStorage.Api.Dtos;

public record CreateApiKeyRequest(string UserId, string? Permissions);

public record CreateApiKeyResponse(Guid Id, string UserId, string Key, string Permissions, DateTimeOffset CreatedAt);

public record ApiKeyResponse(Guid Id, string UserId, string Permissions, DateTimeOffset CreatedAt, DateTimeOffset? LastUsed);

public record ErrorResponse(string Message);
