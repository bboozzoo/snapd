/**
 * M0 proof-of-concept test: load libsnap-confine-private.so from the embedded
 * binary asset via memfd_create + dlopen, then call sc_streq through FFI.
 *
 * This test validates the entire chain:
 *   Zig source → .so → embedded in Bun binary → memfd_create → dlopen → FFI call
 */

import { describe, test, expect, beforeAll } from "bun:test";
import { ptr } from "bun:ffi";
import { loadLibSnap, type LibSnap } from "../src/ffi/loader.ts";

let lib: LibSnap;

beforeAll(async () => {
  lib = await loadLibSnap();
});

// Helper: encode a JS string as a null-terminated Buffer for FFI
function cstr(s: string): Buffer {
  return Buffer.from(s + "\0", "utf8");
}

describe("M0: memfd_create + dlopen roundtrip", () => {
  test("library loaded successfully", () => {
    expect(lib).toBeDefined();
    expect(lib.symbols).toBeDefined();
  });

  test("sc_streq: equal strings", () => {
    const a = cstr("hello");
    const b = cstr("hello");
    expect(lib.symbols.sc_streq(ptr(a), ptr(b))).toBe(true);
  });

  test("sc_streq: different strings", () => {
    const a = cstr("hello");
    const b = cstr("world");
    expect(lib.symbols.sc_streq(ptr(a), ptr(b))).toBe(false);
  });

  test("sc_streq: empty strings are equal", () => {
    const a = cstr("");
    const b = cstr("");
    expect(lib.symbols.sc_streq(ptr(a), ptr(b))).toBe(true);
  });

  test("sc_startswith: prefix match", () => {
    const s = cstr("foobar");
    const p = cstr("foo");
    expect(lib.symbols.sc_startswith(ptr(s), ptr(p))).toBe(true);
  });

  test("sc_startswith: no match", () => {
    const s = cstr("foobar");
    const p = cstr("baz");
    expect(lib.symbols.sc_startswith(ptr(s), ptr(p))).toBe(false);
  });

  test("sc_endswith: suffix match", () => {
    const s = cstr("foobar");
    const x = cstr("bar");
    expect(lib.symbols.sc_endswith(ptr(s), ptr(x))).toBe(true);
  });

  test("sc_endswith: no match", () => {
    const s = cstr("foobar");
    const x = cstr("baz");
    expect(lib.symbols.sc_endswith(ptr(s), ptr(x))).toBe(false);
  });

  test("loading twice returns the same cached handle", async () => {
    const lib2 = await loadLibSnap();
    expect(lib2).toBe(lib);
  });
});
