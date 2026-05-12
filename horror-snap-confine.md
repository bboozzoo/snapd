# horror-snap-confine

> A completely unnecessary, deeply irresponsible, and frankly magnificent rewrite
> of `snap-confine` in TypeScript (via Bun), with `libsnap-confine-private`
> rewritten in Zig. This document is the plan.

---

## Overview

`snap-confine` is a setuid-root binary that sets up mount namespaces, applies
AppArmor transitions, loads seccomp BPF filters, manages cgroups, and ultimately
`execv()`s into `snap-exec`. It is a security-critical piece of infrastructure.

We are going to rewrite it in TypeScript.

### Goals

1. **`libsnap-confine-private`** → rewritten in **Zig**. Produces:
   - `libsnap-confine-private.a` — static archive, linked into the other C tools
     (`snap-discard-ns`, `system-shutdown`, `snapd-generator`, etc.) which are
     not part of this horror show and remain unchanged.
   - `libsnap-confine-private.so` — shared object, embedded inside the Bun binary.

2. **`snap-confine`** → rewritten in **TypeScript**, compiled with `bun build
   --compile` into a standalone ELF that embeds the ~98 MB Bun runtime. The `.so`
   is embedded as a binary asset and loaded at runtime via `memfd_create` +
   `dlopen`.

3. **Tests**:
   - `zig test` — unit tests for the Zig library (replaces GLib `g_test_*`).
   - `bun test` — unit tests for the TypeScript snap-confine modules.
   - The existing C unit test suite for `libsnap-confine-private` (GLib-based)
     continues to link the `.a` artifact and must keep passing throughout.

### Non-goals

- Correctness under adversarial conditions (this is for fun).
- Keeping the binary size under control (Bun embeds ~98 MB of runtime).
- Using `async/await` appropriately (we will use it everywhere, including on
  `mount(2)` wrappers).
- Following any security best practice not already forced upon us by the
  existing C code.

---

## Repository Layout

```
cmd/
├── libsnap-confine-private/        # existing C (unchanged, still used by C tools)
│   └── *.c, *.h                    # kept for other consumers
│
├── libsnap-confine-private-zig/    # NEW: Zig rewrite of the library
│   ├── build.zig                   # zig build script
│   ├── src/
│   │   ├── root.zig                # library root, re-exports all modules
│   │   ├── snap.zig                # name validation (sc_snap_name_validate etc.)
│   │   ├── string_utils.zig        # sc_streq, sc_startswith, sc_endswith, etc.
│   │   ├── locking.zig             # sc_lock_global, sc_lock_snap, flock-based
│   │   ├── utils.zig               # die, debug, sc_nonfatal_mkpath, etc.
│   │   ├── panic.zig               # sc_panic, pluggable exit/msg fn hooks
│   │   ├── error.zig               # sc_error, domain/code/msg struct
│   │   ├── cleanup.zig             # SC_CLEANUP equivalents (defer in Zig)
│   │   ├── apparmor_support.zig    # sc_init_apparmor_support, aa_change_onexec
│   │   ├── cgroup_support.zig      # cgroup v1/v2 detection and management
│   │   ├── cgroup_freezer.zig      # sc_cgroup_freezer_join
│   │   ├── classic.zig             # sc_classify_distro, sc_is_debian_like
│   │   ├── device_cgroup.zig       # sc_device_cgroup_new, allow_device
│   │   ├── feature.zig             # sc_feature_enabled
│   │   ├── infofile.zig            # sc_infofile_get_key
│   │   ├── mount_opt.zig           # sc_do_mount, sc_do_umount
│   │   ├── mountinfo.zig           # /proc/self/mountinfo parser
│   │   ├── privs.zig               # sc_privs_drop, sc_cap_set_ambient
│   │   ├── secure_getenv.zig       # secure_getenv fallback
│   │   ├── snap_dir.zig            # sc_probe_snap_mount_dir_from_pid_1_mount_ns
│   │   ├── bpf_support.zig         # BPF syscall wrappers (conditional)
│   │   └── tool.zig                # sc_open_snap_update_ns, sc_call_snap_update_ns
│   └── include/
│       └── libsnap-confine-private.h   # C header exposing the Zig exports
│                                       # (identical API to the existing .h files)
│
└── snap-confine-ts/                # NEW: TypeScript rewrite of snap-confine
    ├── package.json
    ├── tsconfig.json
    ├── build.ts                    # orchestrates: zig build → bun build --compile
    ├── src/
    │   ├── main.ts                 # entry point — the full execution pipeline
    │   ├── ffi/
    │   │   ├── loader.ts           # memfd_create + dlopen bootstrap
    │   │   ├── libc.ts             # raw libc bindings (mount, umount2, etc.)
    │   │   ├── libcap.ts           # libcap bindings (cap_get_proc, cap_set_proc)
    │   │   ├── libapparmor.ts      # libapparmor bindings (aa_change_onexec)
    │   │   ├── libudev.ts          # libudev bindings
    │   │   └── libsnap.ts          # bindings into libsnap-confine-private.so
    │   ├── args.ts                 # argument parsing (sc_nonfatal_parse_args)
    │   ├── invocation.ts           # sc_invocation struct and init
    │   ├── apparmor-support.ts     # AppArmor transition
    │   ├── cgroup-support.ts       # cgroup v1/v2
    │   ├── classic.ts              # distro classification
    │   ├── cookie-support.ts       # snap cookie from snapd
    │   ├── feature.ts              # snapd feature flags
    │   ├── group-policy.ts         # local group policy enforcement
    │   ├── locking.ts              # global and per-snap locks
    │   ├── mount-support.ts        # mount namespace population + pivot_root
    │   ├── mount-support-nvidia.ts # nvidia driver bind-mounts
    │   ├── mountinfo.ts            # /proc/self/mountinfo parser
    │   ├── ns-support.ts           # namespace lifecycle
    │   ├── panic.ts                # die() equivalent
    │   ├── privs.ts                # capability management
    │   ├── seccomp-support.ts      # seccomp BPF loading
    │   ├── snap.ts                 # name validation
    │   ├── snap-dir.ts             # SNAP_MOUNT_DIR probe
    │   ├── string-utils.ts         # string primitives
    │   ├── udev-support.ts         # device cgroup via udev
    │   ├── user-support.ts         # SNAP_USER_DATA directory creation
    │   └── utils.ts                # debug, getenv_bool, mkpath, etc.
    ├── tests/
    │   ├── args.test.ts
    │   ├── invocation.test.ts
    │   ├── snap.test.ts
    │   ├── string-utils.test.ts
    │   ├── cookie-support.test.ts
    │   ├── mountinfo.test.ts
    │   ├── locking.test.ts
    │   ├── feature.test.ts
    │   ├── infofile.test.ts
    │   ├── classic.test.ts
    │   ├── group-policy.test.ts
    │   ├── seccomp-support.test.ts
    │   ├── ns-support.test.ts
    │   └── mount-support.test.ts
    └── dist/
        └── (generated — not committed)
            ├── libsnap-confine-private.so   # built by zig, embedded by bun
            ├── libsnap-confine-private.a    # built by zig, for C consumers
            └── snap-confine                 # final bun --compile artifact
```

---

## The `.so` Embedding Trick

`bun build --compile` embeds binary assets using the `with { type: "file" }`
import attribute. The `.so` is baked into the ELF at build time and loaded at
runtime without ever touching the filesystem, using Linux's `memfd_create(2)`.

### Build time

```typescript
// src/ffi/loader.ts
import soPath from "../../dist/libsnap-confine-private.so" with { type: "file" };
```

Bun reads the `.so` bytes and embeds them. The import resolves to an internal
`$bunfs/...` path string at runtime.

### Runtime

```typescript
// src/ffi/loader.ts
import { dlopen } from "bun:ffi";
import { libc } from "./libc.ts";  // bootstrapped first against libc.so directly
import soPath from "../../dist/libsnap-confine-private.so" with { type: "file" };

export async function loadLibSnap() {
  // 1. Read the embedded .so bytes
  const soBytes = await Bun.file(soPath).arrayBuffer();

  // 2. Create an anonymous in-memory file (no disk write, no cleanup needed)
  //    memfd_create("libsnap-confine-private", MFD_CLOEXEC=1)
  const memfd = libc.symbols.memfd_create(
    Buffer.from("libsnap-confine-private\0"),
    1  // MFD_CLOEXEC
  );
  if (memfd < 0) throw new Error("memfd_create failed");

  // 3. Write the .so bytes into the memory fd
  libc.symbols.write(memfd, soBytes, soBytes.byteLength);

  // 4. dlopen via /proc/self/fd/<n> — Linux resolves this to the memfd contents
  const path = `/proc/self/fd/${memfd}`;
  return dlopen(path, { /* symbol table — see libsnap.ts */ });
  // fd can be closed after dlopen; the .so stays loaded in memory
}
```

`memfd_create` itself is called via a bootstrapped `libc` handle that is
opened directly against `libc.so.6` with `dlopen` before any of this, since
`bun:ffi`'s `dlopen` is available unconditionally. `libc.so.6` is always
present on the system and does not need to be embedded.

### Why `memfd_create` over a tmpfile

- No disk write.
- No cleanup — the kernel frees it when all file descriptors referencing it
  are closed.
- Not visible in `/tmp` or anywhere else in the filesystem namespace.
- `MFD_CLOEXEC` ensures it is not inherited across `execv()` into `snap-exec`.

---

## FFI Binding Layers

All native function calls go through `bun:ffi`. There are five binding modules:

### `ffi/libc.ts` — libc

Bootstrapped first via `dlopen("libc.so.6", {...})`. Contains all syscall
wrappers needed by snap-confine that are not available as Node.js/Bun built-ins:

| Symbol | Signature | Used for |
|---|---|---|
| `memfd_create` | `(name: ptr, flags: u32) → i32` | `.so` embedding |
| `write` | `(fd: i32, buf: ptr, n: usize) → isize` | writing to memfd |
| `mount` | `(src: ptr, tgt: ptr, fstype: ptr, flags: u64, data: ptr) → i32` | all mounts |
| `umount2` | `(tgt: ptr, flags: i32) → i32` | unmounting |
| `pivot_root` | `(new: ptr, old: ptr) → i32` | the horrifying pivot |
| `setns` | `(fd: i32, nstype: i32) → i32` | joining namespaces |
| `unshare` | `(flags: i32) → i32` | creating new namespace |
| `flock` | `(fd: i32, op: i32) → i32` | advisory locking |
| `prctl` | `(opt: i32, ...args) → i32` | PR_SET_KEEPCAPS etc. |
| `getresuid` | `(r: ptr, e: ptr, s: ptr) → i32` | read UIDs |
| `getresgid` | `(r: ptr, e: ptr, s: ptr) → i32` | read GIDs |
| `setresuid` | `(r: u32, e: u32, s: u32) → i32` | set UIDs |
| `setresgid` | `(r: u32, e: u32, s: u32) → i32` | set GIDs |
| `getgroups` | `(size: i32, list: ptr) → i32` | supplementary groups |
| `fstatat` | `(dirfd: i32, path: ptr, stat: ptr, flags: i32) → i32` | stat with flags |
| `eventfd` | `(init: u32, flags: i32) → i32` | ns helper signalling |
| `pipe2` | `(pipefd: ptr, flags: i32) → i32` | ns helper commands |
| `waitpid` | `(pid: i32, wstatus: ptr, opts: i32) → i32` | helper wait |
| `alarm` | `(seconds: u32) → u32` | sanity timeout |
| `sigaction` | `(sig: i32, act: ptr, old: ptr) → i32` | SIGALRM handler |
| `umask` | `(mask: u32) → u32` | save/restore umask |
| `mkdtemp` | `(tmpl: ptr) → ptr` | scratch dir creation |
| `openat` | `(dirfd: i32, path: ptr, flags: i32, mode: u32) → i32` | safe open |
| `syscall` | variadic | `SYS_pivot_root`, `SYS_bpf` |
| `dlopen` | `(path: ptr, flags: i32) → ptr` | for udev runtime check |
| `dlsym` | `(handle: ptr, sym: ptr) → ptr` | for udev runtime check |

### `ffi/libcap.ts` — libcap

`dlopen("libcap.so.2", {...})`. Used for the capability management pipeline:

| Symbol | Used for |
|---|---|
| `cap_get_proc` | read current process capabilities |
| `cap_dup` | duplicate a cap_t |
| `cap_init` | create empty cap_t (for full drop) |
| `cap_set_flag` | set CAP_EFFECTIVE / CAP_INHERITABLE flags |
| `cap_set_proc` | apply a cap_t to the process |
| `cap_free` | free a cap_t |
| `cap_reset_ambient` | clear ambient capabilities |
| `cap_set_ambient` | set an individual ambient cap |

### `ffi/libapparmor.ts` — libapparmor

`dlopen("libapparmor.so.1", {...})`. Optional — checked at runtime:

| Symbol | Used for |
|---|---|
| `aa_is_enabled` | detect if AppArmor is active |
| `aa_getcon` | get current confinement context |
| `aa_change_onexec` | schedule profile transition on execv |

### `ffi/libudev.ts` — libudev

`dlopen("libudev.so.1", {...})`. Used for device cgroup enumeration:

| Symbol | Used for |
|---|---|
| `udev_new` | create udev context |
| `udev_unref` | release context |
| `udev_enumerate_new` | create device enumerator |
| `udev_enumerate_add_match_tag` | filter by snap udev tag |
| `udev_enumerate_scan_devices` | run the scan |
| `udev_enumerate_get_list_entry` | iterate results |
| `udev_list_entry_get_next` | iterate |
| `udev_list_entry_get_name` | get device syspath |
| `udev_device_new_from_syspath` | open a device |
| `udev_device_get_devtype` | char/block? |
| `udev_device_get_devnum` | major/minor |
| `udev_device_has_tag` | check snap tag |
| `udev_device_has_current_tag` | check current tag (systemd v247+) |
| `udev_device_unref` | release device |
| `udev_enumerate_unref` | release enumerator |

Additionally, `udev_device_has_current_tag` is resolved at runtime via a
secondary `dlsym` probe (matching the existing C code's `dlopen`-based
approach for detecting systemd v247+ support).

### `ffi/libsnap.ts` — libsnap-confine-private.so

The embedded Zig-compiled library. Full symbol table mirroring the C API:

```typescript
// All sc_* functions from the Zig library
export const libsnapSymbols = {
  sc_snap_name_validate:          { args: [ptr, ptr], returns: void_t },
  sc_instance_name_validate:      { args: [ptr, ptr], returns: void_t },
  sc_security_tag_validate:       { args: [ptr, ptr, ptr], returns: bool },
  sc_is_hook_security_tag:        { args: [ptr], returns: bool },
  sc_streq:                       { args: [ptr, ptr], returns: bool },
  sc_startswith:                  { args: [ptr, ptr], returns: bool },
  sc_endswith:                    { args: [ptr, ptr], returns: bool },
  sc_must_snprintf:               { /* variadic — called via libc sprintf instead */ },
  sc_lock_global:                 { args: [], returns: i32 },
  sc_lock_snap:                   { args: [ptr], returns: i32 },
  sc_unlock:                      { args: [i32], returns: void_t },
  sc_enable_sanity_timeout:       { args: [], returns: void_t },
  sc_disable_sanity_timeout:      { args: [], returns: void_t },
  sc_snap_is_inhibited:           { args: [ptr, i32], returns: bool },
  sc_feature_enabled:             { args: [i32], returns: bool },
  sc_infofile_get_key:            { args: [ptr, ptr, ptr, usize], returns: i32 },
  sc_classify_distro:             { args: [], returns: i32 },
  sc_is_debian_like:              { args: [], returns: bool },
  sc_do_mount:                    { args: [ptr, ptr, ptr, u64, ptr], returns: void_t },
  sc_do_optional_mount:           { args: [ptr, ptr, ptr, u64, ptr], returns: void_t },
  sc_do_umount:                   { args: [ptr, i32], returns: void_t },
  sc_parse_mountinfo:             { args: [ptr], returns: ptr },
  sc_free_mountinfo:              { args: [ptr], returns: void_t },
  sc_first_mountinfo_entry:       { args: [ptr], returns: ptr },
  sc_next_mountinfo_entry:        { args: [ptr], returns: ptr },
  sc_privs_drop:                  { args: [], returns: void_t },
  sc_cap_set_ambient:             { args: [i32], returns: void_t },
  sc_probe_snap_mount_dir:        { args: [], returns: ptr },
  sc_cgroup_is_v2:                { args: [], returns: bool },
  sc_cgroup_freezer_join:         { args: [ptr, i32], returns: void_t },
  sc_device_cgroup_new:           { args: [ptr, i32], returns: ptr },
  sc_device_cgroup_allow:         { args: [ptr, i32, i32, i32], returns: void_t },
  sc_device_cgroup_attach_pid:    { args: [ptr, i32], returns: void_t },
  sc_device_cgroup_cleanup:       { args: [ptr], returns: void_t },
  sc_nonfatal_mkpath:             { args: [ptr, u32, i32, i32], returns: i32 },
  sc_is_in_container:             { args: [], returns: bool },
  sc_wait_for_file:               { args: [ptr, usize], returns: bool },
} as const;
```

---

## Phase 1: Zig Library (`libsnap-confine-private-zig`)

### 1.1 Build system

`cmd/libsnap-confine-private-zig/build.zig` uses the Zig build system to
produce both artifacts:

```zig
// build.zig (outline)
const lib_static = b.addStaticLibrary(.{
    .name = "snap-confine-private",
    .root_source_file = .{ .path = "src/root.zig" },
    .target = target,
    .optimize = optimize,
});
lib_static.linkLibC();  // needed for POSIX APIs

const lib_shared = b.addSharedLibrary(.{
    .name = "snap-confine-private",
    .root_source_file = .{ .path = "src/root.zig" },
    .target = target,
    .optimize = optimize,
});
lib_shared.linkLibC();
```

Both are produced from identical source. The `build.ts` orchestration script
runs `zig build` before `bun build --compile`, copying the `.so` into
`snap-confine-ts/dist/` for asset embedding.

### 1.2 C ABI exports

Every public function uses `export` in Zig to get C linkage:

```zig
// src/snap.zig
const std = @import("std");
const c = @import("c.zig");  // @cImport of relevant headers

pub const SNAP_NAME_LEN: usize = 40;

export fn sc_snap_name_validate(
    snap_name: [*c]const u8,
    errorp: ?*?*c.sc_error,
) void {
    // implementation
}
```

### 1.3 C header (`include/libsnap-confine-private.h`)

A single umbrella header exposing the complete C API. This is what other C
tools (`snap-discard-ns` etc.) will include when linking the `.a`. The API is
identical to the existing per-module headers — the C consumers don't change.

### 1.4 Zig test suite

Tests are colocated with the implementation using Zig's `test` blocks. Run
with `zig build test` (or `zig test src/root.zig`):

```zig
// src/snap.zig (excerpt)
test "snap name: valid" {
    try std.testing.expect(snap_name_is_valid("my-snap"));
}

test "snap name: empty is invalid" {
    try std.testing.expect(!snap_name_is_valid(""));
}

test "snap name: too long is invalid" {
    try std.testing.expect(!snap_name_is_valid("a" ** 41));
}
```

Memory leak detection is built-in via `std.testing.allocator` (backed by
`std.heap.GeneralPurposeAllocator`) — no Valgrind needed, though the existing
C test suite still uses it.

### 1.5 Existing C test suite compatibility

The existing `cmd/libsnap-confine-private/unit-tests` (GLib-based) links
`libsnap-confine-private.a`. During the Zig rewrite phase, both the old C
library and the new Zig library are built. The C tests are updated to link
the Zig-produced `.a` instead. This is the compatibility gate: **the C tests
must pass with the Zig `.a` before the old C source is considered replaced**.

Porting order (simplest → most complex, based on dependencies):
1. `string-utils` — pure logic, no syscalls, easiest tests
2. `error` — simple heap struct
3. `panic` — thin wrapper around exit
4. `snap` — name validation, `regcomp/regexec` via libc
5. `cleanup-funcs` — Zig uses `defer`, but C-ABI cleanup fns still needed
6. `secure-getenv` — trivial fallback impl
7. `utils` — `die`, `debug`, `sc_nonfatal_mkpath`, container detection
8. `feature` — file reads
9. `infofile` — key/value file parser
10. `classic` — distro detection
11. `mountinfo` — `/proc/self/mountinfo` parser
12. `mount-opt` — `mount(2)` / `umount2(2)` wrappers
13. `locking` — `flock`, `SIGALRM`, inhibit check
14. `privs` — capability management (links libcap)
15. `apparmor-support` — (links libapparmor, optional)
16. `snap-dir` — PID 1 mount ns probe
17. `tool` — exec `snap-update-ns`
18. `cgroup-support` — cgroup v1/v2
19. `cgroup-freezer-support` — freezer cgroup
20. `device-cgroup-support` — udev + BPF device cgroup
21. `bpf-support` — raw BPF syscall wrappers (conditional on `ENABLE_BPF`)

---

## Phase 2: TypeScript snap-confine

### 2.1 Build orchestration (`build.ts`)

```typescript
// cmd/snap-confine-ts/build.ts
import { $ } from "bun";

// Step 1: build the Zig library (both .a and .so)
await $`zig build -Doptimize=ReleaseFast`.cwd("../libsnap-confine-private-zig");
await $`cp ../libsnap-confine-private-zig/zig-out/lib/libsnap-confine-private.so dist/`;

// Step 2: compile the TS snap-confine, embedding the .so as a binary asset
await Bun.build({
  entrypoints: ["./src/main.ts"],
  compile: true,
  outfile: "./dist/snap-confine",
  minify: false,  // leave readable for horror appreciation
});

console.log("Built dist/snap-confine — please install setuid root at your own risk");
```

### 2.2 `main.ts` — execution pipeline

Mirrors `snap-confine.c` step by step. The full pipeline, translated:

```typescript
// src/main.ts (outline — each step is a module call)
import { parseArgs }                    from "./args.ts";
import { initInvocation }               from "./invocation.ts";
import { initApparmorSupport,
         maybeChangeOnExec }            from "./apparmor-support.ts";
import { assertHostLocalGroupPolicy }   from "./group-policy.ts";
import { probeSnapMountDir }            from "./snap-dir.ts";
import { getCookieFromSnapd }           from "./cookie-support.ts";
import { reassociateWithPid1MountNs,
         initializeMountNs,
         openMountNs, forkHelper,
         joinPreservedNs,
         preservePopulatedMountNs }    from "./ns-support.ts";
import { ensureSharedSnapMount,
         populateMountNs }             from "./mount-support.ts";
import { lockGlobal, lockSnap, unlock } from "./locking.ts";
import { setupDeviceCgroup }            from "./udev-support.ts";
import { setupUserData }               from "./user-support.ts";
import { applySeccompProfile }         from "./seccomp-support.ts";
import { dropPrivileges,
         raiseCapSysAdmin }            from "./privs.ts";
import { loadLibSnap }                 from "./ffi/loader.ts";

// Bootstrap: load the embedded .so via memfd_create
const libsnap = await loadLibSnap();

// Step 1: parse args
const args = parseArgs(process.argv, libsnap);

// Step 2-17: mirror snap-confine.c exactly, calling module functions
// ... (full pipeline)

// Final: execv into snap-exec
// bun does not have a native execv — call it via libc FFI
libc.symbols.execv(
  Buffer.from(`${snapMountDir}/usr/lib/snapd/snap-exec\0`),
  buildExecvArgv(process.argv)
);
// unreachable
```

### 2.3 FFI design principles

- All FFI calls that can fail check return values and call `panic()` (which
  calls `process.exit(1)` with a message) rather than throwing exceptions.
- Strings are always passed as null-terminated `Buffer` objects allocated with
  `Buffer.alloc(n + 1)` — never as JS strings directly.
- File descriptors are tracked as plain numbers. There is no RAII (we tried,
  TypeScript's `using` keyword exists, but managing it across async boundaries
  is its own horror show).
- `async/await` is used throughout for aesthetic reasons, including in places
  where everything is synchronous underneath. `mount()` returning a `Promise`
  is the goal.

### 2.4 `bun test` test suite

Tests use `bun:test` (Jest-compatible):

```typescript
// tests/snap.test.ts
import { describe, test, expect } from "bun:test";
import { snapNameValidate } from "../src/snap.ts";

describe("snap name validation", () => {
  test("valid name", () => {
    expect(() => snapNameValidate("my-snap")).not.toThrow();
  });

  test("empty name throws", () => {
    expect(() => snapNameValidate("")).toThrow();
  });

  test("name too long throws", () => {
    expect(() => snapNameValidate("a".repeat(41))).toThrow();
  });

  test("invalid chars throw", () => {
    expect(() => snapNameValidate("my_snap")).toThrow();
  });
});
```

Tests that require the FFI layer (locking, mounts, namespaces) run against
the real system and require root. They are tagged and skipped in CI unless
`SNAP_CONFINE_TS_INTEGRATION=1` is set.

---

## Build Dependencies

| Tool | Version | Role |
|---|---|---|
| `zig` | 0.15.x | Compiles `libsnap-confine-private-zig` → `.a` and `.so` |
| `bun` | 1.3.x | Runs `build.ts`, compiles TS → snap-confine ELF |
| `libc` | system | Runtime FFI target (always present) |
| `libcap` | system | Runtime FFI target |
| `libapparmor` | system | Runtime FFI target (optional) |
| `libudev` | system | Runtime FFI target |

The existing autotools build for `cmd/` is **not modified**. The new builds are
entirely parallel in `libsnap-confine-private-zig/` and `snap-confine-ts/`.

---

## Milestones

### M0: Proof of concept (load the .so from memory)
- [ ] Write a minimal Zig file exporting one function (`sc_streq`).
- [ ] Build it as `.so` with `zig build`.
- [ ] Write `loader.ts` with the `memfd_create` + `dlopen` trick.
- [ ] Write a `bun test` that calls `sc_streq` via the in-memory `.so`.
- [ ] `bun build --compile` the test binary and verify it runs standalone.

### M1: Zig library — pure logic modules
Port and test (Zig tests + C test suite linking `.a`):
- [ ] `string-utils`, `error`, `panic`
- [ ] `snap` (name validation)
- [ ] `utils`, `feature`, `infofile`, `classic`
- [ ] `mountinfo`

### M2: Zig library — syscall-heavy modules
- [ ] `mount-opt`, `locking`, `privs`
- [ ] `apparmor-support`, `snap-dir`, `tool`
- [ ] `cgroup-support`, `cgroup-freezer-support`
- [ ] `device-cgroup-support`, `bpf-support`

**Gate**: all existing GLib C tests pass linking the Zig `.a`.

### M3: TS snap-confine — pure logic modules
- [ ] `ffi/loader.ts` (memfd_create + dlopen)
- [ ] `ffi/libc.ts`, `ffi/libcap.ts`, `ffi/libapparmor.ts`
- [ ] `ffi/libsnap.ts` (full symbol table)
- [ ] `args.ts`, `invocation.ts`, `snap.ts`, `string-utils.ts`
- [ ] `cookie-support.ts`, `feature.ts`, `classic.ts`, `infofile.ts`
- [ ] `bun test` suite for all of the above

### M4: TS snap-confine — namespace and mount modules
- [ ] `locking.ts`, `ns-support.ts`
- [ ] `mount-support.ts` (the big one — `pivot_root`, all bind-mounts)
- [ ] `mount-support-nvidia.ts`
- [ ] `mountinfo.ts`

### M5: TS snap-confine — security and execution modules
- [ ] `privs.ts` (capability management)
- [ ] `apparmor-support.ts`
- [ ] `seccomp-support.ts` (BPF loading + `prctl(PR_SET_SECCOMP)`)
- [ ] `udev-support.ts` (device cgroup)
- [ ] `group-policy.ts`, `user-support.ts`, `snap-dir.ts`

### M6: `main.ts` and end-to-end integration
- [ ] `main.ts` wiring the full 17-step pipeline
- [ ] `build.ts` orchestration script
- [ ] `bun build --compile` producing `dist/snap-confine`
- [ ] Manual integration test: install setuid root, run a snap, survive

### M7: Suffering and polish
- [ ] Add gratuitous `async/await` to all mount operations
- [ ] Make `pivot_root` return a `Promise<void>`
- [ ] Add JSDoc comments to all syscall wrappers explaining what they do
  in the most unnecessarily detailed way possible
- [ ] Ensure the binary is exactly 98 MB larger than it needs to be
- [ ] Write a `README.md` explaining why this was a good idea

---

## Testing Matrix

| Test suite | Runner | What it tests | Needs root |
|---|---|---|---|
| Zig unit tests | `zig build test` | Zig library pure logic | No |
| Zig unit tests (syscall) | `zig build test` | mount, locking, cgroup | Yes |
| C unit tests (libsnap-private) | `make check` | Zig `.a` via C ABI | Partial |
| `bun test` (pure logic) | `bun test` | TS modules, FFI bindings | No |
| `bun test` (integration) | `bun test` | namespaces, mounts | Yes |
| Manual | n/a | actually confines a snap | Yes (setuid) |

---

## Known Horrors

1. **`execv` is the last call** — after `execv()` there is no more JavaScript.
   The Bun runtime, the garbage collector, the event loop, the promise queue —
   all of it ceases to exist the moment the kernel overlays `snap-exec`. The
   98 MB runtime was there for nothing.

2. **`memfd_create` requires Linux 3.17+** — this is fine for any system
   running a modern `snapd`, but worth noting.

3. **`setuid` + Bun runtime** — a setuid Bun binary inherits the full Bun
   runtime as root. This includes the HTTP client, the SQLite engine, the
   WebSocket implementation, the S3 client, and the bundler. None of these
   are needed. All of them are there.

4. **No `RAII` for file descriptors** — TypeScript's `using` keyword (the
   `Symbol.dispose` protocol) works synchronously. Since we are using
   `async/await` everywhere, fd cleanup is manual. Leaks are likely. The
   `MFD_CLOEXEC` flag on the memfd at least ensures it doesn't survive into
   `snap-exec`.

5. **`pivot_root` from TypeScript** — `pivot_root(2)` is not in `libc` as a
   named function. It must be called via `syscall(SYS_pivot_root, new_root,
   put_old)` which requires knowing the syscall number (155 on x86_64). We
   call it through `libc.symbols.syscall`. On non-x86_64 architectures, the
   syscall number is different. We hardcode x86_64.

6. **The final binary is not reproducible** — Bun embeds a timestamp and
   build metadata into the executable. Every `bun build --compile` produces
   a slightly different binary. This is fine for a C binary that lives in
   `/usr/lib/snapd/snap-confine`. It is less fine for a security-critical
   setuid binary, but we have already committed to this path.
