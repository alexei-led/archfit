var builder = WebApplication.CreateBuilder(args);
var app = builder
    .Build()
#if !WINDOWS
    .UseMaps()
#endif
    ;

app.MapGet("/before", () => "ok");

public static class Routes
{
    public static void Map(WebApplication app)
    {
        app.MapPost("/after", () => "ok");
    }
}
