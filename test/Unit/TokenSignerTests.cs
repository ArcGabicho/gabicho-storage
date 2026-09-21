using GabichoStorage.Api.Options;
using GabichoStorage.Api.Services;
using Microsoft.Extensions.Options;

namespace GabichoStorage.Tests.Unit;

public class TokenSignerTests
{
    private static TokenSigner MakeSigner(string secret) =>
        new(Microsoft.Extensions.Options.Options.Create(new StorageOptions { SigningSecret = secret }));

    [Fact]
    public void RoundTrip_ValidatesFreshToken()
    {
        var signer = MakeSigner("test-secret");

        var token = signer.GenerateDownloadToken("my-bucket", "photo.jpg", TimeSpan.FromMinutes(15));
        var result = signer.ValidateDownloadToken(token);

        Assert.NotNull(result);
        Assert.Equal("my-bucket", result.Value.Bucket);
        Assert.Equal("photo.jpg", result.Value.Filename);
    }

    [Fact]
    public void ExpiredToken_FailsValidation()
    {
        var signer = MakeSigner("test-secret");

        var token = signer.GenerateDownloadToken("my-bucket", "photo.jpg", TimeSpan.FromMinutes(-1));

        Assert.Null(signer.ValidateDownloadToken(token));
    }

    [Fact]
    public void TamperedSignature_FailsValidation()
    {
        var signer = MakeSigner("test-secret");

        var token = signer.GenerateDownloadToken("my-bucket", "photo.jpg", TimeSpan.FromMinutes(15));
        var parts = token.Split('.', 2);
        var tampered = $"{parts[0]}.tamperedSignatureXXXXXXXXXXXXXXXXXXXXXXXXX";

        Assert.Null(signer.ValidateDownloadToken(tampered));
    }

    [Fact]
    public void MixingPayloadAndSignatureFromDifferentTokens_FailsValidation()
    {
        var signer = MakeSigner("test-secret");

        var tokenA = signer.GenerateDownloadToken("bucket-a", "public.jpg", TimeSpan.FromMinutes(15));
        var tokenB = signer.GenerateDownloadToken("bucket-b", "secret.pdf", TimeSpan.FromMinutes(15));

        var payloadA = tokenA.Split('.', 2)[0];
        var sigB = tokenB.Split('.', 2)[1];
        var frankenstein = $"{payloadA}.{sigB}";

        Assert.Null(signer.ValidateDownloadToken(frankenstein));
    }

    [Fact]
    public void WrongSecret_FailsValidation()
    {
        var signerA = MakeSigner("secret-a");
        var signerB = MakeSigner("secret-b");

        var token = signerA.GenerateDownloadToken("my-bucket", "photo.jpg", TimeSpan.FromMinutes(15));

        Assert.Null(signerB.ValidateDownloadToken(token));
    }

    [Theory]
    [InlineData("")]
    [InlineData("no-dot-separator")]
    [InlineData("not-base64!!!.not-base64!!!")]
    public void MalformedTokens_FailValidation(string token)
    {
        var signer = MakeSigner("test-secret");

        Assert.Null(signer.ValidateDownloadToken(token));
    }
}
