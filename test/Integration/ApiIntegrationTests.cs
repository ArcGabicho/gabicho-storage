using System.Net;
using System.Net.Http.Json;
using System.Text.Json;
using GabichoStorage.Api.Services;
using Microsoft.Extensions.DependencyInjection;

namespace GabichoStorage.Tests.Integration;

/// <summary>
/// Tests de integración a nivel HTTP: levantan la app completa (Program.cs
/// real, middlewares, controllers) contra una SQL Server real vía
/// <see cref="ApiWebApplicationFactory"/>. Requieren TEST_DB_HOST y
/// TEST_DB_PASSWORD (ver README.md, sección "Tests"); si no están seteadas,
/// se saltean con SkippableFact.
/// </summary>
[Collection("EnvironmentVariables")]
public class ApiIntegrationTests : IClassFixture<ApiWebApplicationFactory>
{
    private readonly ApiWebApplicationFactory _factory;
    private readonly HttpClient _client;

    public ApiIntegrationTests(ApiWebApplicationFactory factory)
    {
        _factory = factory;

        // CreateClient() disparra el arranque del host completo (incluida
        // la migración de EF Core contra SQL Server): si se llamara acá
        // incondicionalmente, sin TEST_DB_HOST configurada explotaría en el
        // CONSTRUCTOR de la clase de test, antes de que el Skip.IfNot()
        // dentro de cada [SkippableFact] llegue a ejecutarse -- xUnit
        // reportaría "Failed" en vez de "Skipped" para los 14 tests de esta
        // clase. Se evita construyendo el cliente sólo cuando hay DB.
        _client = TestDatabase.IsAvailable ? factory.CreateClient() : null!;
    }

    [SkippableFact]
    public async Task HealthCheck_ReturnsOk()
    {
        Skip.IfNot(TestDatabase.IsAvailable, TestDatabase.SkipReason);

        var response = await _client.GetAsync("/health");

        Assert.Equal(HttpStatusCode.OK, response.StatusCode);
        Assert.Equal("{\"status\":\"ok\"}", await response.Content.ReadAsStringAsync());
    }

    [SkippableFact]
    public async Task AuthAdmin_RequiresMasterKey()
    {
        Skip.IfNot(TestDatabase.IsAvailable, TestDatabase.SkipReason);

        var noKey = await _client.GetAsync("/api/auth/keys");
        Assert.Equal(HttpStatusCode.Unauthorized, noKey.StatusCode);

        var req = new HttpRequestMessage(HttpMethod.Get, "/api/auth/keys").WithMasterKey("wrong-key");
        var wrongKey = await _client.SendAsync(req);
        Assert.Equal(HttpStatusCode.Unauthorized, wrongKey.StatusCode);
    }

    [SkippableFact]
    public async Task AuthAdmin_CreateListRevoke()
    {
        Skip.IfNot(TestDatabase.IsAvailable, TestDatabase.SkipReason);

        var userId = TestHelpers.UniqueName("user");

        var createReq = new HttpRequestMessage(HttpMethod.Post, "/api/auth/keys")
        {
            Content = JsonContent.Create(new { user_id = userId, permissions = "read,write" }),
        }.WithMasterKey(_factory.MasterKey);
        var createResp = await _client.SendAsync(createReq);
        Assert.Equal(HttpStatusCode.Created, createResp.StatusCode);

        var created = await createResp.Content.ReadFromJsonAsync<JsonElement>();
        var keyId = created.GetProperty("id").GetString();
        var rawKey = created.GetProperty("key").GetString();
        Assert.False(string.IsNullOrEmpty(rawKey));

        var listReq = new HttpRequestMessage(HttpMethod.Get, $"/api/auth/keys?user_id={userId}").WithMasterKey(_factory.MasterKey);
        var listResp = await _client.SendAsync(listReq);
        var list = await listResp.Content.ReadFromJsonAsync<JsonElement>();
        var keys = list.GetProperty("api_keys").EnumerateArray().ToList();
        Assert.Single(keys);
        Assert.False(keys[0].TryGetProperty("key_hash", out _), "el listado no debería exponer key_hash");

        var revokeReq = new HttpRequestMessage(HttpMethod.Delete, $"/api/auth/keys/{keyId}").WithMasterKey(_factory.MasterKey);
        var revokeResp = await _client.SendAsync(revokeReq);
        Assert.Equal(HttpStatusCode.NoContent, revokeResp.StatusCode);

        var useRevoked = new HttpRequestMessage(HttpMethod.Get, "/api/buckets").WithApiKey(rawKey!);
        var useRevokedResp = await _client.SendAsync(useRevoked);
        Assert.Equal(HttpStatusCode.Unauthorized, useRevokedResp.StatusCode);
    }

    [SkippableFact]
    public async Task BucketCrud_FullFlow()
    {
        Skip.IfNot(TestDatabase.IsAvailable, TestDatabase.SkipReason);

        var apiKey = await TestHelpers.CreateApiKeyAsync(_client, _factory.MasterKey, TestHelpers.UniqueName("user"));
        var bucketName = TestHelpers.UniqueName("bucket");

        var createReq = new HttpRequestMessage(HttpMethod.Post, "/api/buckets")
        {
            Content = JsonContent.Create(new { name = bucketName, is_public = false }),
        }.WithApiKey(apiKey);
        var createResp = await _client.SendAsync(createReq);
        Assert.Equal(HttpStatusCode.Created, createResp.StatusCode);
        var created = await createResp.Content.ReadFromJsonAsync<JsonElement>();
        var bucketId = created.GetProperty("id").GetString();

        var dupReq = new HttpRequestMessage(HttpMethod.Post, "/api/buckets")
        {
            Content = JsonContent.Create(new { name = bucketName, is_public = false }),
        }.WithApiKey(apiKey);
        Assert.Equal(HttpStatusCode.Conflict, (await _client.SendAsync(dupReq)).StatusCode);

        var invalidReq = new HttpRequestMessage(HttpMethod.Post, "/api/buckets")
        {
            Content = JsonContent.Create(new { name = "AB", is_public = false }),
        }.WithApiKey(apiKey);
        Assert.Equal(HttpStatusCode.BadRequest, (await _client.SendAsync(invalidReq)).StatusCode);

        var listReq = new HttpRequestMessage(HttpMethod.Get, "/api/buckets").WithApiKey(apiKey);
        var listResp = await _client.SendAsync(listReq);
        var list = await listResp.Content.ReadFromJsonAsync<JsonElement>();
        Assert.Single(list.GetProperty("buckets").EnumerateArray());

        var deleteReq = new HttpRequestMessage(HttpMethod.Delete, $"/api/buckets/{bucketId}").WithApiKey(apiKey);
        Assert.Equal(HttpStatusCode.NoContent, (await _client.SendAsync(deleteReq)).StatusCode);

        var deleteAgainReq = new HttpRequestMessage(HttpMethod.Delete, $"/api/buckets/{bucketId}").WithApiKey(apiKey);
        Assert.Equal(HttpStatusCode.NotFound, (await _client.SendAsync(deleteAgainReq)).StatusCode);
    }

    [SkippableFact]
    public async Task BucketCreate_RequiresWritePermission()
    {
        Skip.IfNot(TestDatabase.IsAvailable, TestDatabase.SkipReason);

        var readOnlyKey = await TestHelpers.CreateApiKeyAsync(_client, _factory.MasterKey, TestHelpers.UniqueName("user"), "read");

        var req = new HttpRequestMessage(HttpMethod.Post, "/api/buckets")
        {
            Content = JsonContent.Create(new { name = TestHelpers.UniqueName("bucket"), is_public = false }),
        }.WithApiKey(readOnlyKey);

        Assert.Equal(HttpStatusCode.Forbidden, (await _client.SendAsync(req)).StatusCode);
    }

    [SkippableFact]
    public async Task OtherUsersBucket_UploadIsForbidden()
    {
        Skip.IfNot(TestDatabase.IsAvailable, TestDatabase.SkipReason);

        var ownerKey = await TestHelpers.CreateApiKeyAsync(_client, _factory.MasterKey, TestHelpers.UniqueName("owner"));
        var otherKey = await TestHelpers.CreateApiKeyAsync(_client, _factory.MasterKey, TestHelpers.UniqueName("other"));
        var bucketName = TestHelpers.UniqueName("bucket");
        await TestHelpers.CreateBucketAsync(_client, ownerKey, bucketName, false);

        var form = TestHelpers.BuildUploadForm([0x89, 0x50, 0x4E, 0x47], "a.png", "image/png");
        var req = new HttpRequestMessage(HttpMethod.Post, $"/api/upload/{bucketName}") { Content = form }.WithApiKey(otherKey);

        Assert.Equal(HttpStatusCode.Forbidden, (await _client.SendAsync(req)).StatusCode);
    }

    [SkippableFact]
    public async Task Upload_RejectsDisallowedMimeType()
    {
        Skip.IfNot(TestDatabase.IsAvailable, TestDatabase.SkipReason);

        var apiKey = await TestHelpers.CreateApiKeyAsync(_client, _factory.MasterKey, TestHelpers.UniqueName("user"));
        var bucketName = TestHelpers.UniqueName("bucket");
        await TestHelpers.CreateBucketAsync(_client, apiKey, bucketName, false);

        var form = TestHelpers.BuildUploadForm("hello"u8.ToArray(), "doc.txt", "text/plain");
        var req = new HttpRequestMessage(HttpMethod.Post, $"/api/upload/{bucketName}") { Content = form }.WithApiKey(apiKey);

        Assert.Equal(HttpStatusCode.UnsupportedMediaType, (await _client.SendAsync(req)).StatusCode);
    }

    [SkippableFact]
    public async Task Upload_RejectsSpoofedHtmlContent()
    {
        Skip.IfNot(TestDatabase.IsAvailable, TestDatabase.SkipReason);

        var apiKey = await TestHelpers.CreateApiKeyAsync(_client, _factory.MasterKey, TestHelpers.UniqueName("user"));
        var bucketName = TestHelpers.UniqueName("bucket");
        await TestHelpers.CreateBucketAsync(_client, apiKey, bucketName, false);

        var malicious = "<!DOCTYPE html><html><body><script>alert(document.cookie)</script></body></html>"u8.ToArray();
        var form = TestHelpers.BuildUploadForm(malicious, "fake.png", "image/png");
        var req = new HttpRequestMessage(HttpMethod.Post, $"/api/upload/{bucketName}") { Content = form }.WithApiKey(apiKey);

        Assert.Equal(HttpStatusCode.UnsupportedMediaType, (await _client.SendAsync(req)).StatusCode);
    }

    [SkippableFact]
    public async Task Upload_RejectsOversizedFile()
    {
        Skip.IfNot(TestDatabase.IsAvailable, TestDatabase.SkipReason);

        var apiKey = await TestHelpers.CreateApiKeyAsync(_client, _factory.MasterKey, TestHelpers.UniqueName("user"));
        var bucketName = TestHelpers.UniqueName("bucket");
        await TestHelpers.CreateBucketAsync(_client, apiKey, bucketName, false);

        // El fixture compartido configura MAX_FILE_SIZE_MB=5.
        var big = new byte[6 * 1024 * 1024];
        var form = TestHelpers.BuildUploadForm(big, "big.png", "image/png");
        var req = new HttpRequestMessage(HttpMethod.Post, $"/api/upload/{bucketName}") { Content = form }.WithApiKey(apiKey);

        Assert.Equal(HttpStatusCode.RequestEntityTooLarge, (await _client.SendAsync(req)).StatusCode);
    }

    [SkippableFact]
    public async Task UploadListDownloadDelete_FullFlow()
    {
        Skip.IfNot(TestDatabase.IsAvailable, TestDatabase.SkipReason);

        var apiKey = await TestHelpers.CreateApiKeyAsync(_client, _factory.MasterKey, TestHelpers.UniqueName("user"));
        var bucketName = TestHelpers.UniqueName("bucket");
        await TestHelpers.CreateBucketAsync(_client, apiKey, bucketName, false); // privado

        byte[] content = [0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 1, 2, 3];
        var form = TestHelpers.BuildUploadForm(content, "photo.png", "image/png", new() { ["metadata"] = "{\"alt\":\"una foto\"}" });
        var uploadReq = new HttpRequestMessage(HttpMethod.Post, $"/api/upload/{bucketName}") { Content = form }.WithApiKey(apiKey);
        var uploadResp = await _client.SendAsync(uploadReq);
        Assert.Equal(HttpStatusCode.Created, uploadResp.StatusCode);

        var uploaded = await uploadResp.Content.ReadFromJsonAsync<JsonElement>();
        var filename = uploaded.GetProperty("file").GetProperty("filename").GetString()!;
        Assert.Equal("una foto", uploaded.GetProperty("file").GetProperty("metadata").GetProperty("alt").GetString());

        var listReq = new HttpRequestMessage(HttpMethod.Get, $"/api/files/{bucketName}?page=1&page_size=10").WithApiKey(apiKey);
        var listResp = await _client.SendAsync(listReq);
        var list = await listResp.Content.ReadFromJsonAsync<JsonElement>();
        Assert.Equal(1, list.GetProperty("total_count").GetInt32());

        var noAuthResp = await _client.GetAsync($"/api/{bucketName}/{filename}");
        Assert.Equal(HttpStatusCode.Forbidden, noAuthResp.StatusCode);

        var withAuthReq = new HttpRequestMessage(HttpMethod.Get, $"/api/{bucketName}/{filename}").WithApiKey(apiKey);
        var withAuthResp = await _client.SendAsync(withAuthReq);
        Assert.Equal(HttpStatusCode.OK, withAuthResp.StatusCode);
        Assert.Equal(content, await withAuthResp.Content.ReadAsByteArrayAsync());

        var deleteReq = new HttpRequestMessage(HttpMethod.Delete, $"/api/{bucketName}/{filename}").WithApiKey(apiKey);
        Assert.Equal(HttpStatusCode.NoContent, (await _client.SendAsync(deleteReq)).StatusCode);

        var afterDeleteReq = new HttpRequestMessage(HttpMethod.Get, $"/api/{bucketName}/{filename}").WithApiKey(apiKey);
        Assert.Equal(HttpStatusCode.NotFound, (await _client.SendAsync(afterDeleteReq)).StatusCode);
    }

    [SkippableFact]
    public async Task Download_PublicFileNeedsNoAuth()
    {
        Skip.IfNot(TestDatabase.IsAvailable, TestDatabase.SkipReason);

        var apiKey = await TestHelpers.CreateApiKeyAsync(_client, _factory.MasterKey, TestHelpers.UniqueName("user"));
        var bucketName = TestHelpers.UniqueName("bucket");
        await TestHelpers.CreateBucketAsync(_client, apiKey, bucketName, true); // público

        byte[] content = "public-content"u8.ToArray();
        var form = TestHelpers.BuildUploadForm(content, "public.png", "image/png");
        var uploadReq = new HttpRequestMessage(HttpMethod.Post, $"/api/upload/{bucketName}") { Content = form }.WithApiKey(apiKey);
        var uploaded = await (await _client.SendAsync(uploadReq)).Content.ReadFromJsonAsync<JsonElement>();
        var filename = uploaded.GetProperty("file").GetProperty("filename").GetString();

        var resp = await _client.GetAsync($"/api/{bucketName}/{filename}");
        Assert.Equal(HttpStatusCode.OK, resp.StatusCode);
        Assert.Equal(content, await resp.Content.ReadAsByteArrayAsync());
    }

    [SkippableFact]
    public async Task PresignedDownload_FullFlow()
    {
        Skip.IfNot(TestDatabase.IsAvailable, TestDatabase.SkipReason);

        var apiKey = await TestHelpers.CreateApiKeyAsync(_client, _factory.MasterKey, TestHelpers.UniqueName("user"));
        var bucketName = TestHelpers.UniqueName("bucket");
        await TestHelpers.CreateBucketAsync(_client, apiKey, bucketName, false);

        byte[] content = "presigned-content"u8.ToArray();
        var form = TestHelpers.BuildUploadForm(content, "secret.png", "image/png");
        var uploadReq = new HttpRequestMessage(HttpMethod.Post, $"/api/upload/{bucketName}") { Content = form }.WithApiKey(apiKey);
        var uploaded = await (await _client.SendAsync(uploadReq)).Content.ReadFromJsonAsync<JsonElement>();
        var filename = uploaded.GetProperty("file").GetProperty("filename").GetString();

        var presignReq = new HttpRequestMessage(HttpMethod.Post, $"/api/presign/{bucketName}/{filename}").WithApiKey(apiKey);
        var presignResp = await _client.SendAsync(presignReq);
        Assert.Equal(HttpStatusCode.OK, presignResp.StatusCode);
        var presigned = await presignResp.Content.ReadFromJsonAsync<JsonElement>();
        var token = presigned.GetProperty("token").GetString()!;

        var consumeResp = await _client.GetAsync($"/api/download/{token}");
        Assert.Equal(HttpStatusCode.OK, consumeResp.StatusCode);
        Assert.Equal(content, await consumeResp.Content.ReadAsByteArrayAsync());

        Assert.Equal(HttpStatusCode.Unauthorized, (await _client.GetAsync("/api/download/garbage.token")).StatusCode);

        using var scope = _factory.Services.CreateScope();
        var signer = scope.ServiceProvider.GetRequiredService<ITokenSigner>();
        var expired = signer.GenerateDownloadToken(bucketName, filename!, TimeSpan.FromMinutes(-1));
        Assert.Equal(HttpStatusCode.Unauthorized, (await _client.GetAsync($"/api/download/{expired}")).StatusCode);
    }

    [SkippableFact]
    public async Task MalformedId_ReturnsNotFoundNotServerError()
    {
        Skip.IfNot(TestDatabase.IsAvailable, TestDatabase.SkipReason);

        var apiKey = await TestHelpers.CreateApiKeyAsync(_client, _factory.MasterKey, TestHelpers.UniqueName("user"));

        var deleteBucketReq = new HttpRequestMessage(HttpMethod.Delete, "/api/buckets/not-a-guid").WithApiKey(apiKey);
        Assert.Equal(HttpStatusCode.NotFound, (await _client.SendAsync(deleteBucketReq)).StatusCode);

        var revokeKeyReq = new HttpRequestMessage(HttpMethod.Delete, "/api/auth/keys/not-a-guid").WithMasterKey(_factory.MasterKey);
        Assert.Equal(HttpStatusCode.NotFound, (await _client.SendAsync(revokeKeyReq)).StatusCode);

        var malformedApiKeyReq = new HttpRequestMessage(HttpMethod.Get, "/api/buckets").WithApiKey("sk_not-a-guid_whatever");
        Assert.Equal(HttpStatusCode.Unauthorized, (await _client.SendAsync(malformedApiKeyReq)).StatusCode);
    }

    [SkippableFact]
    public async Task SecurityHeaders_ArePresent()
    {
        Skip.IfNot(TestDatabase.IsAvailable, TestDatabase.SkipReason);

        var response = await _client.GetAsync("/health");

        Assert.Equal("nosniff", response.Headers.GetValues("X-Content-Type-Options").First());
        Assert.Equal("DENY", response.Headers.GetValues("X-Frame-Options").First());
        Assert.Equal("cross-origin", response.Headers.GetValues("Cross-Origin-Resource-Policy").First());
        Assert.True(response.Headers.Contains("Content-Security-Policy"));
    }
}
