//! snap.zig — port of snap.c / snap.h

const std = @import("std");
const C = @import("libc_extras.zig");
const error_mod = @import("error.zig");
const panic_mod = @import("panic.zig");
const string_utils = @import("string_utils.zig");

const ScError = error_mod.ScError;

pub const SC_SNAP_DOMAIN = "snap";
pub const SC_SNAP_INVALID_NAME: c_int = 1;
pub const SC_SNAP_INVALID_INSTANCE_KEY: c_int = 2;
pub const SC_SNAP_INVALID_INSTANCE_NAME: c_int = 3;
pub const SC_SNAP_MOUNT_DIR_UNSUPPORTED: c_int = 4;
pub const SC_SNAP_INVALID_COMPONENT: c_int = 5;

pub const SNAP_NAME_LEN = 40;
pub const SNAP_INSTANCE_KEY_LEN = 10;
pub const SNAP_INSTANCE_LEN = SNAP_NAME_LEN + 1 + SNAP_INSTANCE_KEY_LEN;
pub const SNAP_SECURITY_TAG_MAX_LEN = 256;

// ---------------------------------------------------------------------------
// Helpers — hand-coded name parser
// ---------------------------------------------------------------------------

fn skip_lowercase_letters(p: *[*]const u8) usize {
    var n: usize = 0;
    while (p.*[n] >= 'a' and p.*[n] <= 'z') n += 1;
    p.* = p.*[n..];
    return n;
}

fn skip_digits(p: *[*]const u8) usize {
    var n: usize = 0;
    while (p.*[n] >= '0' and p.*[n] <= '9') n += 1;
    p.* = p.*[n..];
    return n;
}

fn skip_one_char(p: *[*]const u8, c: u8) usize {
    if (p.*[0] == c) { p.* = p.*[1..]; return 1; }
    return 0;
}

fn validate_as_snap_or_component_name(
    name: ?[*:0]const u8,
    err_code: c_int,
    err_subject: [*:0]const u8,
    errorp: ?*?*ScError,
) void {
    var err: ?*ScError = null;

    if (name == null) {
        err = error_mod.sc_error_init(SC_SNAP_DOMAIN, err_code, "%s cannot be NULL", err_subject);
        _ = error_mod.sc_error_forward(errorp, err);
        return;
    }
    const s = std.mem.sliceTo(name.?, 0);

    if (s.len > SNAP_NAME_LEN) {
        err = error_mod.sc_error_init(SC_SNAP_DOMAIN, err_code, "%s must be shorter than %d characters", err_subject, @as(c_int, SNAP_NAME_LEN));
        _ = error_mod.sc_error_forward(errorp, err);
        return;
    }

    var ptr: [*]const u8 = s.ptr;

    if (skip_one_char(@ptrCast(&ptr), '-') > 0) {
        err = error_mod.sc_error_init(SC_SNAP_DOMAIN, err_code, "%s cannot start with a dash", err_subject);
        _ = error_mod.sc_error_forward(errorp, err);
        return;
    }

    var got_letter = false;
    var n: usize = 0;

    while (ptr[0] != 0) {
        var mut = ptr;
        var m = skip_lowercase_letters(&mut);
        if (m > 0) { ptr = mut; n += m; got_letter = true; continue; }
        m = skip_digits(&mut);
        if (m > 0) { ptr = mut; n += m; continue; }
        if (skip_one_char(&mut, '-') > 0) {
            ptr = mut;
            n += 1;
            if (ptr[0] == 0) {
                err = error_mod.sc_error_init(SC_SNAP_DOMAIN, err_code, "%s cannot end with a dash", err_subject);
                break;
            }
            var mut2 = ptr;
            if (skip_one_char(&mut2, '-') > 0) {
                err = error_mod.sc_error_init(SC_SNAP_DOMAIN, err_code, "%s cannot contain two consecutive dashes", err_subject);
                break;
            }
            continue;
        }
        err = error_mod.sc_error_init(SC_SNAP_DOMAIN, err_code, "%s must use lower case letters, digits or dashes", err_subject);
        break;
    }

    if (err == null) {
        if (!got_letter)
            err = error_mod.sc_error_init(SC_SNAP_DOMAIN, err_code, "%s must contain at least one letter", err_subject);
        if (err == null and n < 2)
            err = error_mod.sc_error_init(SC_SNAP_DOMAIN, err_code, "%s must be longer than 1 character", err_subject);
    }

    _ = error_mod.sc_error_forward(errorp, err);
}

// ---------------------------------------------------------------------------
// C-ABI exports
// ---------------------------------------------------------------------------

export fn sc_snap_name_validate(snap_name: ?[*:0]const u8, errorp: ?*?*ScError) void {
    validate_as_snap_or_component_name(snap_name, SC_SNAP_INVALID_NAME, "snap name", errorp);
}

export fn sc_instance_key_validate(instance_key: ?[*:0]const u8, errorp: ?*?*ScError) void {
    var err: ?*ScError = null;

    if (instance_key == null) {
        err = error_mod.sc_error_init(SC_SNAP_DOMAIN, SC_SNAP_INVALID_NAME, "instance key cannot be NULL");
        _ = error_mod.sc_error_forward(errorp, err);
        return;
    }
    const s = std.mem.sliceTo(instance_key.?, 0);

    for (s) |c| {
        const is_lower = c >= 'a' and c <= 'z';
        const is_digit = c >= '0' and c <= '9';
        if (!is_lower and !is_digit) {
            err = error_mod.sc_error_init(SC_SNAP_DOMAIN, SC_SNAP_INVALID_INSTANCE_KEY, "instance key must use lower case letters or digits");
            _ = error_mod.sc_error_forward(errorp, err);
            return;
        }
    }

    if (s.len == 0) {
        err = error_mod.sc_error_init(SC_SNAP_DOMAIN, SC_SNAP_INVALID_INSTANCE_KEY, "instance key must contain at least one letter or digit");
    } else if (s.len > SNAP_INSTANCE_KEY_LEN) {
        err = error_mod.sc_error_init(SC_SNAP_DOMAIN, SC_SNAP_INVALID_INSTANCE_KEY, "instance key must be shorter than 10 characters");
    }

    _ = error_mod.sc_error_forward(errorp, err);
}

export fn sc_instance_name_validate(instance_name: ?[*:0]const u8, errorp: ?*?*ScError) void {
    var err: ?*ScError = null;

    if (instance_name == null) {
        err = error_mod.sc_error_init(SC_SNAP_DOMAIN, SC_SNAP_INVALID_INSTANCE_NAME, "snap instance name cannot be NULL");
        _ = error_mod.sc_error_forward(errorp, err);
        return;
    }
    const s = std.mem.sliceTo(instance_name.?, 0);

    if (s.len > SNAP_INSTANCE_LEN) {
        err = error_mod.sc_error_init(SC_SNAP_DOMAIN, SC_SNAP_INVALID_INSTANCE_NAME, "snap instance name can be at most %d characters long", @as(c_int, SNAP_INSTANCE_LEN));
        _ = error_mod.sc_error_forward(errorp, err);
        return;
    }

    var buf: [SNAP_INSTANCE_LEN + 2]u8 = std.mem.zeroes([SNAP_INSTANCE_LEN + 2]u8);
    @memcpy(buf[0..s.len], s);

    const under1 = std.mem.indexOfScalar(u8, buf[0..s.len], '_');
    const snap_end = if (under1) |u| u else s.len;
    const rest_start = if (under1) |u| u + 1 else s.len;
    const rest = buf[rest_start..s.len];

    if (std.mem.indexOfScalar(u8, rest, '_') != null) {
        err = error_mod.sc_error_init(SC_SNAP_DOMAIN, SC_SNAP_INVALID_INSTANCE_NAME, "snap instance name can contain only one underscore");
        _ = error_mod.sc_error_forward(errorp, err);
        return;
    }

    buf[snap_end] = 0;
    const snap_name_z: [*:0]const u8 = @ptrCast(&buf[0]);
    sc_snap_name_validate(snap_name_z, errorp);
    if (errorp != null and errorp.?.* != null) return;

    if (under1 != null) {
        buf[rest_start + rest.len] = 0;
        const key_z: [*:0]const u8 = @ptrCast(&buf[rest_start]);
        sc_instance_key_validate(key_z, errorp);
    }
}

export fn sc_snap_component_validate(
    snap_component: ?[*:0]const u8,
    snap_instance: ?[*:0]const u8,
    errorp: ?*?*ScError,
) void {
    var err: ?*ScError = null;

    if (snap_component == null) {
        err = error_mod.sc_error_init(SC_SNAP_DOMAIN, SC_SNAP_INVALID_COMPONENT, "snap component cannot be NULL");
        _ = error_mod.sc_error_forward(errorp, err);
        return;
    }
    const s = std.mem.sliceTo(snap_component.?, 0);
    const plus_pos = std.mem.indexOfScalar(u8, s, '+') orelse {
        err = error_mod.sc_error_init(SC_SNAP_DOMAIN, SC_SNAP_INVALID_COMPONENT, "snap component must contain a +");
        _ = error_mod.sc_error_forward(errorp, err);
        return;
    };

    const snap_name_len = plus_pos;
    const component_name_len = s.len - plus_pos - 1;

    if (snap_name_len > SNAP_NAME_LEN) {
        err = error_mod.sc_error_init(SC_SNAP_DOMAIN, SC_SNAP_INVALID_COMPONENT, "snap name must be shorter than 40 characters");
        _ = error_mod.sc_error_forward(errorp, err);
        return;
    }
    if (component_name_len > SNAP_NAME_LEN) {
        err = error_mod.sc_error_init(SC_SNAP_DOMAIN, SC_SNAP_INVALID_COMPONENT, "component name must be shorter than 40 characters");
        _ = error_mod.sc_error_forward(errorp, err);
        return;
    }

    var snap_name_buf: [SNAP_NAME_LEN + 1]u8 = std.mem.zeroes([SNAP_NAME_LEN + 1]u8);
    var component_name_buf: [SNAP_NAME_LEN + 1]u8 = std.mem.zeroes([SNAP_NAME_LEN + 1]u8);
    @memcpy(snap_name_buf[0..snap_name_len], s[0..snap_name_len]);
    @memcpy(component_name_buf[0..component_name_len], s[plus_pos + 1 .. plus_pos + 1 + component_name_len]);

    const snap_name_z: [*:0]const u8 = @ptrCast(&snap_name_buf);
    const component_name_z: [*:0]const u8 = @ptrCast(&component_name_buf);

    validate_as_snap_or_component_name(snap_name_z, SC_SNAP_INVALID_COMPONENT, "snap name in component", errorp);
    if (errorp != null and errorp.?.* != null) return;

    validate_as_snap_or_component_name(component_name_z, SC_SNAP_INVALID_COMPONENT, "component name", errorp);
    if (errorp != null and errorp.?.* != null) return;

    if (snap_instance) |si| {
        var sn_in_instance: [SNAP_NAME_LEN + 1]u8 = std.mem.zeroes([SNAP_NAME_LEN + 1]u8);
        sc_snap_drop_instance_key(si, &sn_in_instance, sn_in_instance.len);
        const sn = std.mem.sliceTo(&sn_in_instance, 0);
        if (!std.mem.eql(u8, sn, snap_name_buf[0..snap_name_len])) {
            err = error_mod.sc_error_init(SC_SNAP_DOMAIN, SC_SNAP_INVALID_COMPONENT, "snap name in component must match snap name in instance");
            _ = error_mod.sc_error_forward(errorp, err);
        }
    }
}

export fn sc_snap_drop_instance_key(instance_name: [*:0]const u8, snap_name: [*]u8, snap_name_size: usize) void {
    sc_snap_split_instance_name(instance_name, snap_name, snap_name_size, null, 0);
}

export fn sc_snap_split_instance_name(
    instance_name: [*:0]const u8,
    snap_name: ?[*]u8,
    snap_name_size: usize,
    instance_key: ?[*]u8,
    instance_key_size: usize,
) void {
    string_utils.sc_string_split(instance_name, '_', snap_name, snap_name_size, instance_key, instance_key_size);
}

export fn sc_snap_split_snap_component(
    snap_component: [*:0]const u8,
    snap_name: ?[*]u8,
    snap_name_size: usize,
    component_name: ?[*]u8,
    component_name_size: usize,
) void {
    string_utils.sc_string_split(snap_component, '+', snap_name, snap_name_size, component_name, component_name_size);
}

export fn sc_security_tag_validate(
    security_tag: [*:0]const u8,
    snap_instance: [*:0]const u8,
    component_name: ?[*:0]const u8,
) bool {
    const tag = std.mem.sliceTo(security_tag, 0);
    if (tag.len > SNAP_SECURITY_TAG_MAX_LEN) return false;

    const whitelist_re =
        "^snap\\.([a-z0-9](-?[a-z0-9])*(_[a-z0-9]{1,10})?)(\\.[a-zA-Z0-9](-?[a-zA-Z0-9])*|(\\+([a-z0-9](-?[a-z0-9])*))?" ++
        "\\.hook\\.[a-z](-?[a-z0-9])*)$";

    var re: C.regex_t = undefined;
    if (C.regcomp(&re, whitelist_re, C.REG_EXTENDED) != 0)
        panic_mod.die("can not compile regex", .{});
    defer C.regfree(&re);

    const num_matches = 9;
    var matches: [num_matches]C.regmatch_t = undefined;
    const status = C.regexec(&re, security_tag, num_matches, &matches, 0);
    if (status != 0 or matches[1].rm_so < 0) return false;

    if (component_name) |cn| {
        if (matches[7].rm_so < 0) return false;
        const cn_s = std.mem.sliceTo(cn, 0);
        if (cn_s.len == 0) return false;
        const len: usize = @intCast(matches[7].rm_eo - matches[7].rm_so);
        if (len != cn_s.len) return false;
        const start: usize = @intCast(matches[7].rm_so);
        if (!std.mem.eql(u8, tag[start .. start + len], cn_s)) return false;
    } else {
        if (matches[7].rm_so >= 0) return false;
    }

    const inst_s = std.mem.sliceTo(snap_instance, 0);
    const m1_len: usize = @intCast(matches[1].rm_eo - matches[1].rm_so);
    if (m1_len != inst_s.len) return false;
    const m1_start: usize = @intCast(matches[1].rm_so);
    return std.mem.eql(u8, tag[m1_start .. m1_start + m1_len], inst_s);
}

export fn sc_is_hook_security_tag(security_tag: [*:0]const u8) bool {
    const whitelist_re = "^snap\\.[a-z](-?[a-z0-9])*(_[a-z0-9]{1,10})?\\.(hook\\.[a-z](-?[a-z0-9])*)$";
    var re: C.regex_t = undefined;
    if (C.regcomp(&re, whitelist_re, C.REG_EXTENDED | C.REG_NOSUB) != 0)
        panic_mod.die("can not compile regex", .{});
    defer C.regfree(&re);
    return C.regexec(&re, security_tag, 0, null, 0) == 0;
}

export fn sc_security_tag_to_unit_name(security_tag: [*:0]const u8) [*:0]u8 {
    var buf: [std.c.PATH_MAX]u8 = std.mem.zeroes([std.c.PATH_MAX]u8);
    var i: usize = 0;
    const tag = std.mem.sliceTo(security_tag, 0);
    for (tag) |c| {
        switch (c) {
            '0'...'9', 'a'...'z', 'A'...'Z', '_', '-', '.' => {
                buf[i] = c; i += 1;
            },
            '+' => {
                @memcpy(buf[i .. i + 4], "\\x2b"); i += 4;
            },
            else => panic_mod.die("unexpected character in security tag", .{}),
        }
    }
    buf[i] = 0;
    return @ptrCast(C.strdup(@as([*:0]const u8, @ptrCast(&buf))) orelse panic_mod.die("strdup failed", .{}));
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

test "sc_snap_name_validate valid names" {
    var err: ?*ScError = null;
    sc_snap_name_validate("hello", &err);
    try std.testing.expect(err == null);
    sc_snap_name_validate("hello-world", &err);
    try std.testing.expect(err == null);
}

test "sc_snap_name_validate invalid names" {
    var err: ?*ScError = null;
    sc_snap_name_validate(null, &err);
    try std.testing.expect(err != null);
    error_mod.sc_error_free(err); err = null;
    sc_snap_name_validate("-bad", &err);
    try std.testing.expect(err != null);
    error_mod.sc_error_free(err); err = null;
    sc_snap_name_validate("bad-", &err);
    try std.testing.expect(err != null);
    error_mod.sc_error_free(err); err = null;
    sc_snap_name_validate("a", &err);
    try std.testing.expect(err != null);
    error_mod.sc_error_free(err);
}

test "sc_instance_key_validate" {
    var err: ?*ScError = null;
    sc_instance_key_validate("abc", &err);
    try std.testing.expect(err == null);
    sc_instance_key_validate("ABC", &err);
    try std.testing.expect(err != null);
    error_mod.sc_error_free(err); err = null;
    sc_instance_key_validate("", &err);
    try std.testing.expect(err != null);
    error_mod.sc_error_free(err);
}

test "sc_security_tag_validate" {
    try std.testing.expect(sc_security_tag_validate("snap.hello.app", "hello", null));
    try std.testing.expect(!sc_security_tag_validate("snap.hello.app", "world", null));
    try std.testing.expect(sc_security_tag_validate("snap.hello.hook.install", "hello", null));
    try std.testing.expect(!sc_security_tag_validate("snap.hello+comp.hook.install", "hello", null));
    try std.testing.expect(sc_security_tag_validate("snap.hello+comp.hook.install", "hello", "comp"));
}

test "sc_is_hook_security_tag" {
    try std.testing.expect(sc_is_hook_security_tag("snap.hello.hook.install"));
    try std.testing.expect(!sc_is_hook_security_tag("snap.hello.app"));
}

test "sc_security_tag_to_unit_name" {
    const result = sc_security_tag_to_unit_name("snap.name+comp.hook.install");
    defer std.c.free(@ptrCast(result));
    try std.testing.expectEqualStrings("snap.name\\x2bcomp.hook.install", std.mem.sliceTo(result, 0));
}

test "sc_snap_drop_instance_key" {
    var name: [64]u8 = std.mem.zeroes([64]u8);
    sc_snap_drop_instance_key("snap_inst", &name, name.len);
    try std.testing.expectEqualStrings("snap", std.mem.sliceTo(&name, 0));
}

test "sc_snap_split_instance_name" {
    var snap_name: [64]u8 = std.mem.zeroes([64]u8);
    var instance_key: [64]u8 = std.mem.zeroes([64]u8);
    sc_snap_split_instance_name("snap_inst", &snap_name, snap_name.len, &instance_key, instance_key.len);
    try std.testing.expectEqualStrings("snap", std.mem.sliceTo(&snap_name, 0));
    try std.testing.expectEqualStrings("inst", std.mem.sliceTo(&instance_key, 0));
}
