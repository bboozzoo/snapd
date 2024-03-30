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
use std::fs::File;
use std::io::{self, BufRead, BufReader, Result};
use std::path::Path;

#[derive(Debug, Copy, Clone, PartialEq)]
pub enum DistroKind {
    Core16,
    CoreOther,
    Classic,
}

pub fn classify(os_release: &Path, expected_meta_snap_yaml: &Path) -> Result<DistroKind> {
    let f = File::open(os_release);
    if let Ok(f) = f {
        let mut is_core = false;
        let mut core_version = 0;
        for l in BufReader::new(f).lines() {
            match l {
                Ok(s) => {
                    match s.as_str() {
                        r#"ID="ubuntu-core""# | r#"ID=ubuntu-core"# => is_core = true,
                        r#"VERSION_ID="16""# | r#"VERSION_ID=16"# => core_version = 16,
                        r#"VARIANT_ID="snappy""# | r#"VARIANT_ID=snappy"# => is_core = true,
                        _ => continue,
                    };
                }
                Err(err) => return Err(err),
            }
        }
        if !is_core && expected_meta_snap_yaml.exists() {
            // reading /etc/os-release was inconclusive, let's see if
            // /meta/snap.yaml exists, if so we're in a snap
            is_core = true
        }
        if is_core {
            if core_version == 16 {
                Ok(DistroKind::Core16)
            } else {
                Ok(DistroKind::CoreOther)
            }
        } else {
            Ok(DistroKind::Classic)
        }
    } else {
        eprintln!("error {}", f.err().unwrap());
        Err(io::Error::new(io::ErrorKind::Other, "not implemnted"))
    }
}

pub fn is_debian_like() -> Result<bool> {
    Err(io::Error::new(io::ErrorKind::Other, "not implemnted"))
}

#[cfg(test)]
mod tests {
    use std::io::Write;
    use std::path::PathBuf;

    use super::*;
    use tempfile::{tempdir, TempDir};

    use std::println as info;

    #[derive(Debug)]
    struct FilesFixture {
        os_release: PathBuf,
        meta_snap_yaml: PathBuf,
        // FIXME how to keep td so that the directory isn't removed?
        td: TempDir,
    }

    fn mock_files(os_release: Option<&str>, meta_snap_yaml: Option<&str>) -> FilesFixture {
        let td = tempdir().unwrap();
        let ff = FilesFixture {
            os_release: td.path().join("os-release-mock"),
            meta_snap_yaml: td.path().join("meta-snap-yaml"),
            td,
        };
        if let Some(content) = os_release {
            File::create(&ff.os_release)
                .unwrap()
                .write_all(content.as_bytes())
                .unwrap();
        }
        if let Some(content) = meta_snap_yaml {
            File::create(&ff.meta_snap_yaml)
                .unwrap()
                .write_all(content.as_bytes())
                .unwrap();
        }
        info!("mocked paths: {:?}", ff);
        ff
    }

    #[test]
    fn test_is_on_classic() {
        let os_release_classic = r#"
NAME=Ubuntu"
VERSION="17.04 (Zesty Zapus)"
ID=ubuntu
ID_LIKE=debian
"#[1..]
            .as_ref();

        let ff = mock_files(Some(os_release_classic), None);
        let res = classify(&ff.os_release, &ff.meta_snap_yaml);
        assert_eq!(res.unwrap(), DistroKind::Classic)
    }
}
