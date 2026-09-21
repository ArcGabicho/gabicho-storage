using GabichoStorage.Api.Services;

namespace GabichoStorage.Tests.Unit;

public class FilenameSanitizerTests
{
    [Theory]
    [InlineData("my-bucket", true)]
    [InlineData("bucket123", true)]
    [InlineData("abc", true)]
    [InlineData("a", false)]
    [InlineData("ab", false)]
    [InlineData("My-Bucket", false)]
    [InlineData("my_bucket", false)]
    [InlineData("-my-bucket", false)]
    [InlineData("my-bucket-", false)]
    [InlineData("", false)]
    [InlineData("has a space", false)]
    public void IsValidBucketName_EnforcesDnsStylePattern(string name, bool expected)
    {
        Assert.Equal(expected, FilenameSanitizer.IsValidBucketName(name));
    }

    [Fact]
    public void IsValidBucketName_BoundaryLengths()
    {
        Assert.True(FilenameSanitizer.IsValidBucketName(new string('a', 63)));
        Assert.False(FilenameSanitizer.IsValidBucketName(new string('a', 64)));
    }

    [Theory]
    [InlineData("photo.jpg", "photo.jpg")]
    [InlineData("my file (1).png", "my_file__1_.png")]
    [InlineData("../../etc/passwd", "passwd")]
    [InlineData("/etc/passwd", "passwd")]
    [InlineData("..hidden", "hidden")]
    [InlineData("", "file")]
    public void Sanitize_ProducesSafeFilename(string input, string expected)
    {
        Assert.Equal(expected, FilenameSanitizer.Sanitize(input));
    }

    [Theory]
    [InlineData("")]
    [InlineData(".")]
    [InlineData("..")]
    [InlineData("...")]
    public void Sanitize_NeverReturnsEmpty(string input)
    {
        Assert.False(string.IsNullOrEmpty(FilenameSanitizer.Sanitize(input)));
    }

    [Theory]
    [InlineData(1024, false)]
    [InlineData(500L * 1024 * 1024, false)]
    [InlineData(500L * 1024 * 1024 + 1, true)]
    [InlineData(0, true)]
    [InlineData(-1, true)]
    public void ValidateFileSize_EnforcesMaxAndRejectsEmpty(long size, bool expectError)
    {
        var error = FilenameSanitizer.ValidateFileSize(size, 500L * 1024 * 1024);
        Assert.Equal(expectError, error is not null);
    }
}
