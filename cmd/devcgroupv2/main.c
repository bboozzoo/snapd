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
#include <stdint.h>
#include <stdio.h>

#include "../libsnap-confine-private/utils.h"

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

    struct bpf_insn prog[] = {
		/* fill */
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
