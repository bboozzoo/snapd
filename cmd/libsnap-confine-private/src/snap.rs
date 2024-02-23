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
use regex::Regex;
use std::ffi::{self, CStr, CString};
use std::os::raw::c_char;
use std::ptr;

use crate::error;

enum Error {
    SC_SNAP_INVALID_NAME = 1,
    SC_SNAP_INVALID_INSTANCE_KEY = 2,
    SC_SNAP_INVALID_INSTANCE_NAME = 3,
}

static SC_SNAP_DOMAIN: &str = "snap";
const SNAP_NAME_LEN: usize = 40;
const SNAP_INSTANCE_KEY_LEN: usize = 10;
const SNAP_SECURITY_TAG_MAX_LEN: usize = 256;

// TODO mediate between FFI cand pure Rust

// #[no_mangle]
// pub extern "C" fn sc_instance_name_validate(instance_name: *const c_char) -> bool {
//     let cinstance_name = unsafe { CStr::from_ptr(instance_name) };
//     cinstance_name.to_str()
//     sc_instance_name_validate_safe(instance_name)
// }

pub fn sc_instance_name_validate_safe(instance_name: &str) -> Result<(), &str> {
    let mut it = instance_name.split("_");
    let maybe_snap_name = it.next();
    let maybe_instance_key = it.next();
    match it.next() {
        // do we have more?
        Some(_) => return Err("snap instance name can contain only one underscore"),
        _ => (),
    }
    if let Some(snap_name) = maybe_snap_name {
        sc_snap_name_validate_safe(snap_name)?
    }
    if let Some(instance_key) = maybe_instance_key {
        sc_instance_key_validate_safe(instance_key)?
    }
    Ok(())
}

#[no_mangle]
pub extern "C" fn sc_instance_key_validate(
    instance_key: *const c_char,
    sc_err: *mut *mut error::sc_error,
) {
    unsafe {
        if let Ok(s) = CStr::from_ptr(instance_key).to_str() {
            if let Err(err) = sc_instance_key_validate_safe(s) {
                *sc_err = error::new(
                    SC_SNAP_DOMAIN,
                    Error::SC_SNAP_INVALID_INSTANCE_KEY as i32,
                    "snap name is not a valid string",
                )
                .into_boxed_ptr();
            }
        } else {
            panic!("cannot convert C string")
        }
    }
}

pub fn sc_instance_key_validate_safe(instance_key: &str) -> Result<(), &str> {
    for c in instance_key.chars() {
        match c {
            'a'..='z' => (),
            '0'..='9' => (),
            _ => return Err("instance key must use lower case letters or digits"),
        }
    }
    if instance_key.len() > SNAP_INSTANCE_KEY_LEN {
        return Err("instance key must be shorter than 10 characters");
    }
    Ok(())
}

#[no_mangle]
pub extern "C" fn sc_snap_name_validate(
    snap_name: *const c_char,
    sc_err: *mut *mut error::sc_error,
) {
    if snap_name == ptr::null() {
        unsafe {
            *sc_err = error::new(
                SC_SNAP_DOMAIN,
                Error::SC_SNAP_INVALID_NAME as i32,
                "snap name cannot be NULL",
            )
            .into_boxed_ptr();
        }
    } else {
        unsafe {
            if let Ok(s) = CStr::from_ptr(snap_name).to_str() {
                if let Err(err) = sc_snap_name_validate_safe(s) {
                    unsafe {
                        *sc_err =
                            error::new(SC_SNAP_DOMAIN, Error::SC_SNAP_INVALID_NAME as i32, err)
                                .into_boxed_ptr();
                    }
                }
            } else {
                *sc_err = error::new(
                    SC_SNAP_DOMAIN,
                    Error::SC_SNAP_INVALID_NAME as i32,
                    "snap name is not a valid string",
                )
                .into_boxed_ptr();
            }
        }
    }
}

pub fn sc_snap_name_validate_safe(snap_name: &str) -> Result<(), &str> {
    // NOTE: This function should be synchronized with the two other
    // implementations: validate_snap_name and snap.ValidateName.

    // This is a regexp-free routine hand-codes the following pattern:
    //
    // "^([a-z0-9]+-?)*[a-z](-?[a-z0-9])*$"
    //
    // The only motivation for not using regular expressions is so that we
    // don't run untrusted input against a potentially complex regular
    // expression engine.
    let mut got_letter = false;
    let mut last: Option<char> = None;
    for c in snap_name.chars() {
        match c {
            'a'..='z' => {
                got_letter = true;
                last = Some(c);
                continue;
            }
            '0'..='9' => {
                last = Some(c);
                continue;
            }
            '-' => {
                match last {
                    Some('-') => return Err("snap name cannot contain two consecutive dashes"),
                    None => return Err("snap name cannot start with a dash"),
                    _ => (),
                }
                last = Some(c);
                continue;
            }
            _ => {
                return Err("snap name must use lower case letters, digits or dashes");
            }
        }
    }
    if last == Some('-') {
        return Err("snap name cannot end with a dash");
    }
    if !got_letter {
        return Err("snap name must contain at least one letter");
    }
    match snap_name.len() {
        0..=1 => return Err("snap name must be longer than 1 character"),
        2..=SNAP_NAME_LEN => (),
        _ => return Err("snap name must be shorter than 40 characters"),
    }
    Ok(())
}

#[no_mangle]
pub extern "C" fn sc_is_hook_security_tag(security_tag: *const c_char) -> bool {
    unsafe {
        let s_security_tag = CStr::from_ptr(security_tag).to_str().unwrap_or("");
        sc_is_hook_security_tag_safe(s_security_tag)
    }
}

pub extern "C" fn sc_is_hook_security_tag_safe(security_tag: &str) -> bool {
    let hook_security_tag_re =
        "^snap\\.[a-z](-?[a-z0-9])*(_[a-z0-9]{1,10})?\\.(hook\\.[a-z](-?[a-z0-9])*)$";
    let re = Regex::new(hook_security_tag_re).expect("canont compile regex");
    re.is_match(security_tag)
}

#[no_mangle]
pub extern "C" fn sc_security_tag_validate(
    security_tag: *const c_char,
    snap_name: *const c_char,
) -> bool {
    unsafe {
        let s_security_tag = CStr::from_ptr(security_tag).to_str().unwrap_or("");
        let s_snap_name = CStr::from_ptr(snap_name).to_str().unwrap_or("");
        sc_security_tag_validate_safe(s_security_tag, s_snap_name)
    }
}

pub extern "C" fn sc_security_tag_validate_safe(security_tag: &str, snap_name: &str) -> bool {
    if security_tag.len() > SNAP_SECURITY_TAG_MAX_LEN {
        return false;
    }

    let valid_re =
	      "^snap\\.([a-z0-9](-?[a-z0-9])*(_[a-z0-9]{1,10})?)\\.([a-zA-Z0-9](-?[a-zA-Z0-9])*|hook\\.[a-z](-?[a-z0-9])*)$";
    let re = Regex::new(valid_re).expect("canont compile regex");
    if let Some(c) = re.captures(security_tag) {
        if let Some(snap_from_tag) = c.get(1) {
            snap_from_tag.as_str() == snap_name
        } else {
            false
        }
    } else {
        // no matches
        false
    }
}

#[no_mangle]
pub extern "C" fn sc_snap_split_instance_name(instance_name: &str) -> (&str, Option<&str>) {
    sc_snap_split_instance_name_safe(instance_name)
}

pub fn sc_snap_split_instance_name_safe(instance_name: &str) -> (&str, Option<&str>) {
    match instance_name.find('_') {
        None => (instance_name, None),
        Some(pos) => {
            // a separator was provided, but the instance key can still be
            // empty, but it's not None
            (&instance_name[..pos], Some(&instance_name[pos + 1..]))
        }
    }
}

#[no_mangle]
pub extern "C" fn sc_snap_drop_instance_key(instance_name: &str) -> Result<&str, &str> {
    sc_snap_drop_instance_key_safe(instance_name)
}

pub fn sc_snap_drop_instance_key_safe(instance_name: &str) -> Result<&str, &str> {
    Ok(instance_name.split("_").next().unwrap())
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::println as info;

    #[test]
    fn test_sc_is_hook_security_tag() {
        assert!(sc_is_hook_security_tag_safe(
            "snap.foo_instance.hook.bar-baz"
        ));
        assert!(sc_is_hook_security_tag_safe("snap.foo_bar.hook.bar-baz"));
        assert!(sc_is_hook_security_tag_safe("snap.foo_bar.hook.f00"));
        assert!(sc_is_hook_security_tag_safe("snap.foo_bar.hook.f-0-0"));

        // Now, test the names we know are not valid hook security tags
        assert!(!sc_is_hook_security_tag_safe("snap.foo_instance.bar-baz"));
        assert!(!sc_is_hook_security_tag_safe("snap.name.app!hook.foo"));
        assert!(!sc_is_hook_security_tag_safe("snap.name.app.hook!foo"));
        assert!(!sc_is_hook_security_tag_safe("snap.name.app.hook.-foo"));
        assert!(!sc_is_hook_security_tag_safe("snap.foo_bar.hook.0abcd"));
        assert!(!sc_is_hook_security_tag_safe("snap.foo.hook.abc--"));
        assert!(!sc_is_hook_security_tag_safe("snap.foo_bar.hook.!foo"));
        assert!(!sc_is_hook_security_tag_safe("snap.foo_bar.hook.-foo"));
        assert!(!sc_is_hook_security_tag_safe("snap.foo_bar.hook!foo"));
        assert!(!sc_is_hook_security_tag_safe("snap.foo_bar.!foo"));
    }

    #[test]
    fn test_sc_security_tag_validate() {
        // First, test the names we know are good
        assert!(sc_security_tag_validate_safe("snap.name.app", "name"));
        assert!(sc_security_tag_validate_safe(
            "snap.network-manager.NetworkManager",
            "network-manager"
        ));
        assert!(sc_security_tag_validate_safe("snap.f00.bar-baz1", "f00"));
        assert!(sc_security_tag_validate_safe("snap.foo.hook.bar", "foo"));
        assert!(sc_security_tag_validate_safe(
            "snap.foo.hook.bar-baz",
            "foo"
        ));
        assert!(sc_security_tag_validate_safe(
            "snap.foo_instance.bar-baz",
            "foo_instance"
        ));
        assert!(sc_security_tag_validate_safe(
            "snap.foo_instance.hook.bar-baz",
            "foo_instance"
        ));
        assert!(sc_security_tag_validate_safe(
            "snap.foo_bar.hook.bar-baz",
            "foo_bar"
        ));

        // Now, test the names we know are bad
        assert!(!sc_security_tag_validate_safe(
            "pkg-foo.bar.0binary-bar+baz",
            "bar"
        ));
        assert!(!sc_security_tag_validate_safe("pkg-foo_bar_1.1", ""));
        assert!(!sc_security_tag_validate_safe("appname/..", ""));
        assert!(!sc_security_tag_validate_safe("snap", ""));
        assert!(!sc_security_tag_validate_safe("snap.", ""));
        assert!(!sc_security_tag_validate_safe("snap.name", "name"));
        assert!(!sc_security_tag_validate_safe("snap.name.", "name"));
        assert!(!sc_security_tag_validate_safe("snap.name.app.", "name"));
        assert!(!sc_security_tag_validate_safe("snap.name.hook.", "name"));
        assert!(!sc_security_tag_validate_safe("snap!name.app", "!name"));
        assert!(!sc_security_tag_validate_safe("snap.-name.app", "-name"));
        assert!(!sc_security_tag_validate_safe("snap.name!app", "name!"));
        assert!(!sc_security_tag_validate_safe("snap.name.-app", "name"));
        assert!(!sc_security_tag_validate_safe(
            "snap.name.app!hook.foo",
            "name"
        ));
        assert!(!sc_security_tag_validate_safe(
            "snap.name.app.hook!foo",
            "name"
        ));
        assert!(!sc_security_tag_validate_safe(
            "snap.name.app.hook.-foo",
            "name"
        ));
        assert!(!sc_security_tag_validate_safe(
            "snap.name.app.hook.f00",
            "name"
        ));
        assert!(!sc_security_tag_validate_safe("sna.pname.app", "pname"));
        assert!(!sc_security_tag_validate_safe("snap.n@me.app", "n@me"));
        assert!(!sc_security_tag_validate_safe("SNAP.name.app", "name"));
        assert!(!sc_security_tag_validate_safe("snap.Name.app", "Name"));
        // This used to be false but it's now allowed.
        assert!(sc_security_tag_validate_safe("snap.0name.app", "0name"));
        assert!(!sc_security_tag_validate_safe("snap.-name.app", "-name"));
        assert!(!sc_security_tag_validate_safe("snap.name.@app", "name"));
        assert!(!sc_security_tag_validate_safe(".name.app", "name"));
        assert!(!sc_security_tag_validate_safe("snap..name.app", ".name"));
        assert!(!sc_security_tag_validate_safe("snap.name..app", "name."));
        assert!(!sc_security_tag_validate_safe("snap.name.app..", "name"));
        // These contain invalid instance key
        assert!(!sc_security_tag_validate_safe("snap.foo_.bar-baz", "foo"));
        assert!(!sc_security_tag_validate_safe(
            "snap.foo_toolonginstance.bar-baz",
            "foo"
        ));
        assert!(!sc_security_tag_validate_safe(
            "snap.foo_inst@nace.bar-baz",
            "foo"
        ));
        assert!(!sc_security_tag_validate_safe(
            "snap.foo_in-stan-ce.bar-baz",
            "foo"
        ));
        assert!(!sc_security_tag_validate_safe(
            "snap.foo_in stan.bar-baz",
            "foo"
        ));

        // Test names that are both good, but snap name doesn't match security tag
        assert!(!sc_security_tag_validate_safe("snap.foo.hook.bar", "fo"));
        assert!(!sc_security_tag_validate_safe("snap.foo.hook.bar", "fooo"));
        assert!(!sc_security_tag_validate_safe("snap.foo.hook.bar", "snap"));
        assert!(!sc_security_tag_validate_safe("snap.foo.hook.bar", "bar"));
        assert!(!sc_security_tag_validate_safe(
            "snap.foo_instance.bar",
            "foo_bar"
        ));

        // Regression test 12to8
        assert!(sc_security_tag_validate_safe("snap.12to8.128to8", "12to8"));
        assert!(sc_security_tag_validate_safe(
            "snap.123test.123test",
            "123test"
        ));
        assert!(sc_security_tag_validate_safe(
            "snap.123test.hook.configure",
            "123test"
        ));

        // regression test snap.eon-edg-shb-pulseaudio.hook.connect-plug-i2c
        assert!(sc_security_tag_validate_safe(
            "snap.foo.hook.connect-plug-i2c",
            "foo"
        ));

        // // Security tag that's too long. The extra +2 is for the string
        // // terminator and to allow us to make the tag too long to validate.
        // char long_tag[SNAP_SECURITY_TAG_MAX_LEN + 2];
        // memset(long_tag, 'b', sizeof long_tag);
        // memcpy(long_tag, "snap.foo.b", sizeof "snap.foo.b" - 1);
        // long_tag[sizeof long_tag - 1] = '\0';
        // assert!(strlen(long_tag) == SNAP_SECURITY_TAG_MAX_LEN + 1);
        // assert!(!sc_security_tag_validate_safe(long_tag, "foo"));

        // // If we make it one byte shorter it will be valid.
        // long_tag[sizeof long_tag - 2] = '\0';
        // assert!(sc_security_tag_validate_safe(long_tag, "foo"));
    }

    fn test_snap_or_instance_name_validate(validate: fn(&str) -> Result<(), &str>) {
        assert_eq!(validate("hello-world"), Ok(()));
        assert_eq!(
            validate("hello world"),
            Err("snap name must use lower case letters, digits or dashes")
        );
        assert_eq!(
            validate(""),
            Err("snap name must contain at least one letter")
        );
        assert_eq!(validate("-foo"), Err("snap name cannot start with a dash"));
        assert_eq!(validate("foo-"), Err("snap name cannot end with a dash"));
        assert_eq!(
            validate("f--oo"),
            Err("snap name cannot contain two consecutive dashes")
        );

        let valid_names = [
            "aa", "aaa", "aaaa", "a-a", "aa-a", "a-aa", "a-b-c", "a0", "a-0", "a-0a", "01game",
            "1-or-2",
        ];
        for name in valid_names {
            info!("checking valid snap name: {}", name);
            assert_eq!(validate(name), Ok(()));
        }

        let invalid_names = [
            // name cannot be empty
            "",
            // too short
            "a",
            // names cannot be too long
            "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
            "xxxxxxxxxxxxxxxxxxxx-xxxxxxxxxxxxxxxxxxxx",
            "1111111111111111111111111111111111111111x",
            "x1111111111111111111111111111111111111111",
            "x-x-x-x-x-x-x-x-x-x-x-x-x-x-x-x-x-x-x-x-x",
            // dashes alone are not a name
            "-",
            "--",
            // double dashes in a name are not allowed
            "a--a",
            // name should not end with a dash
            "a-",
            // name cannot have any spaces in it
            "a ",
            " a",
            "a a",
            // a number alone is not a name
            "0",
            "123",
            "1-2-3",
            // identifier must be plain ASCII
            "日本語",
            // "한글",
            "ру́сский язы́к",
        ];
        for name in invalid_names {
            info!("checking invalid snap name: >{}<", name);
            assert_ne!(validate(name), Ok(()));
        }
    }

    #[test]
    fn test_sc_instance_name_validate() {
        test_snap_or_instance_name_validate(sc_instance_name_validate_safe);
    }

    #[test]
    fn test_sc_snap_name_validate() {
        test_snap_or_instance_name_validate(sc_snap_name_validate_safe);
    }

    #[test]
    fn test_sc_snap_drop_instance_key_basic() {
        assert_eq!(sc_snap_drop_instance_key("foo_bar"), Ok("foo"));
        assert_eq!(sc_snap_drop_instance_key("foo-bar_bar"), Ok("foo-bar"));
        assert_eq!(sc_snap_drop_instance_key("foo-bar"), Ok("foo-bar"));
        assert_eq!(sc_snap_drop_instance_key("_baz"), Ok(""));
        assert_eq!(sc_snap_drop_instance_key("foo"), Ok("foo"));
        /* 40 chars - snap name length */
        assert_eq!(
            sc_snap_drop_instance_key("0123456789012345678901234567890123456789"),
            Ok("0123456789012345678901234567890123456789")
        );
    }

    #[test]
    fn test_sc_snap_split_instance_name_basic() {
        assert_eq!(sc_snap_split_instance_name("foo_bar"), ("foo", Some("bar")));
        assert_eq!(
            sc_snap_split_instance_name("foo-bar_bar"),
            ("foo-bar", Some("bar"))
        );
        assert_eq!(sc_snap_split_instance_name("foo-bar"), ("foo-bar", None));
        assert_eq!(sc_snap_split_instance_name("_baz"), ("", Some("baz")));
        assert_eq!(sc_snap_split_instance_name("foo"), ("foo", None));
        assert_eq!(
            sc_snap_split_instance_name("hello_world_surprise"),
            ("hello", Some("world_surprise"))
        );
        assert_eq!(sc_snap_split_instance_name("_"), ("", Some("")));
        assert_eq!(sc_snap_split_instance_name("foo_"), ("foo", Some("")));
    }
}
