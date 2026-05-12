//! libc_extras.zig — extern declarations for libc functions not yet wrapped
//! by Zig 0.15's std.c, plus C types needed across modules.

const std = @import("std");

// ---------------------------------------------------------------------------
// Opaque C types
// ---------------------------------------------------------------------------

/// C va_list — use std.builtin.VaList (the type returned by @cVaStart).
/// This alias exists for documentation clarity.
pub const va_list = std.builtin.VaList;

/// POSIX regex types (opaque blobs; sizes from glibc on x86_64)
pub const regex_t = extern struct {
    buffer: usize = 0,
    allocated: usize = 0,
    used: usize = 0,
    syntax: c_ulong = 0,
    fastmap: ?*anyopaque = null,
    translate: ?*anyopaque = null,
    re_nsub: usize = 0,
    can_be_null: c_uint = 0,
    regs_allocated: c_uint = 0,
    fastmap_accurate: c_int = 0,
    no_sub: c_int = 0,
    not_bol: c_int = 0,
    not_eol: c_int = 0,
    newline_anchor: c_int = 0,
};

pub const regmatch_t = extern struct {
    rm_so: c_int,
    rm_eo: c_int,
};

pub const REG_EXTENDED: c_int = 1;
pub const REG_NOSUB: c_int = 8;
pub const REG_NEWLINE: c_int = 4;
pub const REG_ICASE: c_int = 2;

// ---------------------------------------------------------------------------
// Missing libc function declarations
// ---------------------------------------------------------------------------

pub extern "c" fn fprintf(stream: *std.c.FILE, fmt: [*:0]const u8, ...) c_int;
pub extern "c" fn vfprintf(stream: *std.c.FILE, fmt: [*:0]const u8, ap: *va_list) c_int;
pub extern "c" fn fgets(buf: [*]u8, size: c_int, stream: *std.c.FILE) ?[*]u8;
pub extern "c" fn fseek(stream: *std.c.FILE, offset: c_long, whence: c_int) c_int;
pub extern "c" fn fflush(stream: *std.c.FILE) c_int;
pub extern "c" fn strdup(s: [*:0]const u8) ?[*:0]u8;
pub extern "c" fn vasprintf(strp: *?[*]u8, fmt: [*:0]const u8, ap: *va_list) c_int;
pub extern "c" fn getline(lineptr: *?[*:0]u8, n: *usize, stream: *std.c.FILE) isize;
pub extern "c" fn strerror(errnum: c_int) [*:0]const u8;
pub extern "c" fn strtok_r(str: ?[*:0]u8, delim: [*:0]const u8, saveptr: *?[*:0]u8) ?[*:0]u8;
pub extern "c" fn sscanf(str: [*:0]const u8, fmt: [*:0]const u8, ...) c_int;
pub extern "c" fn sleep(seconds: c_uint) c_uint;
pub extern "c" fn getegid() std.c.gid_t;
pub extern "c" fn geteuid() std.c.uid_t;
pub extern "c" fn seteuid(uid: std.c.uid_t) c_int;
pub extern "c" fn fchownat(dirfd: std.c.fd_t, pathname: [*:0]const u8, owner: std.c.uid_t, group: std.c.gid_t, flags: c_int) c_int;
pub extern "c" fn fstatat(dirfd: std.c.fd_t, pathname: [*:0]const u8, statbuf: *std.c.Stat, flags: c_int) c_int;
pub extern "c" fn regcomp(preg: *regex_t, pattern: [*:0]const u8, cflags: c_int) c_int;
pub extern "c" fn regexec(preg: *const regex_t, string: [*:0]const u8, nmatch: usize, pmatch: ?[*]regmatch_t, eflags: c_int) c_int;
pub extern "c" fn regfree(preg: *regex_t) void;

pub const SEEK_SET: c_int = 0;

/// stderr FILE* — declared as extern since std.c does not expose it in Zig 0.15
pub extern var stderr: *std.c.FILE;

// Open flags helpers — construct std.c.O values.
pub fn o_flags(comptime opts: struct {
    CLOEXEC: bool = false,
    DIRECTORY: bool = false,
    NOFOLLOW: bool = false,
    PATH: bool = false,
    RDONLY: bool = false,
    WRONLY: bool = false,
    RDWR: bool = false,
    CREAT: bool = false,
    TRUNC: bool = false,
}) std.c.O {
    return .{
        .CLOEXEC = opts.CLOEXEC,
        .DIRECTORY = opts.DIRECTORY,
        .NOFOLLOW = opts.NOFOLLOW,
        .PATH = opts.PATH,
        .CREAT = opts.CREAT,
        .TRUNC = opts.TRUNC,
    };
}

// AT flag helpers
pub const AT_SYMLINK_NOFOLLOW: c_int = std.c.AT.SYMLINK_NOFOLLOW;
pub const AT_FDCWD: c_int = std.c.AT.FDCWD;
