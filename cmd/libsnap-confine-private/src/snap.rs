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
use std::ffi::CStr;
use std::os::raw::c_char;
use std::ptr;

use libsnap_confine_private_rs::{snap};

use crate::error::{self, sc_error_forward};

enum ErrorCode {
    SC_SNAP_INVALID_NAME = 1,
    SC_SNAP_INVALID_INSTANCE_KEY = 2,
    SC_SNAP_INVALID_INSTANCE_NAME = 3,
}

static SC_SNAP_DOMAIN: &str = "snap";

impl From<snap::Error<'_>> for error::sc_error {
    fn from(err: snap::Error) -> error::sc_error {
        error::new(
            SC_SNAP_DOMAIN,
            match err.kind() {
                snap::ErrorKind::InvalidName => ErrorCode::SC_SNAP_INVALID_NAME,
                snap::ErrorKind::InvalidInstanceName => ErrorCode::SC_SNAP_INVALID_INSTANCE_NAME,
                snap::ErrorKind::InvalidInstanceKey => ErrorCode::SC_SNAP_INVALID_INSTANCE_KEY,
            } as i32,
            err.msg(),
        )
    }
}

// TODO mediate between FFI cand pure Rust

#[no_mangle]
pub extern "C" fn sc_instance_name_validate(
    instance_name: *const c_char,
    sc_err: *mut *const error::sc_error,
) {
    if instance_name.is_null() {
        sc_error_forward(
            sc_err,
            error::new(
                SC_SNAP_DOMAIN,
                ErrorCode::SC_SNAP_INVALID_INSTANCE_NAME as i32,
                "snap instance name cannot be NULL",
            )
            .into_boxed_ptr(),
        );
        return;
    }
    let instance_name = match unsafe { CStr::from_ptr(instance_name).to_str() } {
        Ok(s) => s,
        Err(_) => {
            sc_error_forward(
                sc_err,
                error::new(
                    SC_SNAP_DOMAIN,
                    ErrorCode::SC_SNAP_INVALID_NAME as i32,
                    "snap instance name is not a valid string",
                )
                .into_boxed_ptr(),
            );
            return;
        }
    };
    if let Err(err) = snap::sc_instance_name_validate(instance_name) {
        sc_error_forward(sc_err, error::sc_error::from(err).into_boxed_ptr());
    } else {
        sc_error_forward(sc_err, ptr::null());
    }
}

#[no_mangle]
pub extern "C" fn sc_instance_key_validate(
    instance_key: *const c_char,
    sc_err: *mut *mut error::sc_error,
) {
    let instance_key = match unsafe { CStr::from_ptr(instance_key).to_str() } {
        Ok(s) => s,
        Err(_) => {
            unsafe {
                *sc_err = error::new(
                    SC_SNAP_DOMAIN,
                    ErrorCode::SC_SNAP_INVALID_INSTANCE_KEY as i32,
                    "snap name is not a valid string",
                )
                .into_boxed_ptr();
            }
            return;
        }
    };
    if let Err(err) = snap::sc_instance_key_validate(instance_key) {
        unsafe {
            *sc_err = error::sc_error::from(err).into_boxed_ptr();
        }
    }
}

#[no_mangle]
pub extern "C" fn sc_snap_name_validate(
    snap_name: *const c_char,
    sc_err: *mut *const error::sc_error,
) {
    if snap_name.is_null() {
        sc_error_forward(
            sc_err,
            error::new(
                SC_SNAP_DOMAIN,
                ErrorCode::SC_SNAP_INVALID_NAME as i32,
                "snap name cannot be NULL",
            )
            .into_boxed_ptr(),
        );
    } else {
        let maybe_s = unsafe { CStr::from_ptr(snap_name).to_str() };
        if let Ok(s) = maybe_s {
            if let Err(err) = snap::sc_snap_name_validate(s) {
                sc_error_forward(sc_err, error::sc_error::from(err).into_boxed_ptr());
            } else {
                sc_error_forward(sc_err, ptr::null());
            }
        } else {
            sc_error_forward(
                sc_err,
                error::new(
                    SC_SNAP_DOMAIN,
                    ErrorCode::SC_SNAP_INVALID_NAME as i32,
                    "snap name is not a valid string",
                )
                .into_boxed_ptr(),
            );
        }
    }
}


#[no_mangle]
pub extern "C" fn sc_is_hook_security_tag(security_tag: *const c_char) -> bool {
    let s_security_tag = unsafe { CStr::from_ptr(security_tag).to_str().unwrap_or("") };
    snap::sc_is_hook_security_tag(s_security_tag)
}

#[no_mangle]
pub extern "C" fn sc_security_tag_validate(
    security_tag: *const c_char,
    snap_name: *const c_char,
) -> bool {
    let s_security_tag = unsafe { CStr::from_ptr(security_tag).to_str().unwrap_or("") };
    let s_snap_name = unsafe { CStr::from_ptr(snap_name).to_str().unwrap_or("") };
    snap::sc_security_tag_validate(s_security_tag, s_snap_name)
}

// #[no_mangle]
// pub extern "C" fn sc_snap_split_instance_name(instance_name: &str) -> (&str, Option<&str>) {
//     sc_snap_split_instance_name_safe(instance_name)
// }

// #[no_mangle]
// pub extern "C" fn sc_snap_drop_instance_key(instance_name: &str) -> Result<&str, &str> {
//     sc_snap_drop_instance_key_safe(instance_name)
// }

#[cfg(test)]
mod tests {
    use super::*;
}
