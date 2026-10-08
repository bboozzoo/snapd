#
# Copyright (C) 2017 Canonical Ltd
#
# This program is free software: you can redistribute it and/or modify
# it under the terms of the GNU General Public License version 3 as
# published by the Free Software Foundation.
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU General Public License for more details.
#
# You should have received a copy of the GNU General Public License
# along with this program.  If not, see <http://www.gnu.org/licenses/>.

SNAPD_ENVIRONMENT_FILE := /etc/environment
BINDIR := /usr/bin
LIBEXECDIR := /usr/lib
SYSTEMDSYSTEMUNITDIR := /lib/systemd/system

# Mirrors the defaults used by ../env/Makefile and ../systemd-env/Makefile:
# whether the real snap mount directory is the canonical /snap, or the
# alternate /var/lib/snapd/snap (with /snap as a symlink to it). Exactly one
# of snap.mount / var-lib-snapd-snap.mount is relevant for a given
# distribution's configuration, selected below. When both are set (as some
# packaging does, to support either layout on the same distribution), the
# alternate location wins, matching the preference already used elsewhere
# (e.g. openSUSE's %post picks the alternate location for new installs).
USE_CANONICAL_SNAP_MOUNT_DIR ?= true
USE_ALT_SNAP_MOUNT_DIR ?= false
ifeq ($(USE_ALT_SNAP_MOUNT_DIR),true)
SNAP_MOUNT_DIR_UNIT := var-lib-snapd-snap.mount
else
SNAP_MOUNT_DIR_UNIT := snap.mount
endif

# NOTE: the code in wrappers/core18.go assumes that all our service units's
# name start with the "snapd." prefix; units whose filename does not have this
# prefix will not be installed on the host! This is intentionally relied upon
# by SNAP_MOUNT_DIR_UNIT and var-snap.mount, below, which are only relevant
# for classic (non-Core) native packaging and must never be propagated to the
# host by that mechanism.
SYSTEMD_UNITS_GENERATED := $(wildcard *.in)
# NOTE: sort removes duplicates so this gives us all the units, generated or otherwise
SYSTEMD_UNITS = $(sort $(SYSTEMD_UNITS_GENERATED:.in=) $(wildcard *.service) $(wildcard *.timer) $(wildcard *.socket) $(wildcard *.target) $(SNAP_MOUNT_DIR_UNIT) var-snap.mount)

.PHONY: all
all: $(SYSTEMD_UNITS)

.PHONY: check
check: snapd.run-from-snap snapd.core-fixup.sh
	if command -v shellcheck >/dev/null; then shellcheck $^; fi

.PHONY: install
install: $(SYSTEMD_UNITS)
	# NOTE: old (e.g. 14.04) GNU coreutils doesn't -D with -t
	install -d -m 0755 $(DESTDIR)/$(SYSTEMDSYSTEMUNITDIR)
	install -m 0644 -t $(DESTDIR)/$(SYSTEMDSYSTEMUNITDIR) $^
	install -d -m 0755 $(DESTDIR)/$(LIBEXECDIR)/snapd
	install -m 0755 -t $(DESTDIR)/$(LIBEXECDIR)/snapd snapd.core-fixup.sh
	install -m 0755 -t $(DESTDIR)/$(LIBEXECDIR)/snapd snapd.run-from-snap

.PHONY: clean
clean:
	rm -f $(SYSTEMD_UNITS_GENERATED:.in=)

%: %.in
	cat $< | \
		sed s:@libexecdir@:$(LIBEXECDIR):g | \
		sed s:@SNAPD_ENVIRONMENT_FILE@:$(SNAPD_ENVIRONMENT_FILE):g | \
		sed s:@bindir@:$(BINDIR):g | \
		cat > $@
