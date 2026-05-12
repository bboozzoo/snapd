// string_utils.zig — safe string primitives
// Mirrors libsnap-confine-private/string-utils.c
//
// All functions that mirror the C API use `export` for C linkage so they are
// callable from the static/shared library by C consumers and by bun:ffi.

const std = @import("std");

fn die(comptime fmt: []const u8, args: anytype) noreturn {
    std.debug.print(fmt ++ "\n", args);
    std.process.exit(1);
}

/// Check if two C strings are equal. Returns false if either is NULL.
export fn sc_streq(a: ?[*:0]const u8, b: ?[*:0]const u8) bool {
    if (a == null or b == null) return false;
    return std.mem.orderZ(u8, a.?, b.?) == .eq;
}

/// Check if a C string has the given suffix. Returns false if either is NULL.
export fn sc_endswith(str: ?[*:0]const u8, suffix: ?[*:0]const u8) bool {
    if (str == null or suffix == null) return false;
    const s = std.mem.span(str.?);
    const x = std.mem.span(suffix.?);
    return std.mem.endsWith(u8, s, x);
}

/// Check if a C string has the given prefix. Returns false if either is NULL.
export fn sc_startswith(str: ?[*:0]const u8, prefix: ?[*:0]const u8) bool {
    if (str == null or prefix == null) return false;
    const s = std.mem.span(str.?);
    const x = std.mem.span(prefix.?);
    return std.mem.startsWith(u8, s, x);
}

/// Duplicate a C string using the C allocator. Dies on NULL input or OOM.
/// Caller must free() the result.
export fn sc_strdup(str: ?[*:0]const u8) ?[*:0]u8 {
    if (str == null) die("cannot duplicate NULL string", .{});
    const s = std.mem.span(str.?);
    const copy = std.heap.c_allocator.allocSentinel(u8, s.len, 0) catch
        die("cannot allocate string copy (len: {d})", .{s.len});
    @memcpy(copy, s);
    return copy.ptr;
}

/// Append src to the NUL-terminated string in dst[0..dst_size]. Dies on overflow.
/// Returns new length of the string in dst.
export fn sc_string_append(dst: ?[*]u8, dst_size: usize, str: ?[*:0]const u8) usize {
    if (dst == null) die("cannot append string: buffer is NULL", .{});
    if (str == null) die("cannot append string: string is NULL", .{});
    const buf = dst.?[0..dst_size];
    const dst_len = std.mem.indexOfScalar(u8, buf, 0) orelse
        die("cannot append string: dst is unterminated", .{});
    const src = std.mem.span(str.?);
    const max_str_len = dst_size - dst_len;
    if (src.len >= max_str_len)
        die("cannot append string: str is too long or unterminated", .{});
    @memcpy(buf[dst_len .. dst_len + src.len], src);
    buf[dst_len + src.len] = 0;
    return dst_len + src.len;
}

/// Append a single character to the NUL-terminated string in dst[0..dst_size].
/// The character must not be NUL. Dies on overflow or NUL character.
/// Returns new length.
export fn sc_string_append_char(dst: ?[*]u8, dst_size: usize, ch: u8) usize {
    if (dst == null) die("cannot append character: buffer is NULL", .{});
    if (ch == 0) die("cannot append character: cannot append string terminator", .{});
    const buf = dst.?[0..dst_size];
    const dst_len = std.mem.indexOfScalar(u8, buf, 0) orelse
        die("cannot append character: dst is unterminated", .{});
    if (dst_size - dst_len < 2)
        die("cannot append character: not enough space", .{});
    buf[dst_len] = ch;
    buf[dst_len + 1] = 0;
    return dst_len + 1;
}

/// Append two characters to the NUL-terminated string in dst[0..dst_size].
/// Neither character may be NUL. Dies on overflow.
/// Returns new length.
export fn sc_string_append_char_pair(dst: ?[*]u8, dst_size: usize, c1: u8, c2: u8) usize {
    if (dst == null) die("cannot append character pair: buffer is NULL", .{});
    if (c1 == 0 or c2 == 0)
        die("cannot append character pair: cannot append string terminator", .{});
    const buf = dst.?[0..dst_size];
    const dst_len = std.mem.indexOfScalar(u8, buf, 0) orelse
        die("cannot append character pair: dst is unterminated", .{});
    if (dst_size - dst_len < 3)
        die("cannot append character pair: not enough space", .{});
    buf[dst_len] = c1;
    buf[dst_len + 1] = c2;
    buf[dst_len + 2] = 0;
    return dst_len + 2;
}

/// Initialize a buffer as an empty string. Dies if buf is NULL or buf_size is 0.
export fn sc_string_init(buf: ?[*]u8, buf_size: usize) void {
    if (buf == null) die("cannot initialize string, buffer is NULL", .{});
    if (buf_size == 0) die("cannot initialize string, buffer is too small", .{});
    buf.?[0] = 0;
}

/// Quote a string safely for printing, writing into buf[0..buf_size].
/// All non-printable and special characters are hex-escaped.
export fn sc_string_quote(buf: ?[*]u8, buf_size: usize, str: ?[*:0]const u8) void {
    if (str == null) die("cannot quote string: string is NULL", .{});
    const hex = "0123456789abcdef";
    sc_string_init(buf, buf_size);
    _ = sc_string_append_char(buf, buf_size, '"');
    const s = std.mem.span(str.?);
    for (s) |ch| {
        switch (ch) {
            '0'...'9', 'A'...'Z', 'a'...'z',
            ' ', '!', '#', '$', '%', '&', '(', ')', '*', '+', ',', '-',
            '.', '/', ':', ';', '<', '=', '>', '?', '@', '[', '\'', ']',
            '^', '_', '`', '{', '|', '}', '~',
            => _ = sc_string_append_char(buf, buf_size, ch),
            '\n' => _ = sc_string_append_char_pair(buf, buf_size, '\\', 'n'),
            '\r' => _ = sc_string_append_char_pair(buf, buf_size, '\\', 'r'),
            '\t' => _ = sc_string_append_char_pair(buf, buf_size, '\\', 't'),
            11   => _ = sc_string_append_char_pair(buf, buf_size, '\\', 'v'), // \v
            '\\' => _ = sc_string_append_char_pair(buf, buf_size, '\\', '\\'),
            '"'  => _ = sc_string_append_char_pair(buf, buf_size, '\\', '"'),
            else => {
                _ = sc_string_append_char_pair(buf, buf_size, '\\', 'x');
                _ = sc_string_append_char_pair(buf, buf_size,
                    hex[ch >> 4], hex[ch & 15]);
            },
        }
    }
    _ = sc_string_append_char(buf, buf_size, '"');
}

/// Split string on first occurrence of delimiter, writing prefix and suffix
/// into the provided buffers. Either buffer may be NULL to skip that part.
pub export fn sc_string_split(
    string: ?[*:0]const u8,
    delimiter: u8,
    prefix_buf: ?[*]u8,
    prefix_size: usize,
    suffix_buf: ?[*]u8,
    suffix_size: usize,
) void {
    if (string == null)
        die("internal error: cannot split string when it is unset", .{});
    if (prefix_buf == null and suffix_buf == null)
        die("internal error: cannot split string when both prefix and suffix are unset", .{});
    const s = std.mem.span(string.?);
    const pos = std.mem.indexOfScalar(u8, s, delimiter);
    const prefix_slice = if (pos) |p| s[0..p] else s;
    const suffix_slice = if (pos) |p| s[p + 1 ..] else "";

    if (prefix_buf) |pb| {
        if (prefix_slice.len >= prefix_size)
            die("prefix buffer too small", .{});
        @memcpy(pb[0..prefix_slice.len], prefix_slice);
        pb[prefix_slice.len] = 0;
    }
    if (suffix_buf) |sb| {
        if (suffix_slice.len >= suffix_size)
            die("suffix buffer too small", .{});
        @memcpy(sb[0..suffix_slice.len], suffix_slice);
        sb[suffix_slice.len] = 0;
    }
}

/// Remove trailing newlines from string in-place. Returns the string.
export fn sc_str_chomp(string: ?[*:0]u8) ?[*:0]u8 {
    if (string == null) return null;
    const s = std.mem.span(string.?);
    var len = s.len;
    while (len > 0 and s[len - 1] == '\n') : (len -= 1) {}
    string.?[len] = 0;
    return string;
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

test "sc_streq: equal strings" {
    try std.testing.expect(sc_streq("hello", "hello"));
}

test "sc_streq: different strings" {
    try std.testing.expect(!sc_streq("hello", "world"));
}

test "sc_streq: null a" {
    try std.testing.expect(!sc_streq(null, "hello"));
}

test "sc_streq: null b" {
    try std.testing.expect(!sc_streq("hello", null));
}

test "sc_streq: both null" {
    try std.testing.expect(!sc_streq(null, null));
}

test "sc_streq: empty strings" {
    try std.testing.expect(sc_streq("", ""));
}

test "sc_startswith: matching prefix" {
    try std.testing.expect(sc_startswith("foobar", "foo"));
}

test "sc_startswith: non-matching prefix" {
    try std.testing.expect(!sc_startswith("foobar", "baz"));
}

test "sc_startswith: empty prefix" {
    try std.testing.expect(sc_startswith("foobar", ""));
}

test "sc_startswith: null str" {
    try std.testing.expect(!sc_startswith(null, "foo"));
}

test "sc_startswith: null prefix" {
    try std.testing.expect(!sc_startswith("foo", null));
}

test "sc_endswith: matching suffix" {
    try std.testing.expect(sc_endswith("foobar", "bar"));
}

test "sc_endswith: non-matching suffix" {
    try std.testing.expect(!sc_endswith("foobar", "baz"));
}

test "sc_endswith: empty suffix" {
    try std.testing.expect(sc_endswith("foobar", ""));
}

test "sc_endswith: null str" {
    try std.testing.expect(!sc_endswith(null, "bar"));
}

test "sc_endswith: null suffix" {
    try std.testing.expect(!sc_endswith("bar", null));
}

test "sc_string_append: basic append" {
    var buf = [_]u8{0} ** 16;
    buf[0] = 'h'; buf[1] = 'i'; buf[2] = 0;
    const new_len = sc_string_append(&buf, buf.len, " world");
    try std.testing.expectEqual(@as(usize, 8), new_len);
    try std.testing.expectEqualStrings("hi world", buf[0..8]);
}

test "sc_string_append_char: basic" {
    var buf = [_]u8{0} ** 8;
    _ = sc_string_append_char(&buf, buf.len, 'x');
    try std.testing.expectEqualStrings("x", buf[0..1]);
}

test "sc_string_append_char_pair: basic" {
    var buf = [_]u8{0} ** 8;
    _ = sc_string_append_char_pair(&buf, buf.len, 'a', 'b');
    try std.testing.expectEqualStrings("ab", buf[0..2]);
}

test "sc_string_init: sets NUL" {
    var buf = [_]u8{0xFF} ** 4;
    sc_string_init(&buf, buf.len);
    try std.testing.expectEqual(@as(u8, 0), buf[0]);
}

test "sc_str_chomp: removes trailing newline" {
    var buf = "hello\n".*;
    _ = sc_str_chomp(&buf);
    try std.testing.expectEqualStrings("hello", std.mem.span(@as([*:0]u8, &buf)));
}

test "sc_str_chomp: removes multiple trailing newlines" {
    var buf = "hello\n\n\n".*;
    _ = sc_str_chomp(&buf);
    try std.testing.expectEqualStrings("hello", std.mem.span(@as([*:0]u8, &buf)));
}

test "sc_str_chomp: no trailing newline unchanged" {
    var buf = "hello".*;
    _ = sc_str_chomp(&buf);
    try std.testing.expectEqualStrings("hello", std.mem.span(@as([*:0]u8, &buf)));
}

test "sc_string_split: with delimiter" {
    var prefix = [_]u8{0} ** 32;
    var suffix = [_]u8{0} ** 32;
    sc_string_split("foo_bar", '_', &prefix, prefix.len, &suffix, suffix.len);
    try std.testing.expectEqualStrings("foo", std.mem.sliceTo(&prefix, 0));
    try std.testing.expectEqualStrings("bar", std.mem.sliceTo(&suffix, 0));
}

test "sc_string_split: no delimiter" {
    var prefix = [_]u8{0} ** 32;
    var suffix = [_]u8{0} ** 32;
    sc_string_split("foobar", '_', &prefix, prefix.len, &suffix, suffix.len);
    try std.testing.expectEqualStrings("foobar", std.mem.sliceTo(&prefix, 0));
    try std.testing.expectEqualStrings("", std.mem.sliceTo(&suffix, 0));
}

test "sc_string_quote: simple string" {
    var buf = [_]u8{0} ** 64;
    sc_string_quote(&buf, buf.len, "hello");
    try std.testing.expectEqualStrings("\"hello\"", std.mem.sliceTo(&buf, 0));
}

test "sc_string_quote: string with newline" {
    var buf = [_]u8{0} ** 64;
    sc_string_quote(&buf, buf.len, "a\nb");
    try std.testing.expectEqualStrings("\"a\\nb\"", std.mem.sliceTo(&buf, 0));
}
