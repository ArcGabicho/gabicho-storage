using System.Text.Json;
using GabichoStorage.Api.Entities;
using Microsoft.EntityFrameworkCore;

namespace GabichoStorage.Api.Data;

public class AppDbContext(DbContextOptions<AppDbContext> options) : DbContext(options)
{
    public DbSet<Bucket> Buckets => Set<Bucket>();
    public DbSet<FileObject> Files => Set<FileObject>();
    public DbSet<ApiKey> ApiKeys => Set<ApiKey>();

    protected override void OnModelCreating(ModelBuilder modelBuilder)
    {
        modelBuilder.Entity<Bucket>(b =>
        {
            b.ToTable("Buckets");
            b.HasKey(x => x.Id);
            b.Property(x => x.Id).ValueGeneratedOnAdd();
            b.Property(x => x.Name).HasMaxLength(63).IsRequired();
            b.HasIndex(x => x.Name).IsUnique();
            b.Property(x => x.Owner).HasMaxLength(255).IsRequired();
            b.HasIndex(x => x.Owner);
            b.Property(x => x.CreatedAt).HasDefaultValueSql("SYSDATETIMEOFFSET()");

            b.HasMany(x => x.Files)
                .WithOne(x => x.Bucket)
                .HasForeignKey(x => x.BucketId)
                .OnDelete(DeleteBehavior.Cascade);
        });

        modelBuilder.Entity<FileObject>(f =>
        {
            f.ToTable("Files");
            f.HasKey(x => x.Id);
            f.Property(x => x.Id).ValueGeneratedOnAdd();
            f.Property(x => x.Filename).HasMaxLength(255).IsRequired();
            f.Property(x => x.OriginalName).HasMaxLength(255).IsRequired();
            f.Property(x => x.MimeType).HasMaxLength(127).IsRequired();
            f.Property(x => x.Path).IsRequired();
            f.Property(x => x.CreatedAt).HasDefaultValueSql("SYSDATETIMEOFFSET()");
            f.HasIndex(x => new { x.BucketId, x.Filename }).IsUnique();
            f.HasIndex(x => new { x.BucketId, x.CreatedAt });

            // La metadata arbitraria se persiste como JSON en una sola
            // columna nvarchar(max): evita tener que modelar un esquema
            // dinámico, igual que se hizo con JSONB en la versión Postgres.
            f.Property(x => x.Metadata)
                .HasColumnName("MetadataJson")
                .HasConversion(
                    v => JsonSerializer.Serialize(v, JsonOptions),
                    v => DeserializeMetadata(v))
                .Metadata.SetValueComparer(MetadataComparer);
        });

        modelBuilder.Entity<ApiKey>(k =>
        {
            k.ToTable("ApiKeys");
            k.HasKey(x => x.Id);
            k.Property(x => x.Id).ValueGeneratedOnAdd();
            k.Property(x => x.UserId).HasMaxLength(255).IsRequired();
            k.HasIndex(x => x.UserId);
            k.Property(x => x.KeyHash).IsRequired();
            k.Property(x => x.Permissions).HasMaxLength(255).IsRequired();
            k.Property(x => x.CreatedAt).HasDefaultValueSql("SYSDATETIMEOFFSET()");
        });
    }

    private static readonly JsonSerializerOptions JsonOptions = new();

    private static Dictionary<string, object?> DeserializeMetadata(string json)
    {
        if (string.IsNullOrEmpty(json))
        {
            return [];
        }
        return JsonSerializer.Deserialize<Dictionary<string, object?>>(json, JsonOptions) ?? [];
    }

    private static readonly Microsoft.EntityFrameworkCore.ChangeTracking.ValueComparer<Dictionary<string, object?>> MetadataComparer = new(
        (a, b) => JsonSerializer.Serialize(a, JsonOptions) == JsonSerializer.Serialize(b, JsonOptions),
        v => JsonSerializer.Serialize(v, JsonOptions).GetHashCode(),
        v => DeserializeMetadata(JsonSerializer.Serialize(v, JsonOptions)));
}
