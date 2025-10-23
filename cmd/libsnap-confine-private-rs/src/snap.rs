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

#[derive(thiserror::Error, Debug, PartialEq)]
pub enum Error {
    #[error("{0}")]
    InvalidComponent(String),
    #[error("{0}")]
    InvalidInstanceKey(String),
    #[error("{0}")]
    InvalidInstanceName(String),
    #[error("{0}")]
    InvalidName(String),
}

pub fn sc_instance_name_validate(instance_name: &str) -> Result<(), Error> {
    if instance_name.len() > SNAP_INSTANCE_LEN {
        return Err(Error::InvalidInstanceName(
            "snap instance name can be at most 51 characters long".to_string(),
        ));
    }

    let mut it = instance_name.split('_');
    it.next().map_or(Ok(()), sc_snap_name_validate)?;
    it.next().map_or(Ok(()), sc_instance_key_validate)?;
    it.next().map_or(Ok(()), |_| 
        // we have more?
        Err(Error::InvalidInstanceName(
            "snap instance name can contain only one underscore".to_string(),
        )
        ))
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
    validate(instance_key).map_err(|err| Error::InvalidInstanceKey(err.to_string()))
}

fn validate_as_snap_or_component_name(snap_name: &str) -> Result<(), &str> {
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
                    Some('-') => return Err("cannot contain two consecutive dashes"),
                    None => return Err("cannot start with a dash"),
                    _ => (),
                }
                last = Some(c);
                continue;
            }
            _ => {
                return Err("must use lower case letters, digits or dashes");
            }
        }
    }
    if last == Some('-') {
        return Err("cannot end with a dash");
    }
    if !got_letter {
        return Err("must contain at least one letter");
    }
    match snap_name.len() {
        0..=1 => Err("must be longer than 1 character"),
        2..=SNAP_NAME_LEN => Ok(()),
        _ => Err("must be shorter than 40 characters"),
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
    validate_as_snap_or_component_name(snap_name)
        .map_err(|err| Error::InvalidName(format!("snap name {}", err)))
}

pub fn sc_snap_component_validate(
    snap_component: &str,
    snap_instance: Option<&str>,
) -> Result<(), Error> {
    let (snap_name, component_name) = snap_component
        .find('+')
        .ok_or(Error::InvalidComponent(
            "snap component must contain a +".to_string(),
        ))
        .map(|pos| (&snap_component[..pos], &snap_component[pos + 1..]))?;

    if snap_name.len() > SNAP_NAME_LEN {
        return Err(Error::InvalidComponent(
            "snap name must be shorter than 40 characters".to_string(),
        ));
    }

    if component_name.len() > SNAP_NAME_LEN {
        return Err(Error::InvalidComponent(
            "component name must be shorter than 40 characters".to_string(),
        ));
    }

    validate_as_snap_or_component_name(snap_name)
        .map_err(|err| Error::InvalidComponent(format!("snap name in component {}", err)))?;

    validate_as_snap_or_component_name(component_name)
        .map_err(|err| Error::InvalidComponent(format!("component name {}", err)))?;

    snap_instance.map_or(Ok(()), |snap_instance| {
        sc_snap_drop_instance_key(snap_instance)
            .map_err(|err| Error::InvalidComponent(err.to_string()))
            .and_then(|instance_snap_name| {
                if instance_snap_name != snap_name {
                    Err(Error::InvalidComponent(
                        "snap name in component must match snap name in instance".to_string(),
                    ))
                } else {
                    Ok(())
                }
            })
    })
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
    re.captures(security_tag).is_some_and(|c| {
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
    })
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

pub fn sc_security_tag_to_unit_name(instance_name: &str) -> Result<String, Error> {
    let mut s = String::new();
    for c in instance_name.chars() {
        match c {
            '0'..='9' | 'a'..='z' | 'A'..='Z' | '_' | '-' | '.' => s.push(c),
            '+' => s.push_str("\\x2b"),
            _ => {
                return Err(Error::InvalidName(
                    "unexpected character in a validated security tag".to_string(),
                ))
            }
        }
    }
    Ok(s)
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::println as info;

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

    #[test]
    fn test_sc_security_tag_to_unit_name() {
        assert_eq!(
            sc_security_tag_to_unit_name("snap.foo+comp.hook.install"),
            Ok("snap.foo\\x2bcomp.hook.install".to_string())
        );

        assert_eq!(
            sc_security_tag_to_unit_name("snap.foo.bar"),
            Ok("snap.foo.bar".to_string())
        );
        assert_eq!(
            sc_security_tag_to_unit_name("snap.foo_dev.bar"),
            Ok("snap.foo_dev.bar".to_string())
        );
    }

    #[test]
    fn test_sc_security_tag_to_unit_name_invalid() {
        assert_eq!(
            sc_security_tag_to_unit_name("snap.foo|dev.bar"),
            Err(Error::InvalidName(
                "unexpected character in a validated security tag".to_string()
            ))
        );
    }

    fn test_snap_or_instance_name_validate(validate: fn(&str) -> Result<(), Error>) {
        assert_eq!(validate("hello-world"), Ok(()));
        assert_eq!(
            validate("hello world"),
            Err(Error::InvalidName(
                "snap name must use lower case letters, digits or dashes".to_string()
            ))
        );
        assert_eq!(
            validate(""),
            Err(Error::InvalidName(
                "snap name must contain at least one letter".to_string()
            ))
        );
        assert_eq!(
            validate("-foo"),
            Err(Error::InvalidName(
                "snap name cannot start with a dash".to_string()
            ))
        );
        assert_eq!(
            validate("foo-"),
            Err(Error::InvalidName(
                "snap name cannot end with a dash".to_string()
            ))
        );
        assert_eq!(
            validate("f--oo"),
            Err(Error::InvalidName(
                "snap name cannot contain two consecutive dashes".to_string()
            ))
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
            Err(Error::InvalidName(
                "snap name must contain at least one letter".to_string()
            ))
        );

        // just name, with separator, missing instance key
        assert_eq!(
            sc_instance_name_validate("hello-world_"),
            Err(Error::InvalidInstanceKey(
                "instance key must contain at least one letter or digit".to_string()
            ))
        );

        // only separator and instance key, missing name
        assert_eq!(
            sc_instance_name_validate("_bar"),
            Err(Error::InvalidName(
                "snap name must contain at least one letter".to_string()
            ))
        );

        assert_eq!(
            sc_instance_name_validate(""),
            Err(Error::InvalidName(
                "snap name must contain at least one letter".to_string()
            ))
        );

        // third separator
        assert_eq!(
            sc_instance_name_validate("foo_bar_baz"),
            Err(Error::InvalidInstanceName(
                "snap instance name can contain only one underscore".to_string()
            ))
        );

        // too long, 52
        assert_eq!(
            sc_instance_name_validate("0123456789012345678901234567890123456789012345678901"),
            Err(Error::InvalidInstanceName(
                "snap instance name can be at most 51 characters long".to_string()
            ))
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

    #[test]
    fn test_sc_snap_component_validate() {
        assert_eq!(
            sc_snap_component_validate("snapname+compname", None),
            Ok(())
        );

        assert_eq!(
            sc_snap_component_validate("snap-name+comp-name", None),
            Ok(())
        );

        // check that we fail if the snap name isn't in the snap component
        assert_eq!(
            sc_snap_component_validate("snapname+compname", Some("othername")),
            Err(Error::InvalidComponent(
                "snap name in component must match snap name in instance".to_string()
            ))
        );

        assert_eq!(
            sc_snap_component_validate("snapname+compname", Some("othername_instance")),
            Err(Error::InvalidComponent(
                "snap name in component must match snap name in instance".to_string()
            ))
        );

        // component name should never have an instance key in it, so this should
        // fail
        assert_eq!(
            sc_snap_component_validate("snapname_instance+compname", Some("snapname_instance")),
            Err(Error::InvalidComponent(
                "snap name in component must use lower case letters, digits or dashes".to_string()
            ))
        );

        assert_eq!(
            sc_snap_component_validate("snapname_instance+compname", Some("snapname")),
            Err(Error::InvalidComponent(
                "snap name in component must use lower case letters, digits or dashes".to_string()
            ))
        );

        // check that we can validate the snap name in the snap component
        assert_eq!(
            sc_snap_component_validate("snapname+compname", Some("snapname")),
            Ok(())
        );
        assert_eq!(
            sc_snap_component_validate("snapname+compname", Some("snapname_instance")),
            Ok(())
        );

        let cases = [
            "snap-name+",
            "+comp-name",
            "snap-name",
            "snap-name+comp_name",
            "loooooooooooooooooooooooooooong-snap-name+comp-name",
            "snap-name+loooooooooooooooooooooooooooong-comp-name",
        ];

        for case in cases {
            // TODO assert specific error enum with any value
            assert!(sc_snap_component_validate(case, None).is_err());
        }
    }

    #[test]
    fn test_sc_snap_component_validate_respects_error_protocol() {
        assert_eq!(
            sc_snap_component_validate("hello world+comp name", None),
            Err(Error::InvalidComponent(
                "snap name in component must use lower case letters, digits or dashes".to_string()
            ))
        );
    }
}
