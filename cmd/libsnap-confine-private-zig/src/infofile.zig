//! infofile.zig — port of infofile.c / infofile.h

const std = @import("std");
const C = @import("libc_extras.zig");
const error_mod = @import("error.zig");

const ScError = error_mod.ScError;

pub export fn sc_infofile_get_key(stream: *std.c.FILE, key: [*:0]const u8, value: *?[*:0]u8, err_out: ?*?*ScError) c_int {
    return sc_infofile_get_ini_section_key(stream, null, key, value, err_out);
}

export fn sc_infofile_get_ini_section_key(
    stream_opt: ?*std.c.FILE,
    section_opt: ?[*:0]const u8,
    key_opt: ?[*:0]const u8,
    value_opt: ?*?[*:0]u8,
    err_out: ?*?*ScError,
) c_int {
    var err: ?*ScError = null;

    if (stream_opt == null) {
        err = error_mod.sc_error_init_api_misuse("stream cannot be NULL");
        return fwd(err_out, err);
    }
    const stream = stream_opt.?;

    if (key_opt == null) {
        err = error_mod.sc_error_init_api_misuse("key cannot be NULL");
        return fwd(err_out, err);
    }
    const key = key_opt.?;

    if (value_opt == null) {
        err = error_mod.sc_error_init_api_misuse("value cannot be NULL");
        return fwd(err_out, err);
    }
    const value = value_opt.?;

    if (section_opt) |sec| {
        if (std.mem.len(sec) == 0) {
            err = error_mod.sc_error_init_api_misuse("section name cannot be empty");
            return fwd(err_out, err);
        }
    }

    value.* = null;

    var line_buf: ?[*:0]u8 = null;
    var line_size: usize = 0;
    var lineno: c_int = 1;
    var section_matched: bool = (section_opt == null);

    while (true) : (lineno += 1) {
        std.c._errno().* = 0;
        const nread = C.getline(&line_buf, &line_size, stream);
        if (nread < 0) {
            if (std.c._errno().* != 0) {
                err = error_mod.sc_error_init_from_errno(std.c._errno().*, "cannot read beyond line %d", lineno);
            }
            break;
        }
        if (nread == 0) break;

        const line = line_buf.?[0..@intCast(nread)];

        if (std.mem.indexOfScalar(u8, line, 0) != null) {
            err = error_mod.sc_error_init_simple("line %d contains NUL byte", lineno);
            break;
        }
        if (line[line.len - 1] != '\n') {
            err = error_mod.sc_error_init(error_mod.SC_LIBSNAP_DOMAIN, 0, "line %d does not end with a newline", lineno);
            break;
        }

        line_buf.?[@intCast(nread - 1)] = 0;
        const s = std.mem.sliceTo(line_buf.?, 0);

        if (s.len == 0) continue;
        if (s[0] == '#') continue;

        if (s[0] == '[') {
            if (section_opt == null) {
                err = error_mod.sc_error_init_simple("line %d contains unexpected section", lineno);
                break;
            }
            section_matched = false;
            const end = std.mem.indexOfScalar(u8, s[1..], ']') orelse {
                err = error_mod.sc_error_init_simple("line %d is not a valid ini section", lineno);
                break;
            };
            const sec_name = s[1 .. 1 + end];
            const wanted = std.mem.sliceTo(section_opt.?, 0);
            section_matched = std.mem.eql(u8, sec_name, wanted);
            continue;
        }

        if (!section_matched) continue;

        const eq = std.mem.indexOfScalar(u8, s, '=') orelse {
            err = error_mod.sc_error_init_simple("line %d is not a key=value assignment", lineno);
            break;
        };
        if (eq == 0) {
            err = error_mod.sc_error_init_simple("line %d contains empty key", lineno);
            break;
        }
        const scanned_key = s[0..eq];
        const scanned_value = s[eq + 1 ..];
        const wanted_key = std.mem.sliceTo(key, 0);
        if (std.mem.eql(u8, scanned_key, wanted_key)) {
            value.* = @ptrCast(C.strdup(scanned_value.ptr) orelse break);
            break;
        }
    }

    if (line_buf) |lb| std.c.free(@ptrCast(lb));
    return fwd(err_out, err);
}

inline fn fwd(recipient: ?*?*ScError, err: ?*ScError) c_int {
    if (recipient) |r| {
        r.* = err;
        return if (err != null) -1 else 0;
    } else {
        error_mod.sc_die_on_error(err);
        return 0;
    }
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

test "sc_infofile_get_key basic hit" {
    const content = "FOO=bar\nBAZ=qux\n";
    const f = std.c.fmemopen(@constCast(content.ptr), content.len, "r") orelse
        return error.SkipZigTest;
    defer _ = std.c.fclose(f);
    var val: ?[*:0]u8 = null;
    var err: ?*ScError = null;
    const rc = sc_infofile_get_key(f, "FOO", &val, &err);
    try std.testing.expect(rc == 0);
    try std.testing.expect(err == null);
    try std.testing.expect(val != null);
    defer std.c.free(@ptrCast(val));
    try std.testing.expectEqualStrings("bar", std.mem.sliceTo(val.?, 0));
}

test "sc_infofile_get_key miss returns null value" {
    const content = "FOO=bar\n";
    const f = std.c.fmemopen(@constCast(content.ptr), content.len, "r") orelse
        return error.SkipZigTest;
    defer _ = std.c.fclose(f);
    var val: ?[*:0]u8 = null;
    var err: ?*ScError = null;
    const rc = sc_infofile_get_key(f, "MISSING", &val, &err);
    try std.testing.expect(rc == 0);
    try std.testing.expect(err == null);
    try std.testing.expect(val == null);
}

test "sc_infofile_get_key null stream error" {
    var val: ?[*:0]u8 = null;
    var err: ?*ScError = null;
    const rc = sc_infofile_get_ini_section_key(null, null, "KEY", &val, &err);
    try std.testing.expect(rc == -1);
    try std.testing.expect(err != null);
    error_mod.sc_error_free(err);
}

test "sc_infofile_get_ini_section_key basic" {
    const content = "[mysection]\nKEY=value\n";
    const f = std.c.fmemopen(@constCast(content.ptr), content.len, "r") orelse
        return error.SkipZigTest;
    defer _ = std.c.fclose(f);
    var val: ?[*:0]u8 = null;
    var err: ?*ScError = null;
    const rc = sc_infofile_get_ini_section_key(f, "mysection", "KEY", &val, &err);
    try std.testing.expect(rc == 0);
    try std.testing.expect(err == null);
    try std.testing.expect(val != null);
    defer std.c.free(@ptrCast(val));
    try std.testing.expectEqualStrings("value", std.mem.sliceTo(val.?, 0));
}
