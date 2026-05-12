//! feature.zig — port of feature.c / feature.h

const std = @import("std");
const C = @import("libc_extras.zig");
const panic_mod = @import("panic.zig");

pub const SC_FEATURE_PER_USER_MOUNT_NAMESPACE: c_int = 1 << 0;
pub const SC_FEATURE_REFRESH_APP_AWARENESS: c_int = 1 << 1;
pub const SC_FEATURE_PARALLEL_INSTANCES: c_int = 1 << 2;
pub const SC_FEATURE_HIDDEN_SNAP_FOLDER: c_int = 1 << 3;

const feature_flag_dir = "/var/lib/snapd/features";

export fn sc_feature_enabled(flag: c_int) bool {
    const file_name: [*:0]const u8 = switch (flag) {
        SC_FEATURE_PER_USER_MOUNT_NAMESPACE => "per-user-mount-namespace",
        SC_FEATURE_REFRESH_APP_AWARENESS => "refresh-app-awareness",
        SC_FEATURE_PARALLEL_INSTANCES => "parallel-instances",
        SC_FEATURE_HIDDEN_SNAP_FOLDER => "hidden-snap-folder",
        else => panic_mod.die("unknown feature flag code {d}", .{flag}),
    };

    const dirfd = std.c.open(feature_flag_dir, .{ .CLOEXEC = true, .DIRECTORY = true, .NOFOLLOW = true, .PATH = true });
    if (dirfd < 0) {
        if (std.c._errno().* == @intFromEnum(std.c.E.NOENT)) return false;
        panic_mod.die("cannot open path {s}", .{feature_flag_dir});
    }
    defer _ = std.c.close(@intCast(dirfd));

    var st: std.c.Stat = undefined;
    if (C.fstatat(dirfd, file_name, &st, C.AT_SYMLINK_NOFOLLOW) < 0) {
        if (std.c._errno().* == @intFromEnum(std.c.E.NOENT)) return false;
        panic_mod.die("cannot inspect file {s}/{s}", .{ feature_flag_dir, file_name });
    }
    return (st.mode & std.c.S.IFMT) == std.c.S.IFREG;
}

test "sc_feature_enabled returns false for nonexistent dir" {
    _ = sc_feature_enabled(SC_FEATURE_PARALLEL_INSTANCES);
}
