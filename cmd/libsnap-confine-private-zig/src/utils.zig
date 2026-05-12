//! utils.zig — port of utils.c / utils.h

const std = @import("std");
const C = @import("libc_extras.zig");
const panic_mod = @import("panic.zig");

// ---------------------------------------------------------------------------
// sc_identity struct
// ---------------------------------------------------------------------------

pub const ScIdentity = extern struct {
    uid: std.c.uid_t,
    gid: std.c.gid_t,
    flags: u8, // bit 0 = change_uid, bit 1 = change_gid
};

// ---------------------------------------------------------------------------
// C-ABI exports
// ---------------------------------------------------------------------------

export fn die(fmt: [*:0]const u8, ...) noreturn {
    var ap = @cVaStart();
    defer @cVaEnd(&ap);
    _ = C.vfprintf(C.stderr, fmt, &ap);
    _ = C.fprintf(C.stderr, "\n");
    std.c.exit(1);
}

fn getenv_bool_inner(name: [*:0]const u8, default_value: bool) bool {
    const str_value = std.c.getenv(name) orelse return default_value;
    const s = std.mem.sliceTo(str_value, 0);
    if (std.mem.eql(u8, s, "yes") or std.mem.eql(u8, s, "1")) return true;
    if (std.mem.eql(u8, s, "no") or std.mem.eql(u8, s, "0") or std.mem.eql(u8, s, "")) return false;
    _ = C.fprintf(
        C.stderr,
        "WARNING: unrecognized value of environment variable %s (expected yes/no or 1/0)\n",
        name,
    );
    return false;
}

pub fn sc_is_debug_enabled_inner() bool {
    return getenv_bool_inner("SNAP_CONFINE_DEBUG", false) or
        getenv_bool_inner("SNAPD_DEBUG", false);
}

export fn getenv_bool(name: [*:0]const u8, default_value: bool) bool {
    return getenv_bool_inner(name, default_value);
}

export fn sc_is_debug_enabled() bool {
    return sc_is_debug_enabled_inner();
}

export fn sc_is_reexec_enabled() bool {
    return getenv_bool_inner("SNAP_REEXEC", true);
}

export fn debug(fmt: [*:0]const u8, ...) void {
    if (!sc_is_debug_enabled_inner()) return;
    var ap = @cVaStart();
    defer @cVaEnd(&ap);
    _ = C.fprintf(C.stderr, "DEBUG: ");
    _ = C.vfprintf(C.stderr, fmt, &ap);
    _ = C.fprintf(C.stderr, "\n");
}

export fn write_string_to_file(filepath: [*:0]const u8, buf: [*:0]const u8) void {
    const f = std.c.fopen(filepath, "w") orelse
        panic_mod.die("fopen {s} failed", .{std.mem.sliceTo(filepath, 0)});
    const len = std.mem.len(buf);
    if (std.c.fwrite(buf, 1, len, f) != len)
        panic_mod.die("fwrite failed", .{});
    if (C.fflush(f) != 0)
        panic_mod.die("fflush failed", .{});
    if (std.c.fclose(f) != 0)
        panic_mod.die("fclose failed", .{});
}

export fn sc_set_effective_identity(identity: ScIdentity) ScIdentity {
    var old = ScIdentity{ .uid = 0, .gid = 0, .flags = 0 };
    const change_gid = (identity.flags & 0b10) != 0;
    const change_uid = (identity.flags & 0b01) != 0;
    if (change_gid) {
        old.gid = @intCast(C.getegid());
        old.flags |= 0b10;
        if (std.c.setegid(identity.gid) < 0)
            panic_mod.die("cannot set effective group to {d}", .{identity.gid});
        if (C.getegid() != identity.gid)
            panic_mod.die("effective group change failed", .{});
    }
    if (change_uid) {
        old.uid = @intCast(C.geteuid());
        old.flags |= 0b01;
        if (C.seteuid(identity.uid) < 0)
            panic_mod.die("cannot set effective user to {d}", .{identity.uid});
        if (C.geteuid() != identity.uid)
            panic_mod.die("effective user change failed", .{});
    }
    return old;
}

export fn sc_wait_for_file(path: [*:0]const u8, timeout_sec: usize) bool {
    var i: usize = 0;
    while (i < timeout_sec) : (i += 1) {
        if (std.c.access(path, @intCast(std.c.F_OK)) == 0) return true;
        _ = C.sleep(1);
    }
    return false;
}

export fn sc_ensure_mkdirat(fd: c_int, name: [*:0]const u8, mode: std.c.mode_t, uid: std.c.uid_t, gid: std.c.gid_t) c_int {
    if (std.c.mkdirat(@intCast(fd), name, 0o000) < 0) {
        if (std.c._errno().* != @intFromEnum(std.c.E.EXIST)) return -1;
    } else {
        if (C.fchownat(fd, name, uid, gid, C.AT_SYMLINK_NOFOLLOW) < 0) return -1;
        if (std.c.fchmodat(@intCast(fd), name, mode, @intCast(C.AT_SYMLINK_NOFOLLOW)) < 0) {
            const e = std.c._errno().*;
            if (e == @intFromEnum(std.c.E.OPNOTSUPP) or e == @intFromEnum(std.c.E.NOSYS)) {
                std.c._errno().* = 0;
                if (std.c.fchmodat(@intCast(fd), name, mode, 0) < 0) return -1;
            } else return -1;
        }
        std.c._errno().* = 0;
    }
    return 0;
}

export fn sc_ensure_mkdir(path: [*:0]const u8, mode: std.c.mode_t, uid: std.c.uid_t, gid: std.c.gid_t) c_int {
    return sc_ensure_mkdirat(std.c.AT.FDCWD, path, mode, uid, gid);
}

export fn sc_nonfatal_mkpath(path: [*:0]const u8, mode: std.c.mode_t, uid: std.c.uid_t, gid: std.c.gid_t) c_int {
    const path_slice = std.mem.sliceTo(path, 0);
    if (path_slice.len == 0) return 0;

    const path_copy = C.strdup(path) orelse return -1;
    defer std.c.free(@ptrCast(path_copy));

    const open_flags = std.c.O{ .CLOEXEC = true, .DIRECTORY = true, .NOFOLLOW = true };

    var fd: c_int = std.c.AT.FDCWD;
    if (path_copy[0] == '/') {
        fd = std.c.open("/", open_flags);
        if (fd < 0) return -1;
    }

    var walker: ?[*:0]u8 = null;
    var segment = C.strtok_r(path_copy, "/", &walker);
    while (segment != null) : (segment = C.strtok_r(null, "/", &walker)) {
        std.c._errno().* = 0;
        if (sc_ensure_mkdirat(fd, segment.?, mode, uid, gid) != 0) {
            if (fd != std.c.AT.FDCWD) _ = std.c.close(@intCast(fd));
            return -1;
        }
        const prev = fd;
        fd = std.c.openat(@intCast(fd), segment.?, open_flags);
        if (prev != std.c.AT.FDCWD and std.c.close(@intCast(prev)) != 0) {
            if (fd >= 0) _ = std.c.close(@intCast(fd));
            return -1;
        }
        if (fd < 0) return -1;
    }
    if (fd != std.c.AT.FDCWD) _ = std.c.close(@intCast(fd));
    return 0;
}

export fn sc_is_expected_path(path: [*:0]const u8) bool {
    const pattern =
        "^((/var/lib/snapd)?/snap/(snapd|core)/x?[0-9]+/usr/lib|/usr/lib(exec)?)/snapd/snap-confine$";
    var re: C.regex_t = undefined;
    if (C.regcomp(&re, pattern, C.REG_EXTENDED | C.REG_NOSUB) != 0)
        panic_mod.die("can not compile regex {s}", .{pattern});
    const status = C.regexec(&re, path, 0, null, 0);
    C.regfree(&re);
    return status == 0;
}

export fn sc_is_in_container() bool {
    return _sc_is_in_container("/run/systemd/container");
}

fn _sc_is_in_container(p: [*:0]const u8) bool {
    const f = std.c.fopen(p, "r") orelse return false;
    defer _ = std.c.fclose(f);
    var container: [128]u8 = std.mem.zeroes([128]u8);
    if (C.fgets(&container, @intCast(container.len), f) == null) return false;
    // chomp trailing newline
    const s = std.mem.sliceTo(&container, 0);
    if (s.len > 0 and s[s.len - 1] == '\n') container[s.len - 1] = 0;
    return container[0] != 0;
}

export fn sc_is_path_canonical(path: ?[*:0]const u8) bool {
    const p = path orelse return false;
    const s = std.mem.sliceTo(p, 0);
    if (s.len == 0) return false;
    if (s[0] != '/') return false;
    if (s.len == 1) return true;
    if (std.mem.endsWith(u8, s, "/.")) return false;
    if (std.mem.endsWith(u8, s, "/..")) return false;
    if (std.mem.endsWith(u8, s, "/")) return false;
    if (std.mem.indexOf(u8, s, "/../") != null) return false;
    if (std.mem.indexOf(u8, s, "/./") != null) return false;
    if (std.mem.indexOf(u8, s, "//") != null) return false;
    return true;
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

test "sc_is_path_canonical valid paths" {
    try std.testing.expect(sc_is_path_canonical("/"));
    try std.testing.expect(sc_is_path_canonical("/foo/bar"));
}

test "sc_is_path_canonical invalid paths" {
    try std.testing.expect(!sc_is_path_canonical(null));
    try std.testing.expect(!sc_is_path_canonical("foo/bar"));
    try std.testing.expect(!sc_is_path_canonical("/foo/bar/"));
    try std.testing.expect(!sc_is_path_canonical("/foo/./bar"));
    try std.testing.expect(!sc_is_path_canonical("/foo/../bar"));
    try std.testing.expect(!sc_is_path_canonical("/foo//bar"));
    try std.testing.expect(!sc_is_path_canonical("/foo/."));
    try std.testing.expect(!sc_is_path_canonical("/foo/.."));
}

test "sc_is_expected_path" {
    try std.testing.expect(sc_is_expected_path("/snap/core/1234/usr/lib/snapd/snap-confine"));
    try std.testing.expect(sc_is_expected_path("/usr/lib/snapd/snap-confine"));
    try std.testing.expect(!sc_is_expected_path("/usr/bin/snap-confine"));
}

test "getenv_bool default fallback" {
    try std.testing.expect(getenv_bool_inner("_SC_TEST_UNSET_ZZZ", true) == true);
    try std.testing.expect(getenv_bool_inner("_SC_TEST_UNSET_ZZZ", false) == false);
}
