/**
 * poc-main.ts — M0 standalone binary entry point.
 * Demonstrates the full chain running as a compiled Bun executable.
 */

import { ptr } from "bun:ffi";
import { loadLibSnap } from "./src/ffi/loader.ts";

const lib = await loadLibSnap();

function cstr(s: string): Buffer {
  return Buffer.from(s + "\0", "utf8");
}

const a = cstr("snap-name");
const b = cstr("snap-name");
const c = cstr("other-snap");

console.log(`sc_streq("snap-name", "snap-name") = ${lib.symbols.sc_streq(ptr(a), ptr(b))}`);
console.log(`sc_streq("snap-name", "other-snap") = ${lib.symbols.sc_streq(ptr(a), ptr(c))}`);
console.log(`sc_startswith("snap-name", "snap") = ${lib.symbols.sc_startswith(ptr(a), ptr(cstr("snap")))}`);
console.log(`sc_endswith("snap-name", "name") = ${lib.symbols.sc_endswith(ptr(a), ptr(cstr("name")))}`);
console.log("M0 proof-of-concept: all good");
