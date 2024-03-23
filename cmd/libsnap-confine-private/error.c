/*
 * Copyright (C) 2016 Canonical Ltd
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
#include "error.h"

// To get vasprintf
#define _GNU_SOURCE

#include "utils.h"
#include "cleanup-funcs.h"

#include <errno.h>
#include <stdarg.h>
#include <stdio.h>
#include <string.h>

struct sc_error {
	// Error domain defines a scope for particular error codes.
	const char *domain;
	// Code differentiates particular errors for the programmer.
	// The code may be zero if the particular meaning is not relevant.
	int code;
	// Message carries a formatted description of the problem.
	char *msg;
};

extern sc_error *sc_error_new(const char *domain, int code, const char *msg);

static sc_error *sc_error_initv(const char *domain, int code,
				const char *msgfmt, va_list ap)
{
	// Set errno in case we die.
	errno = 0;
	char *msg SC_CLEANUP(sc_cleanup_string) = NULL;
	if (vasprintf(&msg, msgfmt, ap) == -1) {
		die("cannot format error message");
	}
	return sc_error_new(domain, code, msg);
}

sc_error *sc_error_init(const char *domain, int code, const char *msgfmt, ...)
{
	va_list ap;
	va_start(ap, msgfmt);
	sc_error *err = sc_error_initv(domain, code, msgfmt, ap);
	va_end(ap);
	return err;
}

sc_error *sc_error_init_from_errno(int errno_copy, const char *msgfmt, ...)
{
	va_list ap;
	va_start(ap, msgfmt);
	sc_error *err = sc_error_initv(SC_ERRNO_DOMAIN, errno_copy, msgfmt, ap);
	va_end(ap);
	return err;
}

sc_error *sc_error_init_simple(const char *msgfmt, ...)
{
	va_list ap;
	va_start(ap, msgfmt);
	sc_error *err = sc_error_initv(SC_LIBSNAP_DOMAIN,
				       SC_UNSPECIFIED_ERROR, msgfmt, ap);
	va_end(ap);
	return err;
}

sc_error *sc_error_init_api_misuse(const char *msgfmt, ...)
{
	va_list ap;
	va_start(ap, msgfmt);
	sc_error *err = sc_error_initv(SC_LIBSNAP_DOMAIN,
				       SC_API_MISUSE, msgfmt, ap);
	va_end(ap);
	return err;
}

void sc_cleanup_error(sc_error **ptr)
{
	sc_error_free(*ptr);
	*ptr = NULL;
}
