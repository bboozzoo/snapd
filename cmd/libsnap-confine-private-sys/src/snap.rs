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
use core::slice;
use std::ffi::{CStr, CString};
use std::os::raw::c_char;
use std::ptr;

use libsnap_confine_private_rs::snap;

use crate::error::{self, sc_error, sc_error_forward};
use crate::utils::die;

enum ErrorCode {
    SC_SNAP_INVALID_NAME = 1,
    SC_SNAP_INVALID_INSTANCE_KEY = 2,
    SC_SNAP_INVALID_INSTANCE_NAME = 3,
    SC_SNAP_MOUNT_DIR_UNSUPPORTED = 4,
    SC_SNAP_INVALID_COMPONENT = 5,
}

static SC_SNAP_DOMAIN: &str = "snap";

impl From<&snap::Error> for error::sc_error {
    fn from(err: &snap::Error) -> error::sc_error {
        error::new(
            SC_SNAP_DOMAIN,
            match err {
                snap::Error::InvalidName(_) => ErrorCode::SC_SNAP_INVALID_NAME,
                snap::Error::InvalidInstanceName(_) => ErrorCode::SC_SNAP_INVALID_INSTANCE_NAME,
                snap::Error::InvalidInstanceKey(_) => ErrorCode::SC_SNAP_INVALID_INSTANCE_KEY,
                snap::Error::InvalidComponent(_) => ErrorCode::SC_SNAP_INVALID_COMPONENT,
            } as i32,
            &err.to_string(),
        )
    }
}

fn internal_sc_instance_name_validate(instance_name: *const c_char) -> Result<(), error::sc_error> {
    if instance_name.is_null() {
        return Err(error::new(
            SC_SNAP_DOMAIN,
            ErrorCode::SC_SNAP_INVALID_INSTANCE_NAME as i32,
            "snap instance name cannot be NULL",
        ));
    }

    unsafe { CStr::from_ptr(instance_name).to_str() }
        .map_err(|_| {
            error::new(
                SC_SNAP_DOMAIN,
                ErrorCode::SC_SNAP_INVALID_NAME as i32,
                "snap instance name is not a valid string",
            )
        })
        .and_then(|name| {
            snap::sc_instance_name_validate(name).map_err(|err| error::sc_error::from(&err))
        })?;
    Ok(())
}

#[no_mangle]
pub unsafe extern "C" fn sc_instance_name_validate(
    instance_name: *const c_char,
    sc_err: *mut *const error::sc_error,
) {
    if let Err(err) = internal_sc_instance_name_validate(instance_name) {
        sc_error_forward(sc_err, err.into_boxed_ptr());
    } else {
        sc_error_forward(sc_err, ptr::null());
    }
}

fn internal_sc_instance_key_validate(instance_key: *const c_char) -> Result<(), error::sc_error> {
    unsafe { CStr::from_ptr(instance_key).to_str() }
        .map_err(|_| {
            error::new(
                SC_SNAP_DOMAIN,
                ErrorCode::SC_SNAP_INVALID_INSTANCE_KEY as i32,
                "snap name is not a valid string",
            )
        })
        .and_then(|key| {
            snap::sc_instance_key_validate(key).map_err(|err| error::sc_error::from(&err))
        })?;
    Ok(())
}

#[no_mangle]
pub unsafe extern "C" fn sc_instance_key_validate(
    instance_key: *const c_char,
    sc_err: *mut *const error::sc_error,
) {
    if let Err(err) = internal_sc_instance_key_validate(instance_key) {
        sc_error_forward(sc_err, err.into_boxed_ptr());
    } else {
        sc_error_forward(sc_err, ptr::null());
    }
}

fn internal_sc_snap_component_validate(
    snap_component: *const c_char,
    snap_instance: *const c_char,
) -> Result<(), error::sc_error> {
    let component = if snap_component.is_null() {
        return Err(error::new(
            SC_SNAP_DOMAIN,
            ErrorCode::SC_SNAP_INVALID_COMPONENT as i32,
            "snap component cannot be NULL",
        ));
    } else {
        unsafe { CStr::from_ptr(snap_component).to_str() }.map_err(|_| {
            error::new(
                SC_SNAP_DOMAIN,
                ErrorCode::SC_SNAP_INVALID_NAME as i32,
                "snap component is not a valid string",
            )
        })
    }?;

    let instance = if snap_instance.is_null() {
        None
    } else {
        unsafe { CStr::from_ptr(snap_instance).to_str() }
            .map_err(|_| {
                error::new(
                    SC_SNAP_DOMAIN,
                    ErrorCode::SC_SNAP_INVALID_NAME as i32,
                    "snap component is not a valid string",
                )
            })
            .map(|s| Some(s))?
    };

    snap::sc_snap_component_validate(component, instance).map_err(|err| error::sc_error::from(&err))
}

#[no_mangle]
pub unsafe extern "C" fn sc_snap_component_validate(
    snap_component: *const c_char,
    snap_instance: *const c_char,
    sc_err: *mut *const error::sc_error,
) {
    if let Err(err) = internal_sc_snap_component_validate(snap_component, snap_instance) {
        sc_error_forward(sc_err, err.into_boxed_ptr());
    } else {
        sc_error_forward(sc_err, ptr::null());
    }
}

fn internal_sc_snap_name_validate(snap_name: *const c_char) -> Result<(), error::sc_error> {
    if snap_name.is_null() {
        return Err(error::new(
            SC_SNAP_DOMAIN,
            ErrorCode::SC_SNAP_INVALID_NAME as i32,
            "snap name cannot be NULL",
        ));
    };

    unsafe { CStr::from_ptr(snap_name).to_str() }
        .map_err(|_| {
            error::new(
                SC_SNAP_DOMAIN,
                ErrorCode::SC_SNAP_INVALID_NAME as i32,
                "snap name is not a valid string",
            )
        })
        .and_then(|s| snap::sc_snap_name_validate(s).map_err(|err| error::sc_error::from(&err)))
}

#[no_mangle]
pub unsafe extern "C" fn sc_snap_name_validate(
    snap_name: *const c_char,
    sc_err: *mut *const error::sc_error,
) {
    if let Err(err) = internal_sc_snap_name_validate(snap_name) {
        sc_error_forward(sc_err, err.into_boxed_ptr());
    } else {
        sc_error_forward(sc_err, ptr::null());
    }
}

fn internal_sc_is_hook_security_tag(security_tag: *const c_char) -> Result<bool, sc_error> {
    unsafe { CStr::from_ptr(security_tag).to_str() }
        .map_err(|_| {
            error::new(
                SC_SNAP_DOMAIN,
                ErrorCode::SC_SNAP_INVALID_NAME as i32,
                "hook security tag is not a valid string",
            )
        })
        .and_then(|s| Ok(snap::sc_is_hook_security_tag(s)))
}

#[no_mangle]
pub unsafe extern "C" fn sc_is_hook_security_tag(security_tag: *const c_char) -> bool {
    match internal_sc_is_hook_security_tag(security_tag) {
        Err(_) => false,
        Ok(v) => v,
    }
}

fn internal_sc_security_tag_validate(
    c_security_tag: *const c_char,
    c_snap_name: *const c_char,
    c_component_name: *const c_char,
) -> Result<bool, sc_error> {
    let security_tag = if c_security_tag.is_null() {
        return Err(error::new(
            SC_SNAP_DOMAIN,
            ErrorCode::SC_SNAP_INVALID_NAME as i32,
            "security tag cannot be NULL",
        ));
    } else {
        unsafe { CStr::from_ptr(c_security_tag).to_str() }.map_err(|_| {
            error::new(
                SC_SNAP_DOMAIN,
                ErrorCode::SC_SNAP_INVALID_NAME as i32,
                "security tag is not a valid string",
            )
        })?
    };

    let snap_name = if c_snap_name.is_null() {
        return Err(error::new(
            SC_SNAP_DOMAIN,
            ErrorCode::SC_SNAP_INVALID_NAME as i32,
            "snap name cannot be NULL",
        ));
    } else {
        unsafe { CStr::from_ptr(c_snap_name).to_str() }.map_err(|_| {
            error::new(
                SC_SNAP_DOMAIN,
                ErrorCode::SC_SNAP_INVALID_NAME as i32,
                "snap name is not a valid string",
            )
        })?
    };

    let component_name = if c_component_name.is_null() {
        None
    } else {
        unsafe { CStr::from_ptr(c_component_name).to_str() }
            .map_err(|_| {
                error::new(
                    SC_SNAP_DOMAIN,
                    ErrorCode::SC_SNAP_INVALID_NAME as i32,
                    "component name is not a valid string",
                )
            })
            .map(|s| Some(s))?
    };

    Ok(snap::sc_security_tag_validate(
        security_tag,
        snap_name,
        component_name,
    ))
}

#[no_mangle]
pub unsafe extern "C" fn sc_security_tag_validate(
    c_security_tag: *const c_char,
    c_snap_name: *const c_char,
    c_component_name: *const c_char,
) -> bool {
    match internal_sc_security_tag_validate(c_security_tag, c_snap_name, c_component_name) {
        Err(_) => false,
        Ok(v) => v,
    }
}

fn internal_sc_security_tag_to_unit_name(
    c_instance_name: *const c_char,
) -> Result<String, error::sc_error> {
    if c_instance_name.is_null() {
        Err(error::new(
            SC_SNAP_DOMAIN,
            ErrorCode::SC_SNAP_INVALID_NAME as i32,
            "internal error: cannot split instance name when it is unset",
        ))
    } else {
        unsafe { CStr::from_ptr(c_instance_name).to_str() }
            .map_err(|_| {
                error::new(
                    SC_SNAP_DOMAIN,
                    ErrorCode::SC_SNAP_INVALID_NAME as i32,
                    "snap component is not a valid string",
                )
            })
            .and_then(|tag| {
                snap::sc_security_tag_to_unit_name(tag).map_err(|err| error::sc_error::from(&err))
            })
    }
}

#[no_mangle]
pub unsafe extern "C" fn sc_security_tag_to_unit_name(
    c_instance_name: *const c_char,
) -> *mut c_char {
    let c_dup_unit_name = match internal_sc_security_tag_to_unit_name(c_instance_name) {
        Err(err) => die!("{}", err),
        Ok(s) => libc::strdup(
            CString::new(s)
                .expect("cannot convert to C string")
                .as_c_str()
                .as_ptr(),
        ),
    };
    c_dup_unit_name
}

#[no_mangle]
pub unsafe extern "C" fn sc_snap_split_instance_name(
    instance_name: *const c_char,
    snap_name: *mut u8,
    snap_name_size: usize,
    instance_key: *mut u8,
    instance_key_size: usize,
) {
    if instance_name.is_null() {
        die!("internal error: cannot split instance name when it is unset");
    }
    if snap_name.is_null() && instance_key.is_null() {
        die!("internal error: cannot split instance name when both snap name and instance key are unset");
    }
    // TODO die on error?
    let s_instance_name = unsafe { CStr::from_ptr(instance_name).to_str().unwrap_or("") };

    let (name, key) = snap::sc_snap_split_instance_name(s_instance_name);
    if !snap_name.is_null() {
        let name_raw = match CString::new(name) {
            Ok(cs) => cs.into_bytes_with_nul(),
            Err(err) => die!("cannot convert to C string: {}", err),
        };
        if name_raw.len() > snap_name_size {
            die!("snap name buffer too small");
        }
        let snap_name = slice::from_raw_parts_mut(snap_name, snap_name_size);
        snap_name[..name_raw.len()].copy_from_slice(&name_raw);
    }

    if !instance_key.is_null() {
        let instance_key = slice::from_raw_parts_mut(instance_key, instance_key_size);
        if let Some(key) = key {
            let key_raw = match CString::new(key) {
                Ok(cs) => cs.into_bytes_with_nul(),
                Err(err) => die!("cannot convert to C string: {}", err),
            };
            if key_raw.len() > instance_key_size {
                die!("instance key buffer too small");
            }
            instance_key[..key_raw.len()].copy_from_slice(&key_raw);
        } else {
            // terminate buffer with \0
            instance_key[0] = 0;
        }
    }
}

#[no_mangle]
pub unsafe extern "C" fn sc_snap_drop_instance_key(
    instance_name: *const c_char,
    snap_name: *mut u8,
    snap_name_size: usize,
) {
    sc_snap_split_instance_name(instance_name, snap_name, snap_name_size, ptr::null_mut(), 0);
}

#[no_mangle]
pub unsafe extern "C" fn sc_snap_split_snap_component(
    c_snap_component: *const c_char,
    c_snap_name: *mut u8,
    c_snap_name_size: usize,
    c_component_name: *mut u8,
    c_component_name_size: usize,
) {
    if c_snap_component.is_null() {
        die!("internal error: cannot split instance name when it is unset");
    }
    if c_snap_name.is_null() && c_component_name.is_null() {
        die!("internal error: cannot split instance name when both snap name and instance key are unset");
    }
    // TODO die on error?
    let s_component = unsafe { CStr::from_ptr(c_snap_component).to_str().unwrap_or("") };

    let (snap_name, component_name) = snap::sc_snap_split_snap_component(s_component);
    if !c_snap_name.is_null() {
        let snap_name_raw = match CString::new(snap_name) {
            Ok(cs) => cs.into_bytes_with_nul(),
            Err(err) => die!("cannot convert to C string: {}", err),
        };
        if snap_name_raw.len() > c_snap_name_size {
            die!("snap name buffer too small");
        }
        let c_snap_name_buf = slice::from_raw_parts_mut(c_snap_name, c_snap_name_size);
        c_snap_name_buf[..snap_name_raw.len()].copy_from_slice(&snap_name_raw);
    }

    if !c_component_name.is_null() {
        let c_component_name_buf =
            slice::from_raw_parts_mut(c_component_name, c_component_name_size);
        if let Some(component_name) = component_name {
            let component_name_raw = match CString::new(component_name) {
                Ok(cs) => cs.into_bytes_with_nul(),
                Err(err) => die!("cannot convert to C string: {}", err),
            };
            if component_name_raw.len() > c_component_name_size {
                die!("instance key buffer too small");
            }
            c_component_name_buf[..component_name_raw.len()].copy_from_slice(&component_name_raw);
        } else {
            // terminate buffer with \0
            c_component_name_buf[0] = 0;
        }
    }
}
