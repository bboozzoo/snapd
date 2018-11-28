/*
 * Copyright (C) 2018 Canonical Ltd
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
#include "config.h"
#include "selinux-support.h"

#include <selinux/restorecon.h>
#include <selinux/selinux.h>
#include <selinux/context.h>

#include "../libsnap-confine-private/utils.h"
#include "../libsnap-confine-private/string-utils.h"

int sc_selinux_relabel_run_dir(void)
{
	if (is_selinux_enabled() < 1) {
		return 0;
	}

	int ret = selinux_restorecon("/run/snapd",
				     SELINUX_RESTORECON_RECURSE |
				     SELINUX_RESTORECON_IGNORE_MOUNTS |
				     SELINUX_RESTORECON_XDEV);
	if (ret == -1) {
		die("failed to restore context of /run/snapd");
	}
	return ret;
}

/**
 * Set security context for the snap
 *
 **/
int sc_selinux_set_snap_execcon(void)
{
	/* die("foo"); */
	if (is_selinux_enabled() < 1) {
		debug("selinux not enabled");
		return 0;
	}

	char *ctx_str = NULL;
	if (getcon(&ctx_str) == -1) {
		die("failed to obtain current process context");
	}
	debug("exec context: %s", ctx_str);

	context_t ctx = context_new(ctx_str);
	if (ctx == NULL) {
		die("failed to create context from context string %s", ctx_str);
	}

	debug("type: %s", context_type_get(ctx));
	if (sc_streq(context_type_get(ctx), "snappy_t")) {

		/* transition into snappy_snap_t domain */
		if (context_type_set(ctx, "snappy_unconfined_snap_t") != 0) {
			die("failed to update context %s type to snappy_snap_t",
			    ctx_str);
		}

		char *new_ctx_str = context_str(ctx);
		if (new_ctx_str == NULL) {
			die("failed to obtain string of new context");
		}
		if (setexeccon(new_ctx_str) == -1) {
			die("failed to set exec context to %s", new_ctx_str);
		}
		debug("context after next exec: %s", new_ctx_str);

	}

	context_free(ctx);
	freecon(ctx_str);
	return 0;
}
