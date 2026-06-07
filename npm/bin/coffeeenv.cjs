#!/usr/bin/env node
"use strict";

// Wrapper that execs the bundled coffeeenv binary for the current platform.
// All platform binaries are shipped under vendor/<goos>-<goarch>/.

const { spawnSync } = require("node:child_process");
const path = require("node:path");
const fs = require("node:fs");

// node platform/arch -> Go GOOS/GOARCH directory name.
const PLATFORMS = {
  "darwin-arm64": "darwin-arm64",
  "darwin-x64": "darwin-amd64",
  "linux-x64": "linux-amd64",
  "linux-arm64": "linux-arm64",
  "win32-x64": "windows-amd64",
};

function resolveBinary() {
  const key = `${process.platform}-${process.arch}`;
  const dir = PLATFORMS[key];
  if (!dir) {
    throw new Error(
      `coffeeenv: unsupported platform ${key}. Supported: ${Object.keys(PLATFORMS).join(", ")}.\n` +
        `Build from source: go install github.com/coffee-code-io/coffeeenv@latest`,
    );
  }
  const exe = process.platform === "win32" ? "coffeeenv.exe" : "coffeeenv";
  const bin = path.join(__dirname, "..", "vendor", dir, exe);
  if (!fs.existsSync(bin)) {
    throw new Error(`coffeeenv: binary not found at ${bin} (package may be corrupted).`);
  }
  return bin;
}

try {
  const bin = resolveBinary();
  const res = spawnSync(bin, process.argv.slice(2), { stdio: "inherit" });
  if (res.error) throw res.error;
  process.exit(res.status === null ? 1 : res.status);
} catch (err) {
  process.stderr.write(`${err.message}\n`);
  process.exit(1);
}
