using GabichoStorage.Api.Entities;

namespace GabichoStorage.Tests.Unit;

public class ApiKeyPermissionTests
{
    private static ApiKey MakeKey(string permissions) => new()
    {
        Id = Guid.NewGuid(),
        UserId = "user",
        KeyHash = "hash",
        Permissions = permissions,
    };

    [Theory]
    [InlineData("read", Permission.Read, true)]
    [InlineData("write", Permission.Write, true)]
    [InlineData("read", Permission.Write, false)]
    [InlineData("read,write", Permission.Write, true)]
    [InlineData("admin", Permission.Read, true)]
    [InlineData("admin", Permission.Write, true)]
    [InlineData("admin", Permission.Admin, true)]
    [InlineData("read, write", Permission.Write, true)]
    [InlineData("", Permission.Read, false)]
    [InlineData("foo", Permission.Read, false)]
    public void HasPermission_MatchesExpected(string permissions, string check, bool expected)
    {
        var key = MakeKey(permissions);
        Assert.Equal(expected, key.HasPermission(check));
    }
}
