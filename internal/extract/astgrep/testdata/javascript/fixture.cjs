const express = require("express");

module.exports = function cjsGreet(name) {
    return `Hello, ${name}`;
};

const app = express();
app.get("/cjs", (req, res) => {
    res.send("hello");
});
