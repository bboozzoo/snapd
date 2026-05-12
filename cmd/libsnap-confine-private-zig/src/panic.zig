//! panic.zig — port of panic.c / panic.h

const std = @import("std");
const C = @import("libc_extras.zig");

// ---------------------------------------------------------------------------
// Function-pointer types (matching the C typedefs)
// ---------------------------------------------------------------------------

pub const ScPanicExitFn = ?*const fn () callconv(.c) void;
pub const ScPanicMsgFn = ?*const fn ([*:0]const u8, *C.va_list, c_int) callconv(.c) void;

// ---------------------------------------------------------------------------
// Module-level state
// ---------------------------------------------------------------------------

var panic_exit_fn: ScPanicExitFn = null;
var panic_msg_fn: ScPanicMsgFn = null;

// ---------------------------------------------------------------------------
// Public C-ABI exports
// ---------------------------------------------------------------------------

export fn sc_panicv(fmt: [*:0]const u8, ap: *C.va_list) noreturn {
    const errno_copy = std.c._errno().*;

    if (panic_msg_fn) |msg_fn| {
        msg_fn(fmt, ap, @intCast(errno_copy));
    } else {
        _ = C.vfprintf(C.stderr, fmt, ap);
        if (errno_copy != 0) {
            _ = C.fprintf(C.stderr, ": %s\n", C.strerror(errno_copy));
        } else {
            _ = C.fprintf(C.stderr, "\n");
        }
    }

    if (panic_exit_fn) |exit_fn| {
        exit_fn();
    }
    std.c.exit(1);
}

export fn sc_panic(fmt: [*:0]const u8, ...) noreturn {
    // We can't forward variadic args to sc_panicv without va_list, so format
    // through vfprintf directly.
    var ap = @cVaStart();
    defer @cVaEnd(&ap);
    // Copy errno before any further calls.
    const errno_copy = std.c._errno().*;
    if (panic_exit_fn) |exit_fn| {
        _ = C.vfprintf(C.stderr, fmt, &ap);
        if (errno_copy != 0) {
            _ = C.fprintf(C.stderr, ": %s\n", C.strerror(errno_copy));
        } else {
            _ = C.fprintf(C.stderr, "\n");
        }
        exit_fn();
    } else {
        _ = C.vfprintf(C.stderr, fmt, &ap);
        if (errno_copy != 0) {
            _ = C.fprintf(C.stderr, ": %s\n", C.strerror(errno_copy));
        } else {
            _ = C.fprintf(C.stderr, "\n");
        }
    }
    std.c.exit(1);
}

export fn sc_set_panic_exit_fn(fn_ptr: ScPanicExitFn) ScPanicExitFn {
    const old = panic_exit_fn;
    panic_exit_fn = fn_ptr;
    return old;
}

export fn sc_set_panic_msg_fn(fn_ptr: ScPanicMsgFn) ScPanicMsgFn {
    const old = panic_msg_fn;
    panic_msg_fn = fn_ptr;
    return old;
}

// ---------------------------------------------------------------------------
// Internal Zig helper: die() — called by other Zig modules
// ---------------------------------------------------------------------------

pub fn die(comptime fmt: []const u8, args: anytype) noreturn {
    var buf: [1024]u8 = undefined;
    const msg = std.fmt.bufPrintZ(&buf, fmt, args) catch "<die: message too long>";
    _ = C.fprintf(C.stderr, "%s\n", msg.ptr);
    if (panic_exit_fn) |exit_fn| exit_fn();
    std.c.exit(1);
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

test "sc_set_panic_exit_fn round-trips" {
    const old = sc_set_panic_exit_fn(null);
    _ = sc_set_panic_exit_fn(old);
}

test "sc_set_panic_msg_fn round-trips" {
    const old = sc_set_panic_msg_fn(null);
    _ = sc_set_panic_msg_fn(old);
}
