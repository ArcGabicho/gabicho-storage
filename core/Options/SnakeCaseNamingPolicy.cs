using System.Text;
using System.Text.Json;

namespace GabichoStorage.Api.Options;

/// <summary>
/// Convierte PascalCase/camelCase a snake_case para el JSON de la API,
/// manteniendo el mismo contrato (user_id, is_public, created_at, ...) que
/// tenía la versión original en Go, así un frontend existente no necesita
/// cambios al migrar el backend.
/// </summary>
public class SnakeCaseNamingPolicy : JsonNamingPolicy
{
    public override string ConvertName(string name)
    {
        var sb = new StringBuilder(name.Length + 8);
        for (var i = 0; i < name.Length; i++)
        {
            var c = name[i];
            if (char.IsUpper(c))
            {
                if (i > 0)
                {
                    sb.Append('_');
                }
                sb.Append(char.ToLowerInvariant(c));
            }
            else
            {
                sb.Append(c);
            }
        }
        return sb.ToString();
    }
}
