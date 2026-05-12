//! classic.zig — port of classic.c / classic.h

const std = @import("std");
const C = @import("libc_extras.zig");
const error_mod = @import("error.zig");
const infofile = @import("infofile.zig");
const string_utils = @import("string_utils.zig");

pub const SC_HOSTFS_DIR = "/var/lib/snapd/hostfs";

pub const SC_DISTRO_CORE16: c_int = 0;
pub const SC_DISTRO_CORE_OTHER: c_int = 1;
pub const SC_DISTRO_CLASSIC: c_int = 2;

const os_release = "/etc/os-release";
const meta_snap_yaml = "/meta/snap.yaml";

export fn sc_classify_distro() c_int {
    const f = std.c.fopen(os_release, "r") orelse return SC_DISTRO_CLASSIC;
    defer _ = std.c.fclose(f);

    var is_core = false;
    var core_version: c_int = 0;
    var buf: [255]u8 = std.mem.zeroes([255]u8);

    while (C.fgets(&buf, @intCast(buf.len), f) != null) {
        const line = std.mem.sliceTo(&buf, 0);
        if (line.len > 0 and line[line.len - 1] == '\n') buf[line.len - 1] = 0;
        const s = std.mem.sliceTo(&buf, 0);

        if (std.mem.eql(u8, s, "ID=\"ubuntu-core\"") or std.mem.eql(u8, s, "ID=ubuntu-core")) {
            is_core = true;
        } else if (std.mem.eql(u8, s, "VERSION_ID=\"16\"") or std.mem.eql(u8, s, "VERSION_ID=16")) {
            core_version = 16;
        } else if (std.mem.eql(u8, s, "VARIANT_ID=\"snappy\"") or std.mem.eql(u8, s, "VARIANT_ID=snappy")) {
            is_core = true;
        }
    }

    if (!is_core) {
        if (std.c.access(meta_snap_yaml, @intCast(std.c.F_OK)) == 0) is_core = true;
    }

    if (is_core) {
        return if (core_version == 16) SC_DISTRO_CORE16 else SC_DISTRO_CORE_OTHER;
    } else {
        return SC_DISTRO_CLASSIC;
    }
}

export fn sc_is_debian_like() bool {
    const f = std.c.fopen(os_release, "r") orelse return false;
    defer _ = std.c.fclose(f);

    const keys = [_][*:0]const u8{ "ID", "ID_LIKE" };
    for (keys) |key| {
        if (C.fseek(f, 0, C.SEEK_SET) != 0) return false;
        var id_val: ?[*:0]u8 = null;
        var err: ?*error_mod.ScError = null;
        const rc = infofile.sc_infofile_get_key(f, key, &id_val, &err);
        if (rc != 0) {
            error_mod.sc_error_free(err);
            continue;
        }
        if (id_val) |v| {
            defer std.c.free(@ptrCast(v));
            const vs = std.mem.sliceTo(v, 0);
            if (std.mem.eql(u8, vs, "\"debian\"") or std.mem.eql(u8, vs, "debian")) return true;
        }
    }
    return false;
}

test "sc_classify_distro returns an in-range value" {
    const d = sc_classify_distro();
    try std.testing.expect(d == SC_DISTRO_CORE16 or d == SC_DISTRO_CORE_OTHER or d == SC_DISTRO_CLASSIC);
}

test "sc_is_debian_like does not crash" {
    _ = sc_is_debian_like();
}
