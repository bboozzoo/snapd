#!/usr/bin/env bun
/**
 * build.ts — orchestrates the full horror build pipeline:
 *   1. zig build (produces .a and .so in zig-out/lib/)
 *   2. cp .so → cmd/snap-confine-ts/dist/libsnap-confine-private.so
 *   3. bun build --compile cmd/snap-confine-ts/poc-main.ts → dist/snap-confine
 *
 * Run from repo root:
 *   bun run cmd/snap-confine-ts/build.ts
 */

import { join, dirname } from "path";
import { copyFileSync, mkdirSync } from "fs";

const repoRoot = join(import.meta.dir, "..", "..");
const zigDir = join(repoRoot, "cmd", "libsnap-confine-private-zig");
const tsDir = join(repoRoot, "cmd", "snap-confine-ts");
const distDir = join(tsDir, "dist");

async function run(cmd: string[], cwd: string): Promise<void> {
  console.log(`$ ${cmd.join(" ")}  (cwd: ${cwd})`);
  const proc = Bun.spawn(cmd, {
    cwd,
    stdout: "inherit",
    stderr: "inherit",
  });
  const code = await proc.exited;
  if (code !== 0) {
    throw new Error(`command failed with exit code ${code}: ${cmd.join(" ")}`);
  }
}

async function main(): Promise<void> {
  // Step 1: build Zig library
  await run(["zig", "build"], zigDir);

  // Step 2: copy .so into snap-confine-ts/dist/ (Bun asset embedding target)
  mkdirSync(distDir, { recursive: true });
  const soSrc = join(zigDir, "zig-out", "lib", "libsnap-confine-private.so");
  const soDst = join(distDir, "libsnap-confine-private.so");
  copyFileSync(soSrc, soDst);
  console.log(`copied ${soSrc} → ${soDst}`);

  // Step 3: bun build --compile
  const outBin = join(distDir, "snap-confine");
  await run(
    [
      "bun", "build",
      "--compile",
      "--outfile", outBin,
      join(tsDir, "poc-main.ts"),
    ],
    tsDir,
  );

  console.log(`\nbuild complete: ${outBin}`);
}

await main();
