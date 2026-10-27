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
    }
}
