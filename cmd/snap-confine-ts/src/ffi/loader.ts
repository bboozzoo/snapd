/**
 * ffi/loader.ts — embed libsnap-confine-private.so in the binary and load it
 * at runtime via memfd_create + dlopen.
 *
 * The .so is embedded as a binary asset using Bun's `with { type: "file" }`
 * import attribute. At build time (bun build --compile) the bytes are baked
 * into the ELF. At runtime we write them into an anonymous in-memory file
 * (memfd_create) and dlopen it via /proc/self/fd/<n>. No disk write, no
 * cleanup required — the kernel frees it when all fds referencing it close.
 */

import { dlopen, FFIType, ptr } from "bun:ffi";
import { libc } from "./libc.ts";

// Bun embeds this file at build time. During development it resolves to a
// path on disk relative to this source file.
import soPath from "../../dist/libsnap-confine-private.so" with { type: "file" };

const { i32, u32, usize, bool: ffiBool, void: ffiVoid, ptr: ffiPtr, cstring } = FFIType;

// MFD_CLOEXEC — close the fd on execv so it doesn't leak into snap-exec
const MFD_CLOEXEC = 1;

// Symbol table for libsnap-confine-private — grows as we port more modules
const libsnapSymbols = {
  // string_utils
  sc_streq:                { args: [ffiPtr, ffiPtr],         returns: ffiBool },
  sc_startswith:           { args: [ffiPtr, ffiPtr],         returns: ffiBool },
  sc_endswith:             { args: [ffiPtr, ffiPtr],         returns: ffiBool },
  sc_strdup:               { args: [ffiPtr],                 returns: ffiPtr  },
  sc_string_append:        { args: [ffiPtr, usize, ffiPtr],  returns: usize   },
  sc_string_append_char:   { args: [ffiPtr, usize, i32],     returns: usize   },
  sc_string_init:          { args: [ffiPtr, usize],          returns: ffiVoid },
  sc_string_quote:         { args: [ffiPtr, usize, ffiPtr],  returns: ffiVoid },
  sc_string_split:         { args: [ffiPtr, i32, ffiPtr, usize, ffiPtr, usize], returns: ffiVoid },
  sc_str_chomp:            { args: [ffiPtr],                 returns: ffiPtr  },
} as const;

export type LibSnap = ReturnType<typeof dlopen<typeof libsnapSymbols>>;

let _libsnap: LibSnap | null = null;

/**
 * Load libsnap-confine-private.so from the embedded binary asset.
 * Safe to call multiple times — returns the cached handle after first load.
 */
export async function loadLibSnap(): Promise<LibSnap> {
  if (_libsnap !== null) return _libsnap;

  // Step 1: read the embedded .so bytes
  const soBytes = await Bun.file(soPath).arrayBuffer();
  const soView = new Uint8Array(soBytes);

  // Step 2: create an anonymous in-memory file
  const name = Buffer.from("libsnap-confine-private\0");
  const memfd = libc.symbols.memfd_create(ptr(name), MFD_CLOEXEC);
  if (memfd < 0) {
    throw new Error(`memfd_create failed: ${memfd}`);
  }

  // Step 3: write the .so bytes into the memory fd
  // Note: write() returns isize which bun:ffi gives us as bigint
  const written = libc.symbols.write(memfd, ptr(soView), soView.byteLength);
  if (BigInt(written) !== BigInt(soView.byteLength)) {
    libc.symbols.close(memfd);
    throw new Error(`write to memfd failed: wrote ${written} of ${soView.byteLength} bytes`);
  }

  // Step 4: dlopen via /proc/self/fd/<n>
  // Linux resolves this symlink to the memfd contents — no disk path needed.
  const fdPath = `/proc/self/fd/${memfd}`;
  _libsnap = dlopen(fdPath, libsnapSymbols);

  // Step 5: close the fd — the .so stays loaded in memory via the dlopen handle
  libc.symbols.close(memfd);

  return _libsnap;
}
