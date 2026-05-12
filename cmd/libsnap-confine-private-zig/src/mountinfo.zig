//! mountinfo.zig — port of mountinfo.c / mountinfo.h
//!
//! Parses /proc/self/mountinfo (or an arbitrary file) into a linked list of
//! sc_mountinfo_entry structs.  The C implementation uses a flexible array
//! member (char line_buf[0]) at the end of the entry struct so that the line
//! buffer is allocated together with the struct.  We reproduce this in Zig by
//! allocating `@sizeOf(ScMountinfoEntry) + line.len + 1` bytes, casting the
//! pointer, and treating the bytes immediately after the struct as line_buf.

const std = @import("std");
const C = @import("libc_extras.zig");
const panic_mod = @import("panic.zig");

// ---------------------------------------------------------------------------
// Public types (C ABI)
// ---------------------------------------------------------------------------

/// Mirrors sc_mountinfo_entry in mountinfo.h.
/// The flexible array member `line_buf` is NOT included here; callers access
/// it via line_buf_ptr() below.
pub const ScMountinfoEntry = extern struct {
    mount_id: c_int,
    parent_id: c_int,
    dev_major: c_uint,
    dev_minor: c_uint,
    root: ?[*:0]u8,
    mount_dir: ?[*:0]u8,
    mount_opts: ?[*:0]u8,
    optional_fields: ?[*:0]u8,
    fs_type: ?[*:0]u8,
    mount_source: ?[*:0]u8,
    super_opts: ?[*:0]u8,
    next: ?*ScMountinfoEntry,
    // line_buf follows here in memory (flexible array member)
};

pub const ScMountinfo = extern struct {
    first: ?*ScMountinfoEntry,
};

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

/// Return a pointer to the line_buf that lives immediately after *entry.
fn line_buf_ptr(entry: *ScMountinfoEntry) [*]u8 {
    const base: [*]u8 = @ptrCast(entry);
    return base + @sizeOf(ScMountinfoEntry);
}

const ParseState = struct {
    entry: *ScMountinfoEntry,
    line: []const u8,
    offset: usize,
};

fn is_octal_digit(c: u8) bool {
    return c >= '0' and c <= '7';
}

/// Parse the next space-separated (or NUL-terminated) field from ps.line
/// into ps.entry's line_buf, starting at ps.offset.  Updates ps.offset.
/// Returns a sentinel-terminated pointer into line_buf, or null on EOF.
fn parse_next_field_ex(ps: *ParseState, allow_spaces: bool) ?[*:0]u8 {
    const lbuf = line_buf_ptr(ps.entry);
    const input = ps.line;
    var ii: usize = ps.offset; // input index
    var oi: usize = ps.offset; // output index into lbuf
    const start_oi = oi;

    while (true) {
        if (ii >= input.len or input[ii] == 0) {
            if (oi == start_oi) return null; // nothing scanned
            lbuf[oi] = 0;
            ps.offset = ii;
            break;
        }
        const c = input[ii];
        if (c == ' ' and !allow_spaces) {
            lbuf[oi] = 0;
            ii += 1;
            ps.offset = ii;
            break;
        }
        if (c == '\\' and ii + 4 <= input.len and
            is_octal_digit(input[ii + 1]) and
            is_octal_digit(input[ii + 2]) and
            is_octal_digit(input[ii + 3]))
        {
            lbuf[oi] = ((input[ii + 1] - '0') << 6) |
                ((input[ii + 2] - '0') << 3) |
                (input[ii + 3] - '0');
            oi += 1;
            ii += 4;
        } else {
            lbuf[oi] = c;
            oi += 1;
            ii += 1;
        }
    }
    // Return pointer into lbuf at start_oi, as a sentinel-terminated slice.
    return @ptrCast(lbuf + start_oi);
}

fn parse_next_field(ps: *ParseState) ?[*:0]u8 {
    return parse_next_field_ex(ps, false);
}

fn parse_last_field(ps: *ParseState) ?[*:0]u8 {
    return parse_next_field_ex(ps, true);
}

/// Parse a single mountinfo line.  Returns a heap-allocated entry or null.
fn parse_entry(line: []const u8) ?*ScMountinfoEntry {
    // Allocate struct + extra bytes for line_buf
    const alloc_size = @sizeOf(ScMountinfoEntry) + line.len + 1;
    const raw = std.c.calloc(1, alloc_size) orelse return null;
    const entry: *ScMountinfoEntry = @ptrCast(@alignCast(raw));

    // Parse fixed integer fields with sscanf-like scanning.
    var initial_offset: c_int = 0;
    const n = C.sscanf(
        @as([*:0]const u8, @ptrCast(line.ptr)),
        "%d %d %u:%u %n",
        &entry.mount_id,
        &entry.parent_id,
        &entry.dev_major,
        &entry.dev_minor,
        &initial_offset,
    );
    if (n != 4) {
        std.c.free(raw);
        return null;
    }
    var ps = ParseState{
        .entry = entry,
        .line = line,
        .offset = @intCast(initial_offset),
    };

    entry.root = parse_next_field(&ps) orelse { std.c.free(raw); return null; };
    entry.mount_dir = parse_next_field(&ps) orelse { std.c.free(raw); return null; };
    entry.mount_opts = parse_next_field(&ps) orelse { std.c.free(raw); return null; };

    // optional_fields: collect until "-" separator; fields are space-joined.
    const lbuf = line_buf_ptr(entry);
    entry.optional_fields = @ptrCast(lbuf + ps.offset);
    var field_num: usize = 0;
    while (true) {
        const opt = parse_next_field(&ps) orelse {
            std.c.free(raw);
            return null;
        };
        const opt_slice = std.mem.sliceTo(opt, 0);
        if (std.mem.eql(u8, opt_slice, "-")) {
            // Overwrite the "-" with NUL to terminate optional_fields.
            opt[0] = 0;
            break;
        }
        if (field_num > 0) {
            // Re-join with space (the NUL between fields).
            opt[@as(usize, @intCast(@intFromPtr(opt) - @intFromPtr(entry.optional_fields.?) - 1))] = ' ';
        }
        field_num += 1;
    }

    entry.fs_type = parse_next_field(&ps) orelse { std.c.free(raw); return null; };
    entry.mount_source = parse_next_field(&ps) orelse { std.c.free(raw); return null; };
    entry.super_opts = parse_last_field(&ps) orelse { std.c.free(raw); return null; };
    return entry;
}

fn free_mountinfo(info: *ScMountinfo) void {
    var cur = info.first;
    while (cur) |e| {
        const nxt = e.next;
        std.c.free(@ptrCast(e));
        cur = nxt;
    }
    std.c.free(@ptrCast(info));
}

// ---------------------------------------------------------------------------
// Public C-ABI exports
// ---------------------------------------------------------------------------

pub export fn sc_parse_mountinfo(fname_opt: ?[*:0]const u8) ?*ScMountinfo {
    const info_raw = std.c.calloc(1, @sizeOf(ScMountinfo)) orelse return null;
    const info: *ScMountinfo = @ptrCast(@alignCast(info_raw));

    const fname: [*:0]const u8 = fname_opt orelse "/proc/self/mountinfo";
    const f = std.c.fopen(fname, "rt") orelse {
        std.c.free(info_raw);
        return null;
    };
    defer _ = std.c.fclose(f);

    var line_buf: ?[*:0]u8 = null;
    var line_size: usize = 0;
    defer if (line_buf) |lb| std.c.free(@ptrCast(lb));

    var last: ?*ScMountinfoEntry = null;
    while (true) {
        std.c._errno().* = 0;
        const rc = C.getline(&line_buf, &line_size, f);
        if (rc == -1) {
            if (std.c._errno().* != 0) {
                free_mountinfo(info);
                return null;
            }
            break;
        }
        const line_slice = std.mem.sliceTo(line_buf.?, 0);
        const entry = parse_entry(line_slice) orelse {
            free_mountinfo(info);
            return null;
        };
        if (last) |l| {
            l.next = entry;
        } else {
            info.first = entry;
        }
        last = entry;
    }
    return info;
}

pub export fn sc_cleanup_mountinfo(ptr: *?*ScMountinfo) void {
    if (ptr.*) |info| {
        free_mountinfo(info);
        ptr.* = null;
    }
}

pub export fn sc_first_mountinfo_entry(info: *ScMountinfo) ?*ScMountinfoEntry {
    return info.first;
}

pub export fn sc_next_mountinfo_entry(entry: *ScMountinfoEntry) ?*ScMountinfoEntry {
    return entry.next;
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

test "parse simple mountinfo line" {
    const line = "22 1 8:1 / / rw,relatime - ext4 /dev/sda1 rw,errors=remount-ro";
    const entry = parse_entry(line) orelse {
        try std.testing.expect(false);
        return;
    };
    defer std.c.free(@ptrCast(entry));

    try std.testing.expectEqual(@as(c_int, 22), entry.mount_id);
    try std.testing.expectEqual(@as(c_int, 1), entry.parent_id);
    try std.testing.expectEqual(@as(c_uint, 8), entry.dev_major);
    try std.testing.expectEqual(@as(c_uint, 1), entry.dev_minor);
    try std.testing.expectEqualStrings("/", std.mem.sliceTo(entry.root.?, 0));
    try std.testing.expectEqualStrings("/", std.mem.sliceTo(entry.mount_dir.?, 0));
    try std.testing.expectEqualStrings("rw,relatime", std.mem.sliceTo(entry.mount_opts.?, 0));
    try std.testing.expectEqualStrings("", std.mem.sliceTo(entry.optional_fields.?, 0));
    try std.testing.expectEqualStrings("ext4", std.mem.sliceTo(entry.fs_type.?, 0));
    try std.testing.expectEqualStrings("/dev/sda1", std.mem.sliceTo(entry.mount_source.?, 0));
    try std.testing.expectEqualStrings("rw,errors=remount-ro", std.mem.sliceTo(entry.super_opts.?, 0));
}

test "parse mountinfo line with optional fields" {
    const line = "36 35 98:0 /mnt1 /mnt2 rw,noatime master:1 - ext3 /dev/root rw,errors=continue";
    const entry = parse_entry(line) orelse {
        try std.testing.expect(false);
        return;
    };
    defer std.c.free(@ptrCast(entry));

    try std.testing.expectEqual(@as(c_int, 36), entry.mount_id);
    try std.testing.expectEqualStrings("master:1", std.mem.sliceTo(entry.optional_fields.?, 0));
    try std.testing.expectEqualStrings("ext3", std.mem.sliceTo(entry.fs_type.?, 0));
}

test "parse mountinfo line with octal escape in path" {
    // Kernel encodes space as \040
    const line = "100 1 0:1 / /mnt\\040dir rw - tmpfs tmpfs rw";
    const entry = parse_entry(line) orelse {
        try std.testing.expect(false);
        return;
    };
    defer std.c.free(@ptrCast(entry));
    try std.testing.expectEqualStrings("/mnt dir", std.mem.sliceTo(entry.mount_dir.?, 0));
}

test "sc_first and sc_next mountinfo entry" {
    // Write a tiny two-line mountinfo to a temp file and parse it.
    const path = "/tmp/test-mountinfo-zig.txt";
    const content =
        "1 0 8:1 / / rw - ext4 /dev/sda1 rw\n" ++
        "2 1 8:2 / /boot rw - ext4 /dev/sda2 rw\n";
    {
        const f = std.c.fopen(path, "w") orelse return error.SkipZigTest;
        _ = std.c.fwrite(content.ptr, 1, content.len, f);
        _ = std.c.fclose(f);
    }
    defer _ = std.c.unlink(path);

    const info = sc_parse_mountinfo(path) orelse {
        try std.testing.expect(false);
        return;
    };
    defer {
        var p: ?*ScMountinfo = info;
        sc_cleanup_mountinfo(&p);
    }

    const e1 = sc_first_mountinfo_entry(info);
    try std.testing.expect(e1 != null);
    try std.testing.expectEqual(@as(c_int, 1), e1.?.mount_id);

    const e2 = sc_next_mountinfo_entry(e1.?);
    try std.testing.expect(e2 != null);
    try std.testing.expectEqual(@as(c_int, 2), e2.?.mount_id);

    try std.testing.expect(sc_next_mountinfo_entry(e2.?) == null);
}
