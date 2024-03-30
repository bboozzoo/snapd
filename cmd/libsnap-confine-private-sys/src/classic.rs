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
use libsnap_confine_private_rs::distro;

enum sc_distro {
	  SC_DISTRO_CORE16,	// As present in both "core" and later on in "core16"
	  SC_DISTRO_CORE_OTHER,	// Any core distribution.
	  SC_DISTRO_CLASSIC,	// Any classic distribution.
}

#[no_mangle]
pub extern "C" fn sc_classify_distro() -> sc_distro {
    match distro::classify() {
        Ok(distro) => {
            match distro {
                distro::Core16 => SC_DISTRO_CORE16,
                distro::CoreOther => SC_DISTRO_CORE_OTHER,
                distro::Classic => SC_DISTRO_CLASSIC,
            }
        }
        Err(_) => {
            // FIXME die?
            SC_DISTRO_CLASSIC
        }
    }
}

#[no_mangle]
pub extern "C" fn sc_is_debian_like() -> sc_distro {
    match distro::is_debian_like() {
        Ok(is) =>  is,
        Err(_) => {
            // FIXME die?
            false
        }
    }
}
