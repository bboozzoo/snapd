/*
 * Copyright (C) 2024 Canonical Ltd
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

// IMPORTANT: all the code in this file may be run with elevated privileges
// when invoking snap-update-ns from the setuid snap-confine.
//
// This file is a preprocessor for snap-update-ns' main() function. It will
// perform input validation and clear the environment so that snap-update-ns'
// go code runs with safe inputs when called by the setuid() snap-confine.

#include "bootstrap.h"

#include <errno.h>
#include <fcntl.h>
#include <grp.h>
#include <limits.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <unistd.h>

static int debug_log = 0;

#define log(...)                                       \
    do {                                               \
        if (debug_log) {                               \
            fprintf(stderr, __FILE__ ":" __VA_ARGS__); \
        }                                              \
    } while (0)

#define SNAP_SNAPD_CURRENT_NOSLASH "/snap/snapd/current"
#define SNAP_SNAPD_CURRENT "/snap/snapd/current/"

#define FIPS_MOD "ossl-modules-3/fips.so"

static const char *snap_fips_mod_per_arch[] = {
    SNAP_SNAPD_CURRENT "usr/lib/x86_64-linux-gnu/" FIPS_MOD, SNAP_SNAPD_CURRENT "usr/lib/aarch64-linux-gnu/" FIPS_MOD,
    SNAP_SNAPD_CURRENT "usr/lib/arm-linux-gnueabihf/" FIPS_MOD, SNAP_SNAPD_CURRENT "usr/lib/i386-linux-gnu/" FIPS_MOD,
    SNAP_SNAPD_CURRENT "usr/lib/riscv64-linux-gnu/" FIPS_MOD, SNAP_SNAPD_CURRENT "usr/lib/s390x-linux-gnu/" FIPS_MOD,

};

static size_t snap_fips_mod_per_arch_len = sizeof snap_fips_mod_per_arch / sizeof *snap_fips_mod_per_arch;

static const char *maybe_setup_fips(void) {
    const char *mod_path = NULL;

    for (size_t i = 0; i < snap_fips_mod_per_arch_len; i++) {
        struct stat sb = {0};
        if (stat(snap_fips_mod_per_arch[i], &sb) == 0) {
            /* FIPS module found */
            mod_path = snap_fips_mod_per_arch[i];
            log("found FIPS module at %s\n", mod_path);
            break;
        }
    }

    if (mod_path == NULL) {
        /* no FIPS module */
        log("FIPS module not found in the snapd snap\n");
        return NULL;
    }

    /* make a copy, but note we cannot free the string */
    char *modules_path = strdup(mod_path);
    /* replace last / with NULL, note we control the input */
    char *pos = strrchr(modules_path, '/');
    *pos = 0;

    return modules_path;
}

// bootstrap prepares snap-update-ns to work in the namespace of the snap given
// on command line.
void bootstrap(int argc, char **argv, char **envp) {
    int done = 0;

    for (size_t i = 0; envp[i] != NULL; i++) {
#define SNAPD_DEBUG_1 "SNAPD_DEBUG=1"
        if (strncmp(envp[i], SNAPD_DEBUG_1, strlen(SNAPD_DEBUG_1)) == 0) {
            debug_log = 1;
            continue;
        }
#define SNAPD_BOOTSTRAP_DONE_1 "SNAPD_BOOSTRAP_DONE=1"
        if (strncmp(envp[i], SNAPD_BOOTSTRAP_DONE_1, strlen(SNAPD_BOOTSTRAP_DONE_1)) == 0) {
            done = 1;
            continue;
        }
    }

    char self_path[PATH_MAX] = {0};
    if (readlink("/proc/self/exe", self_path, sizeof self_path) < 0) {
        /* cannot read symlink? */
        return;
    }

    log("self path: %s\n", self_path);

    char current_path[PATH_MAX] = {0};

    if (readlink(SNAP_SNAPD_CURRENT_NOSLASH, current_path, sizeof current_path) < 0) {
        /* cannto read symlink? */
        return;
    }

    log("current snapd snap is at %s\n", current_path);

    /* append / */
    strcat(current_path, "/");

    /* check if current revision is the prefix of the path to the current
     * binary */
    if (strncmp(current_path, current_path, strlen(current_path)) != 0) {
        /* not reexeced from the snapd snap */
        log("not reexecuting from the snapd snap\n");
        return;
    }

    int fd = open("/proc/sys/crypto/fips_enabled", O_RDONLY);
    if (fd == -1) {
        /* cannot check for FIPS mode */
        return;
    }

    char buf[1] = {0};
    int rd = read(fd, buf, sizeof(buf));
    close(fd);

    if (rd < 0) {
        /* cannot read */
        return;
    }

    if (buf[0] != '1') {
        /* FIPS not enabled */
        log("FIPS not enabled");
    }

    if (done == 1) {
        log("boostrap already done\n");
        return;
    }

    const char *modules_path = maybe_setup_fips();
    if (modules_path == NULL) {
        log("cannot derive FIPS modules path\n");
        return;
    }

    log("setting OPENSSL_MODULES to %s\n", modules_path);

    size_t envp_size = 0;
    for (size_t i = 0; envp[i] != NULL; i++) {
        envp_size++;
    }

    /* +1 for NULL pointer, +1 for new var, +1 for marker var */
    char **new_envp = alloca((envp_size + 3) * sizeof(char *));
    /* copy old pointers */
    for (size_t i = 0; i < envp_size; i++) {
        new_envp[i] = envp[i];
    }

    char *modules_env = calloc(sizeof(char), PATH_MAX);
    snprintf(modules_env, PATH_MAX, "OPENSSL_MODULES=%s", modules_path);

    log("adding env %s\n", modules_env);

    new_envp[envp_size] = modules_env;
    new_envp[envp_size + 1] = SNAPD_BOOTSTRAP_DONE_1;
    new_envp[envp_size + 2] = NULL;

    if (execve(self_path, argv, new_envp) < 0) {
        log("cannot reexec: %s\n", strerror(errno));
        /* die? */
        abort();
    }
    /* not reached, we either reexec or abort */
    __builtin_unreachable();
}
