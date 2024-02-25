/*
 * Copyright (C) 2015 Canonical Ltd
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
#include "snap.h"

#include <errno.h>
#include <regex.h>
#include <stddef.h>
#include <stdlib.h>
#include <string.h>
#include <ctype.h>

#include "utils.h"
#include "string-utils.h"
#include "cleanup-funcs.h"

void sc_snap_drop_instance_key(const char *instance_name, char *snap_name,
			       size_t snap_name_size)
{
	sc_snap_split_instance_name(instance_name, snap_name, snap_name_size,
				    NULL, 0);
}

void sc_snap_split_instance_name(const char *instance_name, char *snap_name,
				 size_t snap_name_size, char *instance_key,
				 size_t instance_key_size)
{
	if (instance_name == NULL) {
		die("internal error: cannot split instance name when it is unset");
	}
	if (snap_name == NULL && instance_key == NULL) {
		die("internal error: cannot split instance name when both snap name and instance key are unset");
	}

	const char *pos = strchr(instance_name, '_');
	const char *instance_key_start = "";
	size_t snap_name_len = 0;
	size_t instance_key_len = 0;
	if (pos == NULL) {
		snap_name_len = strlen(instance_name);
	} else {
		snap_name_len = pos - instance_name;
		instance_key_start = pos + 1;
		instance_key_len = strlen(instance_key_start);
	}

	if (snap_name != NULL) {
		if (snap_name_len >= snap_name_size) {
			die("snap name buffer too small");
		}

		memcpy(snap_name, instance_name, snap_name_len);
		snap_name[snap_name_len] = '\0';
	}

	if (instance_key != NULL) {
		if (instance_key_len >= instance_key_size) {
			die("instance key buffer too small");
		}
		memcpy(instance_key, instance_key_start, instance_key_len);
		instance_key[instance_key_len] = '\0';
	}
}
