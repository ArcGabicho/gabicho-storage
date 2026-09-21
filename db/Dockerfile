# Wrapper sobre la imagen oficial de SQL Server que agrega soporte para
# MSSQL_SA_PASSWORD_FILE (ver entrypoint-secrets.sh). Todo lo demás -- la
# instalación de SQL Server en sí -- viene sin tocar de la imagen base.
FROM mcr.microsoft.com/mssql/server:2022-latest

USER root
COPY entrypoint-secrets.sh /usr/local/bin/entrypoint-secrets.sh
RUN chmod +x /usr/local/bin/entrypoint-secrets.sh
USER mssql

ENTRYPOINT ["/usr/local/bin/entrypoint-secrets.sh"]
CMD ["/opt/mssql/bin/sqlservr"]
