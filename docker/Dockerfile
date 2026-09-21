# --- Etapa de build ---
FROM mcr.microsoft.com/dotnet/sdk:10.0 AS builder
WORKDIR /src

COPY core/GabichoStorage.Api.csproj core/
RUN dotnet restore core/GabichoStorage.Api.csproj

COPY core/ core/
RUN dotnet publish core/GabichoStorage.Api.csproj \
    -c Release -o /out --no-restore

# --- Etapa final (runtime ASP.NET, sin SDK) ---
FROM mcr.microsoft.com/dotnet/aspnet:10.0

RUN apt-get update && apt-get install -y --no-install-recommends curl \
    && rm -rf /var/lib/apt/lists/*

# La imagen base ya trae un usuario no-root "app" (uid 1654) para esto.
WORKDIR /app
COPY --from=builder /out .

RUN mkdir -p /data/storage && chown -R app:app /data /app

USER app

EXPOSE 8080
ENV ASPNETCORE_URLS=http://+:8080
ENV ASPNETCORE_ENVIRONMENT=Production
ENV DOTNET_EnableDiagnostics=0

HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD ["curl", "-f", "http://localhost:8080/health"]

ENTRYPOINT ["dotnet", "GabichoStorage.Api.dll"]
