import fastify from "fastify";

export async function mjsGreet(name) {
    return `Hello, ${name}`;
}

export class MjsGreeter {
    async greet() {
        return mjsGreet("world");
    }
}

const app = fastify();
app.post("/mjs", async (req, res) => {
    return mjsGreet("world");
});
