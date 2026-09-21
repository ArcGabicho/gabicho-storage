using Microsoft.EntityFrameworkCore;
using Microsoft.EntityFrameworkCore.Design;

namespace GabichoStorage.Api.Data;

/// <summary>
/// Factory usada por las herramientas de diseño de EF Core (`dotnet ef
/// migrations add/update`) para poder generar/aplicar migraciones sin tener
/// que levantar el `Program.cs` completo (que exige los tres secretos de
/// runtime vía StorageOptions.Load() y fallaría si no están seteados). El
/// connection string acá sólo necesita ser sintácticamente válido: estos
/// comandos analizan el modelo, no necesitan conectarse de verdad.
/// </summary>
public class AppDbContextFactory : IDesignTimeDbContextFactory<AppDbContext>
{
    public AppDbContext CreateDbContext(string[] args)
    {
        var optionsBuilder = new DbContextOptionsBuilder<AppDbContext>();
        optionsBuilder.UseSqlServer("Server=localhost,1433;Database=storage;User Id=sa;Password=Placeholder123!;TrustServerCertificate=True");
        return new AppDbContext(optionsBuilder.Options);
    }
}
