// Copies each official plugin's own screens into the app: the ui/ folder of
// every plugin module cmd/runwell requires (its go.mod; in a go.work, a
// checkout of one beside this repository), into src/plugins/<key>/, where
// routes.js picks up its index.js. Those folders are copies (gitignored):
// change a plugin's screens in its own repository. Run by make plugin-ui,
// make ui and make ui-dev; GO names the go command (default: go).
import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";

const core = "github.com/Martin-Business-Consultants/runwellgo/v3";
const ui = path.resolve(import.meta.dirname, "..");
const binary = path.resolve(ui, "../cmd/runwell");
const dest = path.join(ui, "src/plugins");
const go = (...args) => execFileSync(process.env.GO || "go", args, { cwd: binary, encoding: "utf8", stdio: ["ignore", "pipe", "inherit"] });

// The plugins: the modules the binary requires directly, the core aside.
const required = JSON.parse(go("mod", "edit", "-json")).Require.filter((r) => !r.Indirect && r.Path !== core).map((r) => r.Path);
go("mod", "download", ...required);
// go list -m -json prints one object after another, not an array.
const modules = JSON.parse(`[${go("list", "-m", "-json", ...required).replace(/}\s*{/g, "},{")}]`);

// Whatever was copied before goes: a plugin dropped from the binary leaves nothing behind.
for (const entry of fs.readdirSync(dest, { withFileTypes: true })) {
  if (entry.isDirectory()) fs.rmSync(path.join(dest, entry.name), { recursive: true, force: true });
}
fs.rmSync(path.join(dest, "sources.css"), { force: true });

// A plain copy, so the files are writable (the module cache's aren't).
function copy(from, to) {
  fs.mkdirSync(to, { recursive: true });
  for (const entry of fs.readdirSync(from, { withFileTypes: true })) {
    const src = path.join(from, entry.name);
    const dst = path.join(to, entry.name);
    if (entry.isDirectory()) copy(src, dst);
    else if (entry.isFile()) fs.writeFileSync(dst, fs.readFileSync(src));
  }
}

const seen = new Map();
for (const m of modules) {
  const dir = m.Replace?.Dir || m.Dir;
  if (!dir) throw new Error(`${m.Path}: not downloaded (go mod download in cmd/runwell)`);
  const index = path.join(dir, "ui", "index.js");
  if (!fs.existsSync(index)) continue; // its screens are all described in its manifest
  const key = fs.readFileSync(index, "utf8").match(/export const key\s*=\s*"([a-z0-9_]+)"/)?.[1];
  if (!key) throw new Error(`${index}: no export const key = "…"`);
  if (seen.has(key)) throw new Error(`${m.Path} and ${seen.get(key)} both have the key ${key}`);
  seen.set(key, m.Path);
  copy(path.join(dir, "ui"), path.join(dest, key));
  console.log(`src/plugins/${key}  ${m.Path} ${m.Version || "(local)"}`);
}

// Tailwind skips gitignored files unless a @source names them (a folder
// above them isn't enough): app.css imports this list of the copies.
const sources = [...seen.keys()].sort().map((key) => `@source "./${key}";`);
fs.writeFileSync(path.join(dest, "sources.css"), `/* Made by scripts/plugin-ui.js (make plugin-ui): the plugins' screens, for Tailwind to scan. */\n${sources.join("\n")}\n`);
