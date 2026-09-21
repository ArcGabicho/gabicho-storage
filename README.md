# Servicio de Storage

![Wallpaper](https://i.imgur.com/u4Hekr3.png)

Servicio de storage self-hosted en **ASP.NET Core Web API + Entity Framework
Core + SQL Server** como alternativa a Firebase Storage / Cloudflare R2:
buckets, upload/descarga de imágenes y videos, metadata en SQL Server,
autenticación con API keys y links de descarga presignados con expiración.

----

#### Clona el proyecto en local:

```bash
curl -fsSL https://raw.githubusercontent.com/ArcGabicho/gabicho-storage/main/scripts/setup.sh | bash
```

#### Deployar el proyecto en VM:

```bash
curl -fsSL https://raw.githubusercontent.com/ArcGabicho/gabicho-storage/main/scripts/deploy.sh | bash
```

----

Este proyecto está bajo la licencia MIT. Consulta el archivo [LICENSE.md](LICENSE.md) para más detalles.