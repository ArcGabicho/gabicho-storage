using System.Net.Http.Json;
using System.Text;
using System.Text.Json;

namespace GabichoStorage.Tests.Integration;

public static class TestHelpers
{
    public static string UniqueName(string prefix) => $"{prefix}-{Guid.NewGuid():N}"[..Math.Min(prefix.Length + 9, 40)];

    public static async Task<string> CreateApiKeyAsync(HttpClient client, string masterKey, string userId, string permissions = "read,write")
    {
        var request = new HttpRequestMessage(HttpMethod.Post, "/api/auth/keys")
        {
            Content = JsonContent.Create(new { user_id = userId, permissions }),
        };
        request.Headers.Add("X-Master-Key", masterKey);

        var response = await client.SendAsync(request);
        response.EnsureSuccessStatusCode();

        var body = await response.Content.ReadFromJsonAsync<JsonElement>();
        return body.GetProperty("key").GetString()!;
    }

    public static async Task CreateBucketAsync(HttpClient client, string apiKey, string name, bool isPublic)
    {
        var request = new HttpRequestMessage(HttpMethod.Post, "/api/buckets")
        {
            Content = JsonContent.Create(new { name, is_public = isPublic }),
        };
        request.Headers.Add("X-API-Key", apiKey);

        var response = await client.SendAsync(request);
        response.EnsureSuccessStatusCode();
    }

    public static MultipartFormDataContent BuildUploadForm(byte[] content, string filename, string? mimeType, Dictionary<string, string>? extraFields = null)
    {
        var form = new MultipartFormDataContent();
        var fileContent = new ByteArrayContent(content);
        if (mimeType is not null)
        {
            fileContent.Headers.ContentType = new System.Net.Http.Headers.MediaTypeHeaderValue(mimeType);
        }
        form.Add(fileContent, "file", filename);

        if (extraFields is not null)
        {
            foreach (var (key, value) in extraFields)
            {
                form.Add(new StringContent(value, Encoding.UTF8), key);
            }
        }

        return form;
    }

    public static HttpRequestMessage WithApiKey(this HttpRequestMessage request, string apiKey)
    {
        request.Headers.Add("X-API-Key", apiKey);
        return request;
    }

    public static HttpRequestMessage WithMasterKey(this HttpRequestMessage request, string masterKey)
    {
        request.Headers.Add("X-Master-Key", masterKey);
        return request;
    }
}
