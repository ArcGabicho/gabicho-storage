namespace GabichoStorage.Api.Middleware;

/// <summary>
/// Headers de seguridad globales (equivalente al middleware "helmet" de
/// Express/otros frameworks). CrossOriginResourcePolicy queda explícitamente
/// "cross-origin" (no "same-origin", que sería el default recomendado en
/// general) porque este servicio está pensado para que un frontend en otro
/// origen (ej. Next.js en Vercel) embeba imágenes/videos vía
/// &lt;img&gt;/&lt;video&gt;.
/// </summary>
public class SecurityHeadersMiddleware(RequestDelegate next)
{
    public async Task InvokeAsync(HttpContext context)
    {
        context.Response.OnStarting(() =>
        {
            var headers = context.Response.Headers;
            headers["X-Content-Type-Options"] = "nosniff";
            headers["X-Frame-Options"] = "DENY";
            headers["Referrer-Policy"] = "no-referrer";
            headers["Cross-Origin-Resource-Policy"] = "cross-origin";
            headers["Cross-Origin-Opener-Policy"] = "same-origin";
            headers["X-Permitted-Cross-Domain-Policies"] = "none";
            // "sandbox" desactiva scripts, forms y navegación dentro del
            // propio documento servido; "default-src 'none'" bloquea
            // cualquier carga de subrecursos. Es la mitigación estándar
            // (la misma que usa GitHub para raw.githubusercontent.com)
            // contra XSS almacenado vía SVG/HTML subido con un Content-Type
            // de imagen: aunque el archivo contenga <script>, el browser no
            // lo va a poder ejecutar al servirlo, se abra directamente o se
            // embeba en un <img>/<video>.
            headers["Content-Security-Policy"] = "default-src 'none'; sandbox";
            return Task.CompletedTask;
        });

        await next(context);
    }
}
