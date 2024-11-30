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

const SNAP_NAME_LEN: usize = 40;
const SNAP_INSTANCE_KEY_LEN: usize = 10;
const SNAP_INSTANCE_LEN: usize = SNAP_NAME_LEN + 1 + SNAP_INSTANCE_KEY_LEN;
const SNAP_SECURITY_TAG_MAX_LEN: usize = 256;

#[derive(Debug, PartialEq, Clone, Copy)]
pub enum ErrorKind {
    InvalidName,
    InvalidInstanceKey,
    InvalidInstanceName,
}

#[derive(Debug, PartialEq)]
pub struct Error<'a> {
    error_kind: ErrorKind,
    msg: &'a str,
}

impl Error<'_> {
    pub fn new(kind: ErrorKind, msg: &str) -> Error {
        Error {
            error_kind: kind,
            msg,
        }
    }

    pub fn kind(&self) -> ErrorKind {
        self.error_kind
    }

    pub fn msg(&self) -> &str {
        self.msg
    }
}

pub fn sc_instance_name_validate(instance_name: &str) -> Result<(), Error> {
    if instance_name.len() > SNAP_INSTANCE_LEN {
        return Err(Error::new(
            ErrorKind::InvalidInstanceName,
            // TODO use const_format::concatcp ?
            "snap instance name can be at most 51 characters long",
        ));
    }

    let mut it = instance_name.split('_');
    let maybe_snap_name = it.next();
    let maybe_instance_key = it.next();
    if it.next().is_some() {
        // do we have more?
        return Err(Error::new(
            ErrorKind::InvalidInstanceName,
            "snap instance name can contain only one underscore",
        ));
    }
    if let Some(snap_name) = maybe_snap_name {
        sc_snap_name_validate(snap_name)?
    }
    if let Some(instance_key) = maybe_instance_key {
        sc_instance_key_validate(instance_key)?
    }
    Ok(())
}

pub fn sc_instance_key_validate(instance_key: &str) -> Result<(), Error> {
    fn validate(instance_key: &str) -> Result<(), &str> {
        for c in instance_key.chars() {
            match c {
                'a'..='z' => (),
                '0'..='9' => (),
                _ => return Err("instance key must use lower case letters or digits"),
            }
        }
        if instance_key.is_empty() {
            return Err("instance key must contain at least one letter or digit");
        }
        if instance_key.len() > SNAP_INSTANCE_KEY_LEN {
            return Err("instance key must be shorter than 10 characters");
        }
        Ok(())
    }
    match validate(instance_key) {
        Ok(()) => Ok(()),
        Err(err) => Err(Error::new(ErrorKind::InvalidInstanceKey, err)),
    }
}

pub fn sc_snap_name_validate(snap_name: &str) -> Result<(), Error> {
    // NOTE: This function should be synchronized with the two other
    // implementations: validate_snap_name and snap.ValidateName.

    // This is a regexp-free routine hand-codes the following pattern:
    //
    // "^([a-z0-9]+-?)*[a-z](-?[a-z0-9])*$"
    //
    // The only motivation for not using regular expressions is so that we
    // don't run untrusted input against a potentially complex regular
    // expression engine.
    fn validate(snap_name: &str) -> Result<(), &str> {
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
            0..=1 => Err("snap name must be longer than 1 character"),
            2..=SNAP_NAME_LEN => Ok(()),
            _ => Err("snap name must be shorter than 40 characters"),
        }
    }
    match validate(snap_name) {
        Ok(()) => Ok(()),
        Err(err) => Err(Error::new(ErrorKind::InvalidName, err)),
    }
}

pub fn sc_is_hook_security_tag(security_tag: &str) -> bool {
    let hook_security_tag_re =
        "^snap\\.[a-z](-?[a-z0-9])*(_[a-z0-9]{1,10})?\\.(hook\\.[a-z](-?[a-z0-9])*)$";
    let re = Regex::new(hook_security_tag_re).expect("canont compile regex");
    re.is_match(security_tag)
}

pub fn sc_security_tag_validate(security_tag: &str, snap_name: &str, comp: Option<&str>) -> bool {
    if security_tag.len() > SNAP_SECURITY_TAG_MAX_LEN {
        return false;
    }

    let valid_re =
	      "^snap\\.([a-z0-9](-?[a-z0-9])*(_[a-z0-9]{1,10})?)(\\.[a-zA-Z0-9](-?[a-zA-Z0-9])*|(\\+([a-z0-9](-?[a-z0-9])*))?\\.hook\\.[a-z](-?[a-z0-9])*)$";
    let re = Regex::new(valid_re).expect("canont compile regex");
    if let Some(c) = re.captures(security_tag) {
        // first capture is for verifying the full security tag, second capture
        // for verifying the snap_name is correct for this security tag, eighth capture
        // for verifying the component_name is correct for this security tag. the
        // expression currently contains 9 capture groups
        let maybe_snap_from_tag = c.get(1);
        let maybe_comp_from_tag = c.get(7);

        if comp.is_some() != maybe_comp_from_tag.is_some() {
            // if expecting a component, then it must be present, otherwise it
            // must be none
            return false;
        } else if let Some(expected_component) = comp {
            // expecting a component, then it must match
            if let Some(comp_from_tag) = maybe_comp_from_tag {
                if comp_from_tag.as_str() != expected_component {
                    return false;
                }
            }
        }

        if let Some(snap_from_tag) = maybe_snap_from_tag {
            snap_from_tag.as_str() == snap_name
        } else {
            false
        }
    } else {
        // no matches
        false
    }
}

pub fn sc_snap_split_instance_name(instance_name: &str) -> (&str, Option<&str>) {
    match instance_name.find('_') {
        None => (instance_name, None),
        Some(pos) => {
            // a separator was provided, but the instance key can still be
            // empty, but it's not None
            (&instance_name[..pos], Some(&instance_name[pos + 1..]))
        }
    }
}

pub fn sc_snap_drop_instance_key(instance_name: &str) -> Result<&str, &str> {
    Ok(instance_name.split('_').next().unwrap())
}

pub fn sc_snap_split_snap_component(component: &str) -> (&str, Option<&str>) {
    match component.find('+') {
        None => (component, None),
        Some(pos) => (&component[..pos], Some(&component[pos + 1..])),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::println as info;

    macro_rules! exp_error {
        ($kind:expr, $msg:expr) => {
            Err(Error {
                error_kind: $kind,
                msg: $msg,
            })
        };
    }

    #[test]
    fn test_sc_is_hook_security_tag() {
        assert!(sc_is_hook_security_tag("snap.foo_instance.hook.bar-baz"));
        assert!(sc_is_hook_security_tag("snap.foo_bar.hook.bar-baz"));
        assert!(sc_is_hook_security_tag("snap.foo_bar.hook.f00"));
        assert!(sc_is_hook_security_tag("snap.foo_bar.hook.f-0-0"));

        // Now, test the names we know are not valid hook security tags
        assert!(!sc_is_hook_security_tag("snap.foo_instance.bar-baz"));
        assert!(!sc_is_hook_security_tag("snap.name.app!hook.foo"));
        assert!(!sc_is_hook_security_tag("snap.name.app.hook!foo"));
        assert!(!sc_is_hook_security_tag("snap.name.app.hook.-foo"));
        assert!(!sc_is_hook_security_tag("snap.foo_bar.hook.0abcd"));
        assert!(!sc_is_hook_security_tag("snap.foo.hook.abc--"));
        assert!(!sc_is_hook_security_tag("snap.foo_bar.hook.!foo"));
        assert!(!sc_is_hook_security_tag("snap.foo_bar.hook.-foo"));
        assert!(!sc_is_hook_security_tag("snap.foo_bar.hook!foo"));
        assert!(!sc_is_hook_security_tag("snap.foo_bar.!foo"));
    }

    #[test]
    fn test_sc_security_tag_validate() {
        // First, test the names we know are good
        assert!(sc_security_tag_validate("snap.name.app", "name", None));
        assert!(sc_security_tag_validate(
            "snap.network-manager.NetworkManager",
            "network-manager",
            None
        ));
        assert!(sc_security_tag_validate("snap.f00.bar-baz1", "f00", None));
        assert!(sc_security_tag_validate("snap.foo.hook.bar", "foo", None));
        assert!(sc_security_tag_validate(
            "snap.foo.hook.bar-baz",
            "foo",
            None
        ));
        assert!(sc_security_tag_validate(
            "snap.foo_instance.bar-baz",
            "foo_instance",
            None
        ));
        assert!(sc_security_tag_validate(
            "snap.foo_instance.hook.bar-baz",
            "foo_instance",
            None
        ));
        assert!(sc_security_tag_validate(
            "snap.foo_bar.hook.bar-baz",
            "foo_bar",
            None
        ));

        // Now, test the names we know are bad
        assert!(!sc_security_tag_validate(
            "pkg-foo.bar.0binary-bar+baz",
            "bar",
            None
        ));
        assert!(!sc_security_tag_validate("pkg-foo_bar_1.1", "", None));
        assert!(!sc_security_tag_validate("appname/..", "", None));
        assert!(!sc_security_tag_validate("snap", "", None));
        assert!(!sc_security_tag_validate("snap.", "", None));
        assert!(!sc_security_tag_validate("snap.name", "name", None));
        assert!(!sc_security_tag_validate("snap.name.", "name", None));
        assert!(!sc_security_tag_validate("snap.name.app.", "name", None));
        assert!(!sc_security_tag_validate("snap.name.hook.", "name", None));
        assert!(!sc_security_tag_validate("snap!name.app", "!name", None));
        assert!(!sc_security_tag_validate("snap.-name.app", "-name", None));
        assert!(!sc_security_tag_validate("snap.name!app", "name!", None));
        assert!(!sc_security_tag_validate("snap.name.-app", "name", None));
        assert!(!sc_security_tag_validate(
            "snap.name.app!hook.foo",
            "name",
            None
        ));
        assert!(!sc_security_tag_validate(
            "snap.name.app.hook!foo",
            "name",
            None
        ));
        assert!(!sc_security_tag_validate(
            "snap.name.app.hook.-foo",
            "name",
            None
        ));
        assert!(!sc_security_tag_validate(
            "snap.name.app.hook.f00",
            "name",
            None
        ));
        assert!(!sc_security_tag_validate("sna.pname.app", "pname", None));
        assert!(!sc_security_tag_validate("snap.n@me.app", "n@me", None));
        assert!(!sc_security_tag_validate("SNAP.name.app", "name", None));
        assert!(!sc_security_tag_validate("snap.Name.app", "Name", None));
        // This used to be false but it's now allowed.
        assert!(sc_security_tag_validate("snap.0name.app", "0name", None));
        assert!(!sc_security_tag_validate("snap.-name.app", "-name", None));
        assert!(!sc_security_tag_validate("snap.name.@app", "name", None));
        assert!(!sc_security_tag_validate(".name.app", "name", None));
        assert!(!sc_security_tag_validate("snap..name.app", ".name", None));
        assert!(!sc_security_tag_validate("snap.name..app", "name.", None));
        assert!(!sc_security_tag_validate("snap.name.app..", "name", None));
        // These contain invalid instance key
        assert!(!sc_security_tag_validate("snap.foo_.bar-baz", "foo", None));
        assert!(!sc_security_tag_validate(
            "snap.foo_toolonginstance.bar-baz",
            "foo",
            None
        ));
        assert!(!sc_security_tag_validate(
            "snap.foo_inst@nace.bar-baz",
            "foo",
            None
        ));
        assert!(!sc_security_tag_validate(
            "snap.foo_in-stan-ce.bar-baz",
            "foo",
            None
        ));
        assert!(!sc_security_tag_validate(
            "snap.foo_in stan.bar-baz",
            "foo",
            None
        ));

        // Test names that are both good, but snap name doesn't match security tag
        assert!(!sc_security_tag_validate("snap.foo.hook.bar", "fo", None));
        assert!(!sc_security_tag_validate("snap.foo.hook.bar", "fooo", None));
        assert!(!sc_security_tag_validate("snap.foo.hook.bar", "snap", None));
        assert!(!sc_security_tag_validate("snap.foo.hook.bar", "bar", None));
        assert!(!sc_security_tag_validate(
            "snap.foo_instance.bar",
            "foo_bar",
            None
        ));

        // Regression test 12to8
        assert!(sc_security_tag_validate("snap.12to8.128to8", "12to8", None));
        assert!(sc_security_tag_validate(
            "snap.123test.123test",
            "123test",
            None
        ));
        assert!(sc_security_tag_validate(
            "snap.123test.hook.configure",
            "123test",
            None
        ));

        // regression test snap.eon-edg-shb-pulseaudio.hook.connect-plug-i2c
        assert!(sc_security_tag_validate(
            "snap.foo.hook.connect-plug-i2c",
            "foo",
            None
        ));

        // make sure that component hooks can be validated
        assert!(sc_security_tag_validate(
            "snap.foo+comp.hook.install",
            "foo",
            Some("comp")
        ));
        assert!(sc_security_tag_validate(
            "snap.foo_instance+comp.hook.install",
            "foo_instance",
            Some("comp")
        ));
        // make sure that only hooks from components can be validated, not apps
        assert!(!sc_security_tag_validate(
            "snap.foo+comp.app",
            "foo",
            Some("comp")
        ));

        // unexpected component names should not work
        assert!(!sc_security_tag_validate(
            "snap.foo+comp.hook.install",
            "foo",
            None
        ));
        assert!(!sc_security_tag_validate(
            "snap.foo+comp.hook.install",
            "foo",
            None
        ));

        // missing component names when we expect one should not work
        assert!(!sc_security_tag_validate(
            "snap.foo.hook.install",
            "foo",
            Some("comp")
        ));
        assert!(!sc_security_tag_validate(
            "snap.foo.hook.install",
            "foo",
            Some("comp")
        ));

        // mismatch component names should not work
        assert!(!sc_security_tag_validate(
            "snap.foo+comp.hook.install",
            "foo",
            Some("component")
        ));

        // empty component name should not work
        assert!(!sc_security_tag_validate(
            "snap.foo+comp.hook.install",
            "foo",
            Some("")
        ));

        // invalid component names should not work
        assert!(!sc_security_tag_validate(
            "snap.foo+coMp.hook.install",
            "foo",
            Some("coMp")
        ));
        assert!(!sc_security_tag_validate(
            "snap.foo+-omp.hook.install",
            "foo",
            Some("-omp")
        ));
        // // Security tag that's too long. The extra +2 is for the string
        // // terminator and to allow us to make the tag too long to validate.
        // char long_tag[SNAP_SECURITY_TAG_MAX_LEN + 2];
        // memset(long_tag, 'b', sizeof long_tag);
        // memcpy(long_tag, "snap.foo.b", sizeof "snap.foo.b" - 1);
        // long_tag[sizeof long_tag - 1] = '\0';
        // assert!(strlen(long_tag) == SNAP_SECURITY_TAG_MAX_LEN + 1);
        // assert!(!sc_security_tag_validate(long_tag, "foo"));

        // // If we make it one byte shorter it will be valid.
        // long_tag[sizeof long_tag - 2] = '\0';
        // assert!(sc_security_tag_validate(long_tag, "foo"));
    }
    fn test_snap_or_instance_name_validate(validate: fn(&str) -> Result<(), Error>) {
        assert_eq!(validate("hello-world"), Ok(()));
        assert_eq!(
            validate("hello world"),
            exp_error!(
                ErrorKind::InvalidName,
                "snap name must use lower case letters, digits or dashes"
            )
        );
        assert_eq!(
            validate(""),
            exp_error!(
                ErrorKind::InvalidName,
                "snap name must contain at least one letter"
            )
        );
        assert_eq!(
            validate("-foo"),
            exp_error!(ErrorKind::InvalidName, "snap name cannot start with a dash")
        );
        assert_eq!(
            validate("foo-"),
            exp_error!(ErrorKind::InvalidName, "snap name cannot end with a dash")
        );
        assert_eq!(
            validate("f--oo"),
            exp_error!(
                ErrorKind::InvalidName,
                "snap name cannot contain two consecutive dashes"
            )
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

        // Regression test: 12to8 and 123test
        assert_eq!(validate("12to8"), Ok(()));
        assert_eq!(validate("123test"), Ok(()));

        let good_bad_name = "u-94903713687486543234157734673284536758";
        for i in 3..=good_bad_name.len() {
            let name = &good_bad_name[..i];
            info!("checking valid snap name: >{}<", name);
            assert_eq!(validate(name), Ok(()))
        }
    }

    #[test]
    fn test_shared_sc_instance_name_validate() {
        test_snap_or_instance_name_validate(sc_instance_name_validate);
    }

    #[test]
    fn test_shared_sc_snap_name_validate() {
        test_snap_or_instance_name_validate(sc_snap_name_validate);
    }

    #[test]
    fn test_sc_instance_name_validate() {
        assert_eq!(sc_instance_name_validate("hello-world"), Ok(()));
        assert_eq!(sc_instance_name_validate("hello-world_foo"), Ok(()));

        // just the separator
        assert_eq!(
            sc_instance_name_validate("_"),
            exp_error!(
                ErrorKind::InvalidName,
                "snap name must contain at least one letter"
            )
        );

        // just name, with separator, missing instance key
        assert_eq!(
            sc_instance_name_validate("hello-world_"),
            exp_error!(
                ErrorKind::InvalidInstanceKey,
                "instance key must contain at least one letter or digit"
            )
        );

        // only separator and instance key, missing name
        assert_eq!(
            sc_instance_name_validate("_bar"),
            exp_error!(
                ErrorKind::InvalidName,
                "snap name must contain at least one letter"
            )
        );

        assert_eq!(
            sc_instance_name_validate(""),
            exp_error!(
                ErrorKind::InvalidName,
                "snap name must contain at least one letter"
            )
        );

        // third separator
        assert_eq!(
            sc_instance_name_validate("foo_bar_baz"),
            exp_error!(
                ErrorKind::InvalidInstanceName,
                "snap instance name can contain only one underscore"
            )
        );

        // too long, 52
        assert_eq!(
            sc_instance_name_validate("0123456789012345678901234567890123456789012345678901"),
            exp_error!(
                ErrorKind::InvalidInstanceName,
                "snap instance name can be at most 51 characters long"
            )
        );

        let valid_names = [
            "aa",
            "aaa",
            "aaaa",
            "aa_a",
            "aa_1",
            "aa_123",
            "aa_0123456789",
        ];
        for name in valid_names {
            info!("checking valid instance name: {}", name);
            assert_eq!(sc_instance_name_validate(name), Ok(()));
        }
        let invalid_names = [
            // too short
            "a",
            // only letters and digits in the instance key
            "a_--23))",
            "a_ ",
            "a_091234#",
            "a_123_456",
            // up to 10 characters for the instance key
            "a_01234567891",
            "a_0123456789123",
            // snap name must not be more than 40 characters, regardless of instance
            // key
            "01234567890123456789012345678901234567890_foobar",
            "01234567890123456789-01234567890123456789_foobar",
            // instance key  must be plain ASCII
            "foobar_日本語",
            // way too many underscores
            "foobar_baz_zed_daz",
            "foobar______",
        ];
        for name in invalid_names {
            info!("checking invalid instance name: >{}<", name);
            assert_ne!(sc_instance_name_validate(name), Ok(()));
        }
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

    // TODO add sc_snap_split_snap_component tests
}
