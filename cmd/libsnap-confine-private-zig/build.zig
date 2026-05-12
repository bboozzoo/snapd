const std = @import("std");

pub fn build(b: *std.Build) void {
    const target = b.standardTargetOptions(.{});
    const optimize = b.standardOptimizeOption(.{});

    const root_mod = b.createModule(.{
        .root_source_file = b.path("src/root.zig"),
        .target = target,
        .optimize = optimize,
        .link_libc = true,
    });

    // Static library — for C tools (snap-discard-ns, system-shutdown, etc.)
    const lib_static = b.addLibrary(.{
        .name = "snap-confine-private",
        .root_module = root_mod,
        .linkage = .static,
    });
    b.installArtifact(lib_static);

    // Shared library — embedded in the Bun snap-confine binary via memfd_create
    const lib_shared = b.addLibrary(.{
        .name = "snap-confine-private",
        .root_module = root_mod,
        .linkage = .dynamic,
    });
    b.installArtifact(lib_shared);

    // Unit tests
    const test_mod = b.createModule(.{
        .root_source_file = b.path("src/root.zig"),
        .target = target,
        .optimize = optimize,
        .link_libc = true,
    });
    const tests = b.addTest(.{
        .root_module = test_mod,
    });

    const run_tests = b.addRunArtifact(tests);
    const test_step = b.step("test", "Run unit tests");
    test_step.dependOn(&run_tests.step);
}
