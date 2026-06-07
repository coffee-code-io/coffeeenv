// Cross-compile the coffeeenv binary for every supported platform into
// npm/vendor/<goos>-<goarch>/. Run from the npm/ package; the Go module is the
// parent directory. Requires the Go toolchain.

import { execFileSync } from "node:child_process";
import { mkdirSync, rmSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const npmDir = join(__dirname, "..");
const goModuleDir = join(npmDir, ".."); // the coffeeenv Go module root
const vendorDir = join(npmDir, "vendor");

// GOOS, GOARCH pairs to build.
const TARGETS = [
  ["darwin", "arm64"],
  ["darwin", "amd64"],
  ["linux", "amd64"],
  ["linux", "arm64"],
  ["windows", "amd64"],
];

rmSync(vendorDir, { recursive: true, force: true });

for (const [goos, goarch] of TARGETS) {
  const outDir = join(vendorDir, `${goos}-${goarch}`);
  mkdirSync(outDir, { recursive: true });
  const exe = goos === "windows" ? "coffeeenv.exe" : "coffeeenv";
  const out = join(outDir, exe);
  process.stdout.write(`building ${goos}/${goarch} -> ${out}\n`);
  execFileSync(
    "go",
    ["build", "-trimpath", "-ldflags", "-s -w", "-o", out, "."],
    {
      cwd: goModuleDir,
      stdio: "inherit",
      env: { ...process.env, GOOS: goos, GOARCH: goarch, CGO_ENABLED: "0" },
    },
  );
}

process.stdout.write(`done: ${TARGETS.length} binaries in ${vendorDir}\n`);
