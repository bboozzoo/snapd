/**
 * ffi/libc.ts — bootstrap libc bindings
 *
 * This is the first FFI handle opened. It is opened directly against libc.so.6
 * (always present on the system) and provides the primitives needed to load
 * everything else, including memfd_create which is used to load the embedded
 * libsnap-confine-private.so without writing to disk.
 */

import { dlopen, FFIType } from "bun:ffi";

const { ptr, i32, u32, u64, isize, usize, bool: ffiBool, void: ffiVoid, cstring } = FFIType;

export const libc = dlopen("libc.so.6", {
  // --- memory fd (for .so embedding) ---
  memfd_create: {
    args: [ptr, u32],
    returns: i32,
  },
  write: {
    args: [i32, ptr, usize],
    returns: isize,
  },
  close: {
    args: [i32],
    returns: i32,
  },

  // --- process / identity ---
  getpid: {
    args: [],
    returns: i32,
  },
  getuid: {
    args: [],
    returns: u32,
  },
  getgid: {
    args: [],
    returns: u32,
  },

  // --- filesystem ---
  access: {
    args: [ptr, i32],
    returns: i32,
  },
});

export type Libc = typeof libc;
