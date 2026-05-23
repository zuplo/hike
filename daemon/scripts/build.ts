// Build hike-fleetd as a single static binary per target platform via Bun's
// --compile flag. Run with: bun run scripts/build.ts
//
// Output goes to daemon/dist/hike-fleetd-<target>.
import { mkdirSync } from "node:fs";
import { join } from "node:path";
import { $ } from "bun";

const targets = [
  "bun-darwin-arm64",
  "bun-darwin-x64",
  "bun-linux-x64",
  "bun-linux-arm64",
];

const outDir = join(import.meta.dir, "..", "dist");
mkdirSync(outDir, { recursive: true });

const entry = join(import.meta.dir, "..", "src", "main.ts");

for (const target of targets) {
  const outfile = join(outDir, `hike-fleetd-${target.replace(/^bun-/, "")}`);
  console.log(`Building ${target} → ${outfile}`);
  await $`bun build --compile --minify --sourcemap --target=${target} --outfile ${outfile} ${entry}`;
}

console.log("\nDone. Copy the appropriate binary to ~/.hike/bin/hike-fleetd");
