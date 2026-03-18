#!/usr/bin/env node
"use strict";

const { existsSync, mkdirSync, copyFileSync, chmodSync } = require("fs");
const { join } = require("path");

const PLATFORMS = {
  "linux-x64":     "@cctech2077/clawnet-linux-x64",
  "linux-arm64":   "@cctech2077/clawnet-linux-arm64",
  "darwin-x64":    "@cctech2077/clawnet-darwin-x64",
  "darwin-arm64":  "@cctech2077/clawnet-darwin-arm64",
  "win32-x64":     "@cctech2077/clawnet-win32-x64",
};

const key = `${process.platform}-${process.arch}`;
const pkg = PLATFORMS[key];

if (!pkg) {
  console.error(`ClawNet: unsupported platform ${key}`);
  console.error(`Supported: ${Object.keys(PLATFORMS).join(", ")}`);
  process.exit(1);
}

const isWindows = process.platform === "win32";
const binName = isWindows ? "clawnet.exe" : "clawnet";

let binPath;
try {
  binPath = require.resolve(`${pkg}/bin/${binName}`);
} catch {
  console.error(`ClawNet: platform package ${pkg} not installed.`);
  console.error(`Try: npm install ${pkg}`);
  process.exit(1);
}

const destDir = join(__dirname, "bin");
const dest = join(destDir, binName);

mkdirSync(destDir, { recursive: true });
copyFileSync(binPath, dest);
if (!isWindows) {
  chmodSync(dest, 0o755);
}
