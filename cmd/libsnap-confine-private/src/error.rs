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

use libc;
use std::boxed::Box;
use std::ffi::{CStr, CString};
use std::os::raw::{c_char, c_int};
use std::ptr;

use crate::utils::die;

#[derive(Debug)]
pub struct sc_error {
    domain: CString,
    code: c_int,
    msg: CString,
}

#[no_mangle]
pub extern "C" fn sc_error_new(
    domain: *const c_char,
    code: c_int,
    msgfmt: *const c_char,
) -> *mut sc_error {
    // TODO error handling?
    let s_domain = unsafe { CStr::from_ptr(domain).to_str().unwrap() };
    let s_msg = unsafe { CStr::from_ptr(msgfmt).to_str().unwrap() };
    let mut err = Box::new(new(s_domain, code, s_msg));
    Box::into_raw(err)
}

pub fn new(domain: &str, code: i32, msg: &str) -> sc_error {
    sc_error {
        domain: CString::new(domain).unwrap(),
        code,
        msg: CString::new(msg).unwrap(),
    }
}

impl sc_error {
    pub fn into_boxed(self: Self) -> Box<Self> {
        Box::new(self)
    }

    pub fn into_boxed_ptr(self: Self) -> *mut Self {
        Box::into_raw(self.into_boxed())
    }
}

#[no_mangle]
pub extern "C" fn sc_error_free(self_err: *mut sc_error) {
    if self_err.is_null() {
        return;
    }
    let _ = unsafe { Box::from_raw(self_err) };
}

#[no_mangle]
pub extern "C" fn sc_error_domain(self_err: *const sc_error) -> *const c_char {
    unsafe {
        if self_err.is_null() {
            die!("cannot obtain domain from NULL error");
        }
        (*self_err).domain.as_ptr()
    }
}

#[no_mangle]
pub extern "C" fn sc_error_code(self_err: *const sc_error) -> c_int {
    unsafe {
        if self_err.is_null() {
            // TODO die
            panic!("cannot obtain error code from NULL error");
        }
        (*self_err).code
    }
}

#[no_mangle]
pub extern "C" fn sc_error_msg(self_err: *const sc_error) -> *const c_char {
    unsafe {
        if self_err == ptr::null() {
            // TODO die
            panic!("cannot obtain error message from NULL error");
        }
        (*self_err).msg.as_ptr()
    }
}

#[no_mangle]
pub extern "C" fn sc_error_forward(
    recipient: *mut *const sc_error,
    self_err: *const sc_error,
) -> c_int {
    if recipient != ptr::null_mut() {
        unsafe {
            *recipient = self_err;
        }
    }
    if self_err != ptr::null() {
        -1
    } else {
        0
    }
}

#[no_mangle]
pub extern "C" fn sc_error_match(
    self_err: *const sc_error,
    domain: *const c_char,
    code: c_int,
) -> bool {
    if domain == ptr::null() {
        die!("cannot match error to a NULL domain");
    }
    if self_err == ptr::null() {
        return false;
    }
    let domain = unsafe { CStr::from_ptr(domain).to_str().unwrap() };
    unsafe { domain == (*self_err).domain.to_str().unwrap() && code == (*self_err).code }
}

// #[no_mangle]
// pub extern "C" fn sc_die_on_error(self_err: *const sc_error) {
//     if self_err != ptr::null() {
//         unsafe { libc::exit(1) }
//     }
// }
