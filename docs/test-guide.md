# Guía de tests

```bash
# Sólo unitarios (sin SQL Server):
dotnet test --filter "FullyQualifiedName~Unit"

# Suite completa (requiere SQL Server real):
docker run --rm -d --name storage-test-mssql \
  -e ACCEPT_EULA=Y -e "MSSQL_SA_PASSWORD=Str0ng!Passw0rd123" \
  -p 127.0.0.1:1433:1433 mcr.microsoft.com/mssql/server:2022-latest

export TEST_DB_HOST=localhost TEST_DB_PORT=1433 TEST_DB_USER=sa TEST_DB_PASSWORD='Str0ng!Passw0rd123'
dotnet test

docker stop storage-test-mssql
```

Los tests de integración levantan la app completa (mismo `Program.cs`, vía
`WebApplicationFactory<Program>`) contra una base fresca (nombre random) en
esa instancia de SQL Server, y se saltean automáticamente
(`[SkippableFact]`) si `TEST_DB_HOST`/`TEST_DB_PASSWORD` no están seteadas.
