namespace GabichoStorage.Tests;

/// <summary>
/// xUnit paraleliza test CLASSES por default (no test methods dentro de la
/// misma clase). StorageOptionsTests, ApiWebApplicationFactory (usada por
/// ApiIntegrationTests) y RateLimitingTests mutan variables de entorno del
/// PROCESO (no hay forma de scopearlas por thread), así que si corrieran en
/// paralelo entre sí se pisarían los valores unas a otras -- exactamente el
/// bug que se encontró al escribir estos tests (un test terminaba viendo
/// límites o passwords de otro). Esta collection fuerza que las tres
/// corran secuenciales entre sí.
/// </summary>
[CollectionDefinition("EnvironmentVariables", DisableParallelization = true)]
public class EnvironmentVariableTestCollection;
