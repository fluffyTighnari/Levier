import { cp, mkdir } from "node:fs/promises";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const site = dirname(dirname(fileURLToPath(import.meta.url)));
const source = join(site, "docs");
const target = join(site, "dist", "docs");

await mkdir(join(target, "content"), { recursive: true });
await cp(join(source, "content"), join(target, "content"), { recursive: true });
await cp(join(source, "manifest.json"), join(target, "manifest.json"));
console.log("copied docs/content and manifest.json into dist/docs");
