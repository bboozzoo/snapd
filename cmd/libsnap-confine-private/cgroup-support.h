/*
 * Copyright (C) 2019 Canonical Ltd
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

#ifndef SC_CGROUP_SUPPORT_H
#define SC_CGROUP_SUPPORT_H

#include <fcntl.h>
#include <stdbool.h>

/**
 * sc_cgroup_create_and_join joins, perhaps creating, a cgroup hierarchy.
 *
 * The code assumes that an existing hierarchy rooted at "parent". It follows
 * up with a sub-hierarchy called "name", creating it if necessary. The created
 * sub-hierarchy is made to belong to root.root and the specified process is
 * moved there.
 **/
void sc_cgroup_create_and_join(const char *parent, const char *name, pid_t pid);

/**
 * sc_cgroup_is_v2() returns true if running on cgroups v2
 *
 **/
bool sc_cgroup_is_v2(void);

/**
 * sc_cgroup_is_tracking_snap checks whether the snap process are being
 * currently tracked in a cgroup.
 *
 * Note that sc_cgroup_is_tracking_snap will traverse the cgroups hierarchy
 * looking for a group name with a specific prefix. This is inherently racy. The
 * caller must have take the per snap instance lock.
 */
bool sc_cgroup_is_tracking_snap(const char *snap_instance);

#endif
