// No public declarations or confirmed framework imports.
function privateGreet() {
    return "private";
}
class PrivateGreeter {
    greet() { return privateGreet(); }
}
export const arrowGreet = () => "arrow";
export default function () { return "anonymous"; }
const object = { greet() { return "object method"; } };
object.get("/not-a-route", () => {});
