// Prints the third-party terms for the prebuilt UI in dist/ to stdout.
// Package versions and license texts come from the installed pnpm
// node_modules tree (run `pnpm install --frozen-lockfile` at the repo root
// first).
import { existsSync, readdirSync, readFileSync } from "node:fs";
import { execSync } from "node:child_process";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const webDir = join(dirname(fileURLToPath(import.meta.url)), "..");

// Build tools whose own code is emitted into dist: tailwindcss generates the
// preflight and utility CSS, vite injects its modulepreload polyfill.
const bundledTooling = new Set(["tailwindcss", "vite"]);

// Inter ships as a font file rather than a package, so its OFL lives beside it.
const staticTerms = [
  {
    title: "Inter 4.001 (git-66647c0bb), SIL Open Font License 1.1",
    file: "src/assets/fonts/OFL.txt",
  },
];

const termPattern = /^(licen[cs]e|copying|notice|patents)/i;

function fail(message) {
  process.stderr.write(`${message}\n`);
  process.exit(1);
}

// shippedPackages lists this package's installed dependency tree via pnpm,
// which resolves the workspace store layout.
function shippedPackages() {
  let out;
  try {
    out = execSync("pnpm --filter web ls --depth 100 --json", { encoding: "utf8", stdio: ["ignore", "pipe", "ignore"] });
  } catch {
    fail("pnpm list failed; run pnpm install --frozen-lockfile at the repo root");
  }
  const seen = new Map();
  const walk = (deps, dev) => {
    for (const [name, meta] of Object.entries(deps || {})) {
      if (!seen.has(name)) seen.set(name, { name, version: meta.version, dir: meta.path, dev });
      walk(meta.dependencies, dev);
    }
  };
  for (const proj of JSON.parse(out)) {
    walk(proj.dependencies, false);
    walk(proj.devDependencies, true);
  }
  return [...seen.values()]
    .filter((e) => e.dir && (!e.dev || bundledTooling.has(e.name)))
    .sort((a, b) => a.name.localeCompare(b.name));
}

function packageSection(pkg) {
  const terms = readdirSync(pkg.dir).filter((name) => termPattern.test(name)).sort();
  if (terms.length === 0) {
    fail(`no license terms found for ${pkg.name}`);
  }
  return terms
    .map((term) => `=== ${pkg.name} ${pkg.version} (${term}) ===\n\n${readFileSync(join(pkg.dir, term), "utf8")}`)
    .join("\n\n");
}

const sections = shippedPackages().map(packageSection);
for (const { title, file } of staticTerms) {
  sections.push(`=== ${title} ===\n\n${readFileSync(join(webDir, file), "utf8")}`);
}

process.stdout.write(
  "Third-party terms for the prebuilt authentication UI in proxy/web/dist.\n" +
    "Generated from the installed pnpm dependency tree at release time.\n\n\n" +
    sections.join("\n\n\n") +
    "\n",
);
