import express from "express";

export function jsGreet(name) {
    return `Hello, ${name}`;
}

export class JsGreeter {
    greet() {
        return jsGreet("world");
    }
}

const app = express();
app.get("/js", (req, res) => {
    res.send(jsGreet("world"));
});
