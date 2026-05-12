// libsnap-confine-private — Zig implementation
// Root module: forces inclusion of all sub-modules so their `export fn`
// symbols appear in both the static archive and the shared library.
//
// Using `comptime { _ = @import(...) }` is required — a plain
// `pub const x = @import(...)` does NOT force the linker to include
// exported symbols from sub-modules in a shared library.

comptime {
    _ = @import("string_utils.zig");
}

// Pull in all test blocks from sub-modules when running `zig build test`
test {
    _ = @import("string_utils.zig");
}
