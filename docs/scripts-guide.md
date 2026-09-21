# Guía de scripts

## `scripts/generate-secrets.sh`

Genera los tres archivos de secretos que consume `docker-compose.yml` vía
Docker secrets (ver [docker-guide.md](docker-guide.md)):

- `secrets/db_password.txt`: `openssl rand -base64 32` (no `-hex`: SQL
  Server exige caracteres de al menos 3 de 4 clases — mayúsculas,
  minúsculas, dígitos, símbolos).
- `secrets/signing_secret.txt`: usado para firmar los links de descarga
  presignados (HMAC-SHA256).
- `secrets/admin_master_key.txt`: `openssl rand -hex 32`, usado como
  `ADMIN_MASTER_KEY` para administrar API keys.

Los tres archivos quedan con permisos `644` y el directorio `secrets/`
está en `.gitignore`: nunca deben commitearse.

```bash
chmod +x scripts/generate-secrets.sh
./scripts/generate-secrets.sh
```
