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

use std::os::raw::{c_char, c_int};
use std::ffi::{self, CString, Cstr};
use std::Box;
use std::ptr;
use libc;

#[repr(C)]
#[derive(Debug)]
struct sc_error {
    domain: *const c_char,
    code: c_int,
    msg: *mut c_char,
}

#[no_mangle]
pub extern "C" fn sc_error_init(domain: *const c_char, code: int,
                     msgfmt: *const c_char) -> *mut sc_error {
    let mut err = sc_error{
        domain,
        code,
    };
    Box::into_raw(err)
}

#[no_mangle]
pub extern "C" fn sc_error_domain(self: *const sc_error) -> *const c_char {
    self.domain
}

#[no_mangle]
pub extern "C" fn sc_error_code(self: *const sc_error) -> c_int {
    self.code
}

#[no_mangle]
pub extern "C" fn sc_error_msg(self: *const sc_error) -> *const c_char {
    self.msg
}

#[no_mangle]
pub extern "C" fn sc_error_forward(recipient: *mut sc_error, self: *const sc_error) {

}

#[no_mangle]
pub extern "C" fn sc_error_match(self: *const sc_error, domain: *const c_char, code: c_int) -> bool {

}

#[no_mangle]
pub extern "C" fn sc_die_on_error(self: *const sc_error) {
    if self != ptr::null() {

        libc::exit(1)
    }
}
