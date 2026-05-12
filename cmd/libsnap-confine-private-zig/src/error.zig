//! error.zig — port of error.c / error.h

const std = @import("std");
const C = @import("libc_extras.zig");
const panic_mod = @import("panic.zig");

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

pub const SC_ERRNO_DOMAIN = "errno";
pub const SC_LIBSNAP_DOMAIN = "libsnap-confine-private";

pub const SC_UNSPECIFIED_ERROR: c_int = 0;
pub const SC_API_MISUSE: c_int = 1;
pub const SC_BUG: c_int = 2;

// ---------------------------------------------------------------------------
// sc_error struct
// ---------------------------------------------------------------------------

pub const ScError = extern struct {
    domain: [*:0]const u8,
    code: c_int,
    msg: [*:0]u8,
};

// ---------------------------------------------------------------------------
// Internal helper
// ---------------------------------------------------------------------------

fn sc_error_initv(domain: [*:0]const u8, code: c_int, msgfmt: [*:0]const u8, ap: *C.va_list) *ScError {
    const err_ptr = std.c.calloc(1, @sizeOf(ScError)) orelse
        panic_mod.die("cannot allocate memory for error object", .{});
    const err: *ScError = @ptrCast(@alignCast(err_ptr));
    err.domain = domain;
    err.code = code;
    var msg: ?[*]u8 = null;
    if (C.vasprintf(&msg, msgfmt, ap) == -1) {
        panic_mod.die("cannot format error message", .{});
    }
    err.msg = @ptrCast(msg.?);
    return err;
}

// ---------------------------------------------------------------------------
// Public C-ABI exports
// ---------------------------------------------------------------------------

pub export fn sc_error_init(domain: [*:0]const u8, code: c_int, msgfmt: [*:0]const u8, ...) *ScError {
    var ap = @cVaStart();
    defer @cVaEnd(&ap);
    return sc_error_initv(domain, code, msgfmt, &ap);
}

pub export fn sc_error_init_from_errno(errno_copy: c_int, msgfmt: [*:0]const u8, ...) *ScError {
    var ap = @cVaStart();
    defer @cVaEnd(&ap);
    return sc_error_initv(SC_ERRNO_DOMAIN, errno_copy, msgfmt, &ap);
}

pub export fn sc_error_init_simple(msgfmt: [*:0]const u8, ...) *ScError {
    var ap = @cVaStart();
    defer @cVaEnd(&ap);
    return sc_error_initv(SC_LIBSNAP_DOMAIN, SC_UNSPECIFIED_ERROR, msgfmt, &ap);
}

pub export fn sc_error_init_api_misuse(msgfmt: [*:0]const u8, ...) *ScError {
    var ap = @cVaStart();
    defer @cVaEnd(&ap);
    return sc_error_initv(SC_LIBSNAP_DOMAIN, SC_API_MISUSE, msgfmt, &ap);
}

pub export fn sc_error_domain(err: *ScError) [*:0]const u8 {
    return err.domain;
}

pub export fn sc_error_code(err: *ScError) c_int {
    return err.code;
}

pub export fn sc_error_msg(err: *ScError) [*:0]const u8 {
    return err.msg;
}

pub export fn sc_error_free(err: ?*ScError) void {
    if (err) |e| {
        std.c.free(@ptrCast(e.msg));
        std.c.free(@ptrCast(e));
    }
}

pub export fn sc_cleanup_error(ptr: **?*ScError) void {
    sc_error_free(ptr.*.*);
    ptr.*.* = null;
}

pub export fn sc_die_on_error(error_ptr: ?*ScError) void {
    if (error_ptr) |err| {
        if (std.mem.orderZ(u8, err.domain, SC_ERRNO_DOMAIN) == .eq) {
            _ = C.fprintf(C.stderr, "%s: %s\n", err.msg, C.strerror(err.code));
        } else {
            _ = C.fprintf(C.stderr, "%s\n", err.msg);
        }
        sc_error_free(err);
        std.c.exit(1);
    }
}

pub export fn sc_error_forward(recipient: ?*?*ScError, error_ptr: ?*ScError) c_int {
    if (recipient) |r| {
        r.* = error_ptr;
    } else {
        sc_die_on_error(error_ptr);
    }
    return if (error_ptr != null) -1 else 0;
}

pub export fn sc_error_match(error_ptr: ?*ScError, domain: [*:0]const u8, code: c_int) bool {
    const err = error_ptr orelse return false;
    return std.mem.orderZ(u8, err.domain, domain) == .eq and err.code == code;
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

test "sc_error_init and free" {
    const err = sc_error_init(SC_LIBSNAP_DOMAIN, SC_UNSPECIFIED_ERROR, "test error %d", @as(c_int, 42));
    defer sc_error_free(err);
    try std.testing.expectEqualStrings(SC_LIBSNAP_DOMAIN, std.mem.sliceTo(sc_error_domain(err), 0));
    try std.testing.expect(sc_error_code(err) == SC_UNSPECIFIED_ERROR);
    try std.testing.expectEqualStrings("test error 42", std.mem.sliceTo(sc_error_msg(err), 0));
}

test "sc_error_init_simple" {
    const err = sc_error_init_simple("simple %s", "msg");
    defer sc_error_free(err);
    try std.testing.expectEqualStrings(SC_LIBSNAP_DOMAIN, std.mem.sliceTo(sc_error_domain(err), 0));
    try std.testing.expect(sc_error_code(err) == SC_UNSPECIFIED_ERROR);
}

test "sc_error_init_api_misuse" {
    const err = sc_error_init_api_misuse("bad usage");
    defer sc_error_free(err);
    try std.testing.expect(sc_error_code(err) == SC_API_MISUSE);
}

test "sc_error_init_from_errno" {
    const err = sc_error_init_from_errno(2, "no such file");
    defer sc_error_free(err);
    try std.testing.expectEqualStrings(SC_ERRNO_DOMAIN, std.mem.sliceTo(sc_error_domain(err), 0));
    try std.testing.expect(sc_error_code(err) == 2);
}

test "sc_error_free null is ok" {
    sc_error_free(null);
}

test "sc_error_match" {
    const err = sc_error_init(SC_LIBSNAP_DOMAIN, SC_API_MISUSE, "oops");
    defer sc_error_free(err);
    try std.testing.expect(sc_error_match(err, SC_LIBSNAP_DOMAIN, SC_API_MISUSE));
    try std.testing.expect(!sc_error_match(err, SC_ERRNO_DOMAIN, SC_API_MISUSE));
    try std.testing.expect(!sc_error_match(null, SC_LIBSNAP_DOMAIN, 0));
}

test "sc_error_forward to recipient" {
    var recipient: ?*ScError = undefined;
    const err = sc_error_init_simple("fwd");
    const rc = sc_error_forward(&recipient, err);
    try std.testing.expect(rc == -1);
    try std.testing.expect(recipient == err);
    sc_error_free(recipient);
}

test "sc_error_forward null error returns 0" {
    var recipient: ?*ScError = null;
    const rc = sc_error_forward(&recipient, null);
    try std.testing.expect(rc == 0);
    try std.testing.expect(recipient == null);
}
