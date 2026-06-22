/*
 * Copyright (C) 2026 Canonical Ltd
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

/*
 * snap-cli-wrap is an explicit entrypoint to the snap CLI command. It is
 * expected to be used only on systems where implicit transitions cannot be
 * fully described in the security policy targeting usr/bin/snap. One such
 * scenario is a system with dedicated SELinux policies for snap and snapd
 * commands: since snap is a symlink to the snapd binary, the kernel resolves
 * the symlink before checking the file label for the domain transition, so
 * the snap CLI would end up in the snapd domain (snappy_t) rather than the
 * snap CLI domain (snappy_cli_t). This wrapper is a real binary carrying the
 * snappy_cli_exec_t label, ensuring the correct domain transition fires on
 * exec.
 */

#include <stdio.h>
#include <unistd.h>

#define SNAPD_PATH LIBEXECDIR "/snapd"

int main(int argc, char **argv) {
    execv(SNAPD_PATH, argv);
    /* execv only returns on error */
    perror("execv " SNAPD_PATH);
    return 1;
}
