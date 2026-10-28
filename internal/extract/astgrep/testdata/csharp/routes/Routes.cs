using Microsoft.AspNetCore.Mvc;

[Route("api/orders")]
public class OrdersController : ControllerBase
{
    [HttpGet]
    public IActionResult List() => Ok();

    [HttpGet("{id}")]
    public IActionResult Get(int id) => Ok();

    [HttpPost("create")]
    public IActionResult Create() => Ok();

    [HttpPut("{id}")]
    [HttpDelete("{ref}")]
    public IActionResult Change(int id) => Ok();
}

public static class Endpoints
{
    public static void Map(WebApplication app)
    {
        app.MapGet("/status", () => "ok");
        app.MapPost("/jobs", (Job j) => Results.Ok());
        app.MapPut("/jobs/{id}", (int id) => Results.Ok());
        app.MapDelete("/jobs/{id}", (int id) => Results.Ok());
        app.MapGroup("/api").MapGet("/items", () => "ok");
    }
}

public class WeatherController : ControllerBase
{
    [HttpGet(Name = "GetWeatherForecast")]
    public IActionResult Forecast() => Ok();
}

public static class MinimalApiForms
{
    public static void Register(IEndpointRouteBuilder app)
    {
        app.MapPatch("/jobs/{id}", (int id) => Results.Ok());
        app.MapMethods("/jobs", new[] { "HEAD" }, () => Results.Ok());
        app.MapIdentityApi<User>();
        app.MapHealthChecks("/health");
    }
}

public static class EndpointWrappers
{
    public static RouteHandlerBuilder MapGet(this IEndpointRouteBuilder builder, Delegate handler, string pattern = "")
    {
        return builder.MapGet(pattern, handler);
    }
}

[Route("v2/{organizationId}/groups")]
public class PrefixOnlyController : ControllerBase
{
    [HttpGet("")]
    public IActionResult List() => Ok();

    [HttpPost("")]
    public IActionResult Create() => Ok();
}
