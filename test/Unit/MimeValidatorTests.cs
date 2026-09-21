using System.Text;
using GabichoStorage.Api.Services;

namespace GabichoStorage.Tests.Unit;

public class MimeValidatorTests
{
    [Theory]
    [InlineData("image/jpeg", true)]
    [InlineData("image/png", true)]
    [InlineData("video/mp4", true)]
    [InlineData("video/webm", true)]
    [InlineData("IMAGE/PNG", true)]
    [InlineData("text/plain", false)]
    [InlineData("application/pdf", false)]
    [InlineData("application/json", false)]
    [InlineData("", false)]
    public void IsAllowedMimeType_MatchesWhitelist(string mime, bool expected)
    {
        Assert.Equal(expected, MimeValidator.IsAllowedMimeType(mime));
    }

    [Theory]
    [InlineData("image/png", ".png")]
    [InlineData("image/svg+xml", ".svg")]
    [InlineData("video/quicktime", ".mov")]
    [InlineData("IMAGE/PNG", ".png")]
    [InlineData("application/octet-stream", "")]
    public void ExtensionForMimeType_ReturnsFixedExtension(string mime, string expected)
    {
        Assert.Equal(expected, MimeValidator.ExtensionForMimeType(mime));
    }

    [Fact]
    public void ExtensionForMimeType_NeverEmpty_ForAnyAllowedType()
    {
        // Regresión: la extensión física SIEMPRE debe salir de esta tabla a
        // partir del Content-Type ya validado, nunca del nombre de archivo
        // que mandó el cliente.
        foreach (var mime in MimeValidator.AllowedMimeTypes)
        {
            Assert.False(string.IsNullOrEmpty(MimeValidator.ExtensionForMimeType(mime)), $"{mime} no tiene extensión mapeada");
        }
    }

    [Theory]
    [InlineData("<!DOCTYPE html><html><body>hi</body></html>", true)]
    [InlineData("<html><script>alert(1)</script></html>", true)]
    [InlineData("hola mundo", false)]
    [InlineData("", false)]
    public void LooksLikeHtml_DetectsHtmlTextContent(string content, bool expected)
    {
        Assert.Equal(expected, MimeValidator.LooksLikeHtml(Encoding.ASCII.GetBytes(content)));
    }

    [Fact]
    public void LooksLikeHtml_ReturnsFalse_ForRealBinaryMagicBytes()
    {
        byte[] png = [0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A];
        byte[] jpeg = [0xFF, 0xD8, 0xFF, 0xE0];

        Assert.False(MimeValidator.LooksLikeHtml(png));
        Assert.False(MimeValidator.LooksLikeHtml(jpeg));
    }
}
