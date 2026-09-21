using GabichoStorage.Api.Services;

namespace GabichoStorage.Tests.Unit;

public class ApiKeyGeneratorTests
{
    [Fact]
    public void Generate_And_Verify_RoundTrip_Succeeds()
    {
        var id = Guid.NewGuid();
        var (rawKey, hash) = ApiKeyGenerator.Generate(id);

        Assert.StartsWith($"sk_{id}_", rawKey);
        Assert.True(ApiKeyGenerator.Verify(rawKey, hash));
    }

    [Fact]
    public void Generate_KeepsRawKeyUnderBcryptLimit_ForTheHashedSecret()
    {
        // Regresión: bcrypt rechaza inputs > 72 bytes. La key completa
        // "sk_<guid>_<secret>" los supera; sólo el secreto debe hashearse,
        // nunca la key completa, o Generate() explotaría acá.
        var id = Guid.NewGuid();
        var (rawKey, _) = ApiKeyGenerator.Generate(id);

        Assert.True(rawKey.Length > 72, "la raw key debería superar el límite de bcrypt para que este test sea útil");
    }

    [Fact]
    public void ParseId_ExtractsEmbeddedId()
    {
        var id = Guid.NewGuid();
        var (rawKey, _) = ApiKeyGenerator.Generate(id);

        var parsed = ApiKeyGenerator.ParseId(rawKey);

        Assert.Equal(id, parsed);
    }

    [Theory]
    [InlineData("")]
    [InlineData("notaskkey")]
    [InlineData("wrongprefix_11111111-1111-1111-1111-111111111111_secret")]
    [InlineData("sk_not-a-guid_secret")]
    public void ParseId_ReturnsNull_ForInvalidFormats(string rawKey)
    {
        Assert.Null(ApiKeyGenerator.ParseId(rawKey));
    }

    [Fact]
    public void Verify_ReturnsFalse_ForWrongSecret()
    {
        var id = Guid.NewGuid();
        var (_, hash) = ApiKeyGenerator.Generate(id);

        var forged = $"sk_{id}_totally-wrong-secret";

        Assert.False(ApiKeyGenerator.Verify(forged, hash));
    }

    [Fact]
    public void Verify_ReturnsFalse_ForMalformedKey()
    {
        var (_, hash) = ApiKeyGenerator.Generate(Guid.NewGuid());

        Assert.False(ApiKeyGenerator.Verify("garbage-without-underscores", hash));
    }

    [Fact]
    public void Generate_ProducesUniqueSecrets()
    {
        var id = Guid.NewGuid();
        var (raw1, _) = ApiKeyGenerator.Generate(id);
        var (raw2, _) = ApiKeyGenerator.Generate(id);

        Assert.NotEqual(raw1, raw2);
    }
}
