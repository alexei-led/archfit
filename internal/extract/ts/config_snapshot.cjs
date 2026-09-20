const fs = require("node:fs");
const path = require("node:path");
const { pathToFileURL } = require("node:url");

function nativeLoader() {
  let directory = path.dirname(fs.realpathSync(process.argv[1]));
  for (let depth = 0; depth < 5; depth++) {
    const manifest = path.join(directory, "package.json");
    if (fs.existsSync(manifest)) {
      const pkg = JSON.parse(fs.readFileSync(manifest, "utf8"));
      if (pkg.name === "dependency-cruiser") {
        const exported = pkg.exports?.["./config-utl/extract-depcruise-config"];
        const entry = typeof exported === "string" ? exported : exported?.import;
        if (typeof entry !== "string" || !entry.startsWith("./")) {
          throw new Error("native configuration loader is unavailable");
        }
        return { url: pathToFileURL(path.join(directory, entry)).href, version: pkg.version };
      }
    }
    directory = path.dirname(directory);
  }
  throw new Error("dependency-cruiser runtime could not be identified");
}

function jsonSafe(value, seen = new Set()) {
  if (value === null || typeof value === "string" || typeof value === "boolean") return true;
  if (typeof value === "number") return Number.isFinite(value);
  if (typeof value !== "object" || seen.has(value)) return false;
  if (
    !Array.isArray(value) &&
    Object.getPrototypeOf(value) !== Object.prototype &&
    Object.getPrototypeOf(value) !== null
  )
    return false;
  if (Reflect.ownKeys(value).some((key) => typeof key === "symbol")) return false;
  seen.add(value);
  const valid = Object.keys(value).every((key) => {
    const descriptor = Object.getOwnPropertyDescriptor(value, key);
    return !descriptor.get && !descriptor.set && jsonSafe(descriptor.value, seen);
  });
  seen.delete(value);
  return valid;
}

function unsupportedOptions(config) {
  const options = config?.options || {};
  return (
    options.webpackConfig ||
    options.babelConfig ||
    options.enhancedResolveOptions?.plugins ||
    options.externalModuleResolutionStrategy === "yarn-pnp"
  );
}

function writeSnapshot(snapshot) {
  fs.writeFileSync(archfitSnapshot.output, JSON.stringify(snapshot), { mode: 0o600 });
}

module.exports = (async () => {
  let runtime;
  let load;
  try {
    runtime = nativeLoader();
    load = (await import(runtime.url)).default;
    if (typeof load !== "function") throw new Error("native configuration loader is unavailable");
  } catch {
    writeSnapshot({ fallback: true });
    throw new Error("archfit could not snapshot the native dependency-cruiser configuration");
  }
  const config = await load(archfitSnapshot.config);
  if (!jsonSafe(config) || unsupportedOptions(config)) {
    writeSnapshot({ version: runtime.version, unknown: true });
    return config;
  }
  const frozen = JSON.parse(JSON.stringify(config));
  writeSnapshot({ version: runtime.version, config: frozen });
  return frozen;
})();
