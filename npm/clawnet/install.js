#!/usr/bin/env node
"use strict";

const { existsSync, mkdirSync, copyFileSync, chmodSync } = require("fs");
const { join } = require("path");

const PLATFORMS = {
  "linux-x64":     "@chatchat/clawnet-linux-x64",
  "linux-arm64":   "@chatchat/clawnet-linux-arm64",
  "darwin-x64":    "@chatchat/clawnet-darwin-x64",
  "darwin-arm64":  "@chatchat/clawnet-darwin-arm64",
};

const key = `${process.platform}-${process.arch}`;
const pkg = PLATFORMS[key];

if (!pkg) {
  console.error(`ClawNet: unsupported platform ${key}`);
  console.error(`Supported: ${Object.keys(PLATFORMS).join(", ")}`);
  process.exit(1);
}

let binPath;
try {
  binPath = require.resolve(`${pkg}/bin/clawnet`);
} catch {
  console.error(`ClawNet: platform package ${pkg} not installed.`);
  console.error(`Try: npm install ${pkg}`);
  process.exit(1);
}

const destDir = join(__dirname, "bin");
const dest = join(destDir, "clawnet");

mkdirSync(destDir, { recursive: true });
copyFileSync(binPath, dest);
chmodSync(dest, 0o755);
