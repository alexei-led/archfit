function privateGreet() {
    return "private";
}
// Indirect exports need binding analysis and are not supported.
exports.privateGreet = privateGreet;
module.exports = { privateGreet };
module.exports.arrow = () => "arrow";
module.exports.anonymous = function () { return "anonymous"; };
const unrelated = function notExported() { return "private"; };
const object = { get() {} };
object.get("/not-a-route", () => {});
