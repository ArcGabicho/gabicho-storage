using System.Net;

namespace GabichoStorage.Tests.Integration;

/// <summary>
/// Usa su propia instancia de <see cref="ApiWebApplicationFactory"/> con
/// límites bajos, separada del fixture compartido de
/// <see cref="ApiIntegrationTests"/> (que usa límites altos a propósito
/// para no interferir con el resto de los tests).
/// </summary>
[Collection("EnvironmentVariables")]
public class RateLimitingTests : IDisposable
{
    private readonly ApiWebApplicationFactory _factory;
    private readonly HttpClient _client;

    public RateLimitingTests()
    {
        _factory = new ApiWebApplicationFactory { ApiRateLimitMax = 3, UploadRateLimitMax = 2 };
        // Ver el comentario equivalente en ApiIntegrationTests: sin esta
        // guarda, CreateClient() explota en el constructor cuando no hay
        // TEST_DB_HOST configurada, antes de que el Skip.IfNot() de cada
        // test llegue a correr.
        _client = TestDatabase.IsAvailable ? _factory.CreateClient() : null!;
    }

    public void Dispose()
    {
        _client?.Dispose();
        _factory.Dispose();
        GC.SuppressFinalize(this);
    }

    [SkippableFact]
    public async Task GeneralApiLimit_Returns429AfterExceeding()
    {
        Skip.IfNot(TestDatabase.IsAvailable, TestDatabase.SkipReason);

        var apiKey = await TestHelpers.CreateApiKeyAsync(_client, _factory.MasterKey, TestHelpers.UniqueName("user"));

        var statuses = new List<HttpStatusCode>();
        for (var i = 0; i < 5; i++)
        {
            var req = new HttpRequestMessage(HttpMethod.Get, "/api/buckets").WithApiKey(apiKey);
            statuses.Add((await _client.SendAsync(req)).StatusCode);
        }

        Assert.Equal([HttpStatusCode.OK, HttpStatusCode.OK, HttpStatusCode.OK, HttpStatusCode.TooManyRequests, HttpStatusCode.TooManyRequests], statuses);
    }

    [SkippableFact]
    public async Task UploadLimit_IsIndependentAndStricterThanGeneralLimit()
    {
        Skip.IfNot(TestDatabase.IsAvailable, TestDatabase.SkipReason);

        var apiKey = await TestHelpers.CreateApiKeyAsync(_client, _factory.MasterKey, TestHelpers.UniqueName("user"));
        var bucketName = TestHelpers.UniqueName("bucket");
        await TestHelpers.CreateBucketAsync(_client, apiKey, bucketName, true);

        var statuses = new List<HttpStatusCode>();
        for (var i = 0; i < 4; i++)
        {
            var form = TestHelpers.BuildUploadForm([0x89, 0x50, 0x4E, 0x47], $"a{i}.png", "image/png");
            var req = new HttpRequestMessage(HttpMethod.Post, $"/api/upload/{bucketName}") { Content = form }.WithApiKey(apiKey);
            statuses.Add((await _client.SendAsync(req)).StatusCode);
        }

        // El límite de upload (2) es más estricto que el general (3) y
        // reemplaza al general específicamente en este endpoint: se corta
        // en la request 3, no en la 4.
        Assert.Equal([HttpStatusCode.Created, HttpStatusCode.Created, HttpStatusCode.TooManyRequests, HttpStatusCode.TooManyRequests], statuses);
    }

    [SkippableFact]
    public async Task Rejection_IncludesRetryAfterHeader()
    {
        Skip.IfNot(TestDatabase.IsAvailable, TestDatabase.SkipReason);

        var apiKey = await TestHelpers.CreateApiKeyAsync(_client, _factory.MasterKey, TestHelpers.UniqueName("user"));

        HttpResponseMessage? rejected = null;
        for (var i = 0; i < 5 && rejected is null; i++)
        {
            var req = new HttpRequestMessage(HttpMethod.Get, "/api/buckets").WithApiKey(apiKey);
            var resp = await _client.SendAsync(req);
            if (resp.StatusCode == HttpStatusCode.TooManyRequests)
            {
                rejected = resp;
            }
        }

        Assert.NotNull(rejected);
        Assert.True(rejected!.Headers.RetryAfter is not null);
    }
}
