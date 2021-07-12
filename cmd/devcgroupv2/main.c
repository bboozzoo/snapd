/*
 * Copyright (C) 2021 Canonical Ltd
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License version 3 as
 * published by the Free Software Foundation.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 */

#define _GNU_SOURCE

#include <bpf/bpf.h>
#include <bpf/libbpf.h>
#include <fcntl.h>
#include <linux/bpf.h>
#include <stddef.h>
#include <stdint.h>
#include <stdio.h>

#include "../libsnap-confine-private/utils.h"
#include "bpf_insn.h"

__attribute__((unused)) static size_t align_to(size_t val, size_t alignment) {
    if (val % alignment == 0) {
        return val;
    }
    return alignment * ((val + alignment) / alignment);
}

int main(int argc, char **argv) {
    char log_buf[4096] = {0};

    if (argc < 3) {
        die("missing parameters, usage: %s <cgroup> <map-with-policy>", argv[0]);
    }

    const char *cgroup_path = argv[1];
    const char *map_obj_path = argv[2];

    int cgroup_fd = open(cgroup_path, O_PATH | O_DIRECTORY);
    if (cgroup_fd < 0) {
        die("cannot open cgroup directory");
    }

    int map_fd = bpf_obj_get(map_obj_path);
    if (map_fd < 0) {
        die("cannot obtain map fd");
    }

    // Basic rules about registers:
    // r0    - return value of built in functions and exit code of the program
    // r1-r5 - respective arguments to built in functions, clobbered by calls
    // r6-r9 - general purpose, preserved by callees
    // r10   - read only, stack pointer
    // Stack is 512 bytes.
    //
    // The function declaration implementing the program looks like this:
    // int program(struct bpf_cgroup_dev_ctx * ctx)
    // where *ctx is passed in r1, while the result goes to r0
    //
    // The map holding device entries has the following keys:
    struct key {
        uint8_t type;
        uint32_t major;
        uint32_t minor;
    } __attribute__((packed));
    /* just a placeholder for map value */
    uint8_t map_value __attribute__((unused));
    // where the value is 1 byte, but effectively ignored at this time. We are
    // using the map as a set, but 0 sized key cannot be used when creating a
    // map.
    size_t key_start = 17;
    /* NOTE: we pull a nasty hack, the structure is packed and its size isn't
     * aligned to multiples of 4; if we place it on a stack at an address
     * aligned to 4 bytes, the starting offsets of major and minor would be
     * unaligned; however, the first field of the structure is 1 byte, so we can
     * put the structure at 4 byte aligned address -1 and thus major and minor
     * end up aligned without too much hassle */
    struct bpf_insn prog[] = {
        /* r1 holds pointer to bpf_cgroup_dev_ctx */
        /* initialize r0 */
        BPF_MOV64_IMM(BPF_REG_0, 0), /* r0 = 0 */
        /* make some place on the stack for the key */
        BPF_MOV64_REG(BPF_REG_6, BPF_REG_10), /* r6 = r10 (sp) */
        /* r6 = where the key starts on the stack */
        BPF_ALU64_IMM(BPF_ADD, BPF_REG_6, -key_start), /* r6 = sp + (-key start offset) */
        /* copy major to our key */
        BPF_LDX_MEM(BPF_W, BPF_REG_2, BPF_REG_1,
                    offsetof(struct bpf_cgroup_dev_ctx, major)),               /* r2 = *(u32)(r1->major) */
        BPF_STX_MEM(BPF_W, BPF_REG_6, BPF_REG_2, offsetof(struct key, major)), /* *(r6 + offsetof(major)) = r2 */
        /* copy minor to our key */
        BPF_LDX_MEM(BPF_W, BPF_REG_2, BPF_REG_1,
                    offsetof(struct bpf_cgroup_dev_ctx, minor)),               /* r2 = *(u32)(r1->minor) */
        BPF_STX_MEM(BPF_W, BPF_REG_6, BPF_REG_2, offsetof(struct key, minor)), /* *(r6 + offsetof(minor)) = r2 */
        /* copy device access_type to r2 */
        BPF_LDX_MEM(BPF_W, BPF_REG_2, BPF_REG_1,
                    offsetof(struct bpf_cgroup_dev_ctx, access_type)), /* r2 = *(u32*)(r1->access_type) */
        /* access_type is encoded as (BPF_DEVCG_ACC_* << 16) | BPF_DEVCG_DEV_*,
         * but we only care about type */
        BPF_ALU32_IMM(BPF_AND, BPF_REG_2, 0xffff), /* r2 = r2 & 0xffff */
        /* is it a block device? */
        BPF_JMP_IMM(BPF_JNE, BPF_REG_2, BPF_DEVCG_DEV_BLOCK, 2),       /* if (r2 != BPF_DEVCG_DEV_BLOCK) goto pc + 2 */
        BPF_ST_MEM(BPF_B, BPF_REG_6, offsetof(struct key, type), 'b'), /* *(uint8*)(r6->type) = 'b' */
        BPF_JMP_A(5),
        BPF_JMP_IMM(BPF_JNE, BPF_REG_2, BPF_DEVCG_DEV_CHAR, 2),        /* if (r2 != BPF_DEVCG_DEV_CHAR) goto pc + 2 */
        BPF_ST_MEM(BPF_B, BPF_REG_6, offsetof(struct key, type), 'c'), /* *(uint8*)(r6->type) = 'c' */
        BPF_JMP_A(2),
        /* unknown device type */
        BPF_MOV64_IMM(BPF_REG_0, 0), /* r0 = 0 */
        BPF_EXIT_INSN(),
        /* back on happy path, prepare arguments for map lookup */
        BPF_LD_MAP_FD(BPF_REG_1, map_fd),
        BPF_MOV64_REG(BPF_REG_2, BPF_REG_6),                                 /* r2 = (struct key *) r6, */
        BPF_RAW_INSN(BPF_JMP | BPF_CALL, 0, 0, 0, BPF_FUNC_map_lookup_elem), /* r0 = bpf_map_lookup_elem(<map>,
           &key) */
        BPF_JMP_IMM(BPF_JEQ, BPF_REG_0, 0, 2),                               /* if (value_ptr == 0) goto pc + 2*/
        BPF_MOV64_IMM(BPF_REG_0, 1),                                         /* r0 = 1 */
        BPF_JMP_A(1),
        BPF_MOV64_IMM(BPF_REG_0, 0), /* r0 = 0 */
        BPF_EXIT_INSN(),
    };

    int verify = bpf_verify_program(BPF_PROG_TYPE_CGROUP_DEVICE, prog, sizeof(prog) / sizeof(prog[0]), 0, "GPL", 0,
                                    log_buf, sizeof(log_buf), 1);
    if (verify < 0) {
        die("program verification failed:\n%s\n", log_buf);
    }

    /* Attach bpf program */
    int prog_fd =
        bpf_load_program(BPF_PROG_TYPE_CGROUP_DEVICE, prog, sizeof(prog) / sizeof(prog[0]), "GPL", 0, NULL, 0);
    if (prog_fd < 0) {
        die("Failed to load program");
    }

    if (bpf_prog_attach(prog_fd, cgroup_fd, BPF_CGROUP_DEVICE, 0) < 0) {
        die("cannot attach program to cgroup");
    }

    return 0;
}
