# TODO: Enable driver-libs interfaces on Ubuntu Core

> **Status:** Phase 1 has landed. **Direction chosen: Option C — a dedicated `export`
> security backend** that builds an export tree, with `snap-confine` exposing it in the
> snap's mount namespace. Options A, B and the `snap-update-ns` alternatives were
> explored and set aside; their rationale is kept so the ground isn't re-covered.
>
> **All design questions are now settled** — see
> [Design decisions](#design-decisions-formerly-open-questions). Implementation can
> begin.
>
> **Two properties are requirements**, both pointing at a frozen, atomically-committed
> snapshot:
> - **Set atomicity** — a consumer must never observe a half-applied set.
> - **Cross-component consistency** — libraries reach the snap as *bind mounts of a
>   specific revision* and are therefore frozen for the namespace's lifetime. The
>   metadata describing those libraries is delivered the same way, so both halves share
>   one lifetime.
>
> **Known accepted limitation (Q14):** a newly connected driver is not visible to an
> already-running snap; snapd does **not** discard or rebuild a consumer's namespace when
> the driver set changes. This is the `snap-discard-ns` TODO in
> `tests/main/nvidia-userspace-libs/task.yaml`. Deliberately **out of scope** — newly
> connected drivers require an app restart.
>
> **The "unverified assumption" below is resolved, not just checked off** — see
> [Unverified assumption](#unverified-assumption--needs-checking-before-implementation).

---

## Background

The following six interfaces expose GPU/compute driver libraries to the system:

- `opengl-driver-libs`
- `egl-driver-libs`
- `cuda-driver-libs`
- `gbm-driver-libs`
- `opengles-driver-libs`
- `vulkan-driver-libs`

On **classic**, these interfaces activate three backends when a slot connects:

1. **ldconfig** – writes `/etc/ld.so.conf.d/snap.system.conf` (runtime linker)
2. **symlinks** (egl, gbm, vulkan only) – symlinks into `/etc/glvnd/egl_vendor.d/`,
   `/usr/lib/<arch>/gbm/`, `/etc/vulkan/icd.d/`, `/etc/vulkan/implicit_layer.d/`,
   `/etc/vulkan/explicit_layer.d/`
3. **configfiles** – writes
   `/var/lib/snapd/export/system_<snap>_<slot>_<iface>.library-source`
   (consumed by `snap-confine` and `snap-exec` at runtime)

On **Ubuntu Core**, backends 1 and 2 are meaningless (no host ldcache, no host
`/etc/glvnd/` or `/etc/vulkan/` trees). Backend 3 is the right *shape* for Core, but the
`.library-source` file only exposes **library directories** — it does not carry the
**ICD/layer configuration JSON files** that EGL and Vulkan consumers need. Supplying
those on Core is the open problem this document tracks.

### Why the ICD files can't just be pointed at

`snap-confine` bind-mounts snap library directories under
`/var/lib/snapd/lib/system/gpu/<snap>/<rev>/<subpath>/` via `sc_mount_exported_paths()`.
For the *config* files, on classic it reads them out of `/etc/glvnd/` and `/etc/vulkan/`
(symlinks planted by the symlinks backend) and copies them into tmpfs mounts. On Core
those `/etc/` symlinks don't exist, so a new delivery mechanism is required.

### `library_path` is a bare filename — this matters a lot

`checkEglIcdFile` / `checkVulkanIcdFile` enforce that `library_path` is a **bare
filename** (e.g. `libEGL_nvidia.so.0`), never an absolute path — the code comment says
*"we are implicitly limiting library_path to be a file name instead of a full path."*

At runtime the EGL (libglvnd) and Vulkan loaders resolve that bare name via `dlopen`,
searching `LD_LIBRARY_PATH` → `SNAP_LIBRARY_PATH` → the `gpu/<snap>/<rev>/<subpath>/`
bind-mounts. So library discovery (`.library-source` + `buildLibPath`) and ICD config
discovery are **complementary and independent**:

- the JSON names a library,
- the library search path makes it findable.

**Consequence:** the exported ICD content is *revision-independent*. This is why
revision-scoping the exported config buys nothing on correctness grounds, and is
central to the design discussion below.

---

## Design evolution: options considered

### Option A — `configfiles` + per-connection directories · ❌ REJECTED

Layout: `/var/lib/snapd/export/data/system_<snap>_<slot>_<iface>/<subdir>/`, one
directory per connection. Required extending `configfiles.Specification` with a
`connectedPathPatterns []string` field and an `AddConnectedPathPattern()` method,
plus matching changes in `ensureConfigfiles()`.

**Why rejected — cleanup-on-disconnect bug.** `ConfigfilesConnectedPlug` only runs for
*currently connected* slots. On disconnect, that connection's directory-specific pattern
would never be re-registered on the next `Setup()`, so `EnsureDirState` would never
revisit the directory and the stale ICD JSON files would remain on disk **permanently**.

The bug was found by comparing against the **symlinks** backend, which solves the same
problem with fixed, interface-level `TrackedDirectories()` re-scanned on every `Setup()`
regardless of what is connected.

### Option B — `configfiles` + flat per-interface pool · ⚠️ SUPERSEDED

Layout: `/var/lib/snapd/export/data/system_<iface>/<subdir>/<encoded-name>.json`, one
fixed directory per interface, provider identity encoded in the filename using the
existing `symlinksForSourceDir` scheme
(`<priority>_snap_<instance>[+<comp>]_<slot>_<escapedPath>`).

This fixed Option A's bug: because the pattern is fixed and interface-level, it is
always registered, so `EnsureDirState` always reconciles the directory. It also needed
**no changes to `interfaces/configfiles/` at all** — `PathPatterns()` could express it
as ordinary static globs.

Strong property: the encoded name contains **no revision**, so a refresh rewrites the
*same filename* with new content via atomic rename → a reader sees old-or-new, never
both, never neither. The refresh race disappears rather than being managed.

**Why superseded — no *set* atomicity.** Per-file atomic replace still lets a reader
observe 3 new files + 2 old ones mid-update. A Vulkan driver shipping an ICD plus an
implicit layer that must agree would be corrupted by such a partial view. For a
general-purpose export primitive this is the wrong guarantee to bake in.

**Lessons retained from Option B** (still apply to any design):

- *If* a design registers cleanup patterns, they must be registered **unconditionally**
  (classic *and* core) even when content is only written on core. Gating pattern
  registration on `release.OnClassic` silently reintroduces Option A's bug.
- `osutil.EnsureDirStateGlobs` (`osutil/syncdir.go:132-142`) **hard-errors** if any
  content basename fails to match the registered glob — an aborted setup, not a silent
  skip. Any generated-filename scheme needs a test asserting it matches its own pattern.
  (Verified safe for the encoded names: `systemd.EscapeUnitNamePath` keeps `.` and maps
  `/`→`-`, and `sourceDirFilesCheck` only accepts `.json` sources.)

### Option C — dedicated `export` security backend · ✅ CHOSEN DIRECTION

A new backend that copies declared files out of a snap (or its components) into a
staging tree under `/var/lib/snapd/export/`, organised per interface, with a
**manifest** naming the currently-valid set. `snap-confine` reads the manifest and pools
the content into the snap's mount namespace.

**What it buys over Option B:**

- **Set atomicity** — content is materialised into a fresh per-unit directory, then the
  manifest is flipped in one atomic write. Readers see the complete old set or the
  complete new set, never a mixture.
- **Safe deferred GC** — because the manifest already excludes a stale directory,
  nothing will newly reference it; removing it becomes pure disk reclamation with no
  correctness pressure. Option B cannot defer, since replacement *is* the update.
- **Clear ownership** — `configfiles` is documented as *"modifies the classic rootfs"*;
  a staging area for `snap-confine` is a different concern. A dedicated backend owns
  `/var/lib/snapd/export/**` outright, enabling safe GC of unrecognised entries, and
  gets its own `SandboxFeatures()` tag.
- **Different reconciliation model** — `EnsureDirState` is built for a flat directory of
  files; a directory-plus-manifest layout needs hand-rolled reconciliation, which is
  itself an argument for not bolting this onto `configfiles`.
- **Frozen delivery, consistent with libraries** — content is copied into a tmpfs at
  namespace creation, so it shares the lifetime semantics of the bind-mounted libraries
  (see *Cross-component consistency* below). Options D and E fail on exactly this.

**Cost:** a new backend in the `Setup()` loop for every snap; more code and tests. If
GPU ICDs remain the only consumer, a ~20-line helper on `configfiles` (taking a source
*path* and doing the read) would cover it. The case for the backend rests on wanting a
general "export files from a snap" primitive — which is the direction being taken.

### Option D — `snap-update-ns` mount entries · ❌ REJECTED

Explored as an alternative to `snap-confine` doing the work: have the **mount** backend
emit fstab entries so `snap-update-ns` binds the export content into the namespace.

The machinery genuinely supports it:

- **`x-snapd.kind=file`** bind-mounts individual regular files (`change.go:117-132`,
  `MkfileAll`); `x-snapd.kind=symlink` creates symlinks.
- **`x-snapd.ignore-missing`** turns a missing source into a silent no-op
  (`change.go:95-97`) — already used by `network_control` and `desktop`.
- **Non-layout entries fail soft** — logged and skipped, the rest of the profile still
  applies (`update.go:181-196`).
- **`UpdateSnapNamespace`** re-applies profiles to *already-running* snaps whenever the
  fstab content changes (`interfaces/mount/backend.go:97-135`).
- **`ExtraLayouts`** (`interfaces/backend.go:68-73`) lets snapd inject layouts that
  bypass snap.yaml validation — journal quota already binds from `/run/systemd/…`,
  i.e. outside `$SNAP*`, proving the escape hatch works.

**Why rejected — it breaks cross-component consistency.** A directory bind mount is a
*live* view of the source inode, so metadata would track the current export state while
the libraries stay pinned to the revision captured at namespace creation. That is not a
transient window; it is a **persistent mismatch lasting until the namespace is rebuilt**
(see *Cross-component consistency*).

Secondary problems:

- **The liveness win is illusory anyway.** The `snap-discard-ns` TODO is about a *newly
  connected* driver being invisible. Live metadata does not fix it: the new driver's
  *libraries* are not bind-mounted into the running namespace, so the loader reads the
  new ICD, `dlopen`s a library that isn't there, and skips it. Different failure path,
  same outcome.
- The consumer snap does **not** appear to be in `affectedSnaps` when a *driver*
  interface connects (the connection is system↔driver), so its profile would never be
  regenerated — meaning per-file entries wouldn't update live either.
- `snap-confine` remounts the `gpu/` tmpfs **read-only** before `snap-update-ns` runs, so
  an entry targeting inside it would trigger writable-mimic construction over a tmpfs
  snapd just created.
- Mount-point clashes are silently renamed to `-2`, `-3` (`interfaces/mount/spec.go:310-314`)
  — a real hazard for any per-file scheme where two providers ship the same filename.
- Every mount needs a matching per-snap AppArmor snippet via `spec.AddUpdateNSf(...)`;
  the `snap-update-ns` base template grants no general mount permission.

### Option E — direct access, AppArmor grant only · ❌ REJECTED

The minimal variant: no mount at all. `/var/lib/snapd` is **already** bind-mounted into
every strict-mode namespace, recursive-slave (`cmd/snap-confine/mount-support.c:913`), so
`/var/lib/snapd/export/…` is visible at the same path inside the snap and host changes
propagate live. All that's missing is an AppArmor rule on the consumer.

**Why rejected:** same fatal flaw as Option D — live metadata over frozen libraries. It
additionally turns a snapd-internal staging path into a public contract that consumers
would hardcode, with no curated boundary.

Recorded because it is by a wide margin the least code, and worth reconsidering only if
the consistency requirement is ever dropped.

### Cross-component consistency — the property that decided C over D/E

Libraries reach the snap via `sc_mount_exported_paths()`:

```c
sc_do_mount(path, dest_path, NULL, MS_BIND | MS_RDONLY, NULL);
```

where `path` is a **revision-specific** directory read from `.library-source` (e.g.
`/snap/foo/33/libs`). A bind mount pins the inode, so a running namespace keeps that
revision's libraries even after the host unmounts `/snap/foo/33`. **Libraries are frozen
for the namespace's lifetime.**

Freezing is not incidental — it is *correct*. A shared object cannot be safely
hot-swapped under a running process: existing mappings stay bound to the old inode while
new `dlopen`s would pick up the new one, giving one process a mixed driver stack.
Staleness is strictly better.

The metadata **describes those exact libraries**. An ICD naming `libEGL_nvidia.so.0` is
only meaningful relative to the library set it shipped with. Letting the description
float while the thing described is pinned is the inconsistency:

| | Libraries | Metadata | Result |
|---|---|---|---|
| **C** copy at ns creation | rev 33 (pinned) | rev 33 (frozen) | consistent |
| **D** dir bind, live | rev 33 (pinned) | rev 34 (live) | **mismatched until rebuild** |
| **E** direct access | rev 33 (pinned) | rev 34 (live) | **mismatched until rebuild** |

That this rarely bites today — because `library_path` is a stable SONAME
(`libEGL_nvidia.so.0`, not a version-qualified filename) — is luck, not design. It is
exactly the latent inconsistency that surfaces the one time a driver changes SONAME
across revisions.

**Consequence:** the namespace is the unit of consistency. Everything within it comes
from one generation; picking up a new generation means rebuilding it. See Q14.

### On atomicity: what classic does, and why Core may warrant more

Classic's host-driver path does **not** provide atomicity, and it is worth recording why
that does not settle the question.

`sc_populate_libgl_with_hostfs_symlinks()` creates **symlinks into
`/var/lib/snapd/hostfs`** — for regular files and for absolute symlink targets alike:

```c
case S_IFLNK:
    if (hostfs_symlink_target[0] == '/') {
        sc_must_snprintf(symlink_target, ..., "/var/lib/snapd/hostfs%s", hostfs_symlink_target);
case S_IFREG:
    sc_must_snprintf(symlink_target, ..., "/var/lib/snapd/hostfs%s", pathname);
```

`sc_mkdir_and_mount_and_glob_files()` — tmpfs + that symlink farm + remount-ro — is used
by the **primary** multiarch branch (driver found in the standard `/usr/lib/<triplet>`,
the common Ubuntu case) *and* by `sc_mount_vulkan()` / `sc_mount_egl()`. So on classic
with host-packaged NVIDIA, **both libraries and ICD configs are symlinks into live host
state**. Only the fallback branch (`/usr/lib/nvidia`, via `sc_mkdir_and_mount_and_bind`)
pins with a real bind mount.

An `apt upgrade nvidia-driver-*` unlinks those files and the symlinks dangle — for
already-running snaps, mid-flight. The failure mode is survivable and well-understood:
`dlopen` fails, the loader skips that ICD, the driver is unavailable until restart.

**This calibrates severity; it does not dispose of the requirement.** Core plausibly
warrants a higher bar:

- **Refreshes are unattended.** `apt upgrade` is a deliberate act by someone who knows
  they just touched the driver stack. Core auto-refresh fires on a timer with nobody
  watching.
- **It is the platform's model.** Core is transactional end to end — atomic refreshes,
  rollback, A/B boot assets. Half-applied driver config violates the invariant
  everything else maintains.
- **Classic's behaviour is a constraint, not a choice.** snapd doesn't own the host's
  packages there and can only point at what dpkg leaves behind. On Core it owns the
  whole stack, so "we couldn't do better there" does not imply "we shouldn't here."
- **The failure surface differs.** Desktop: restart the app. Unattended appliance: a
  driver that fails to load at 4am is an outage until someone notices.


---

## Current design (Option C)

> All design questions are settled — see [Design decisions](#design-decisions-formerly-open-questions).
> Cross-references below point at the decision that fixed each choice.

### Layout

Shown with the **leading candidate** for unit granularity/naming (per-container — see
*Unit granularity and naming* below; decided per Q2/Q6):

```
/var/lib/snapd/export/system/<iface>/            # see Q4 — hierarchical; `system` is a reserved snap name
    <snap>_<slot>_<snaprev>/                     # content from $SNAP
        egl_vendor.d/<encoded-name>.json
    <snap>_<slot>_<snaprev>+<comp>_<comprev>/    # content from exactly one component
        egl_vendor.d/<encoded-name>.json
    export.sources                               # manifest, lists files (Q3)
```

A vulkan connection would additionally carry `icd.d/`, `implicit_layer.d/` and
`explicit_layer.d/` inside each unit.

`system/` denotes *"plug side is the system snap"* — the same meaning the `system_`
prefix carries in today's `.library-source` names. Because it is stated once at the
parent level, it is **not** repeated on the child directories.

**Per-interface grouping** replaces the flat `system_<snap>_<slot>_<iface>.*` naming.
The forward-compatibility argument: every one of these interfaces carries a comment
anticipating non-system plugs (*"in the future we will allow snaps having this as
plug"*), and a hierarchy extends naturally to `<plug-snap-instance>/<iface>/`.

### The manifest (`export.sources`)

Plain text, one entry per line — deliberately the same shape as `.library-source`, which
is already parsed by `fgets`/`sc_str_chomp` in `snap-confine` and `bufio.Scanner` in
`snapenv.buildLibPath()`. Near-zero marginal parsing cost on the C side.

**Entries are file paths** relative to the interface root, of the form
`<unit>/<subdir>/<file>` — see the worked example under *Delivery into the snap's mount
namespace*, which shows why this beats listing directories (Q3).

The manifest is the **atomic commit point**: stale directories may linger on disk, but
the manifest always names exactly the current set.

**Critical invariant:** the manifest must never list two units for the same
(snap, slot, container) simultaneously — that would register the same driver twice, and
duplicate ICDs cause duplicate device enumeration in the Vulkan/EGL loaders, which is
worse than a missing driver. This appears to hold naturally: the repository holds one
connection per (plug, slot) resolved against the slot snap's *current* revision, so any
single `Setup()` sees exactly one revision per snap and one per component. Transient
double-*directories* on disk are fine; double-*manifest-entries* are not.

### Encoded filenames are still required inside each unit directory

The per-unit directory does **not** remove the need for
`<priority>_snap_<instance>[+<comp>]_<slot>_<escapedPath>` naming, because:

- **EGL priority ordering is global** across all providers, so the numeric prefix must
  survive into whatever flat directory the consumer ultimately scans;
- two components of the same snap can both ship `vendor.json` → collision within one
  unit directory.

The unit directory is an atomicity/GC boundary, not a namespace that simplifies naming.

### Delivery into the snap's mount namespace

Content from the manifest-listed units is **pooled** into a flat directory per
subdirectory kind:

```
/var/lib/snapd/lib/system/gpu/system/<iface>/<subdir>/<encoded-name>.json
```

#### `system/` is safe here — it is a reserved snap name

`sc_mount_exported_paths()` already writes `gpu/<snap-instance>/<rev>/…`, so `gpu/`
holds instance-named entries. A `gpu/system/` directory cannot collide with them,
because **no snap can ever be named `system`** — this is enforced at install time, not
merely by convention:

```go
// overlord/snapstate/snap.go:828-830, in checkInstallPreconditions
if snapsup.InstanceName() == "system" {
    return fmt.Errorf("cannot install reserved snap name 'system'")
}
```

The check is against `InstanceName()`, so parallel installs such as `system_foo` are
blocked too. It is reinforced at the API layer, where `"system"` is a *nickname*
remapped to the real system snap — `CoreSnapdSystemMapper.RemapSnapFromRequest()` →
`"snapd"`, or `CoreCoreSystemMapper` → `"core"` (`overlord/ifacestate/helpers.go:1476`,
`:1523`) — so it never appears as a real name in state or in memory either.

Consequences:

- The underscore form `gpu/system_<iface>/` is **not** needed; both sides of the
  pipeline use the same hierarchical shape.
- The `gpu/libs/` + `gpu/data/` split floated in an earlier pass as collision-avoidance
  is **unnecessary and dropped**. `gpu/<snap>/<rev>/…` (libraries) and
  `gpu/system/<iface>/…` (config) are cleanly disjoint.
- It strengthens Q4's forward-compat story: when non-system plugs arrive,
  `gpu/<plug-snap-instance>/<iface>/` slots in beside `gpu/system/<iface>/` without
  ambiguity. **The reserved name is the namespace separator.**

`/var/lib/snapd/lib/system/gpu/system/<iface>/` does repeat `system`, since
`SC_LIBGPU_DIR` is already `…/lib/system/gpu`. That is cosmetic; the explicit `system/`
level is what carries the "plug side is the system snap" meaning and keeps the pooled
tree parallel with the export tree.

#### Worked example

Interface `vulkan-driver-libs`; snap `test-nvidia-interfaces` rev 33, slot `vulkan-dl`,
plus a component `comp1` rev 12 contributing an extra ICD. (Encoded names below are the
real ones produced by `symlinksForSourceDir`, as asserted by the landed classic test
`tests/main/nvidia-userspace-libs/test-nvidia-libs/test`.)

**Export tree** — per-container units:

```
/var/lib/snapd/export/system/vulkan-driver-libs/
    test-nvidia-interfaces_vulkan-dl_33/
        icd.d/
            snap_test-nvidia-interfaces_vulkan-dl_usr-share-vulkan-icd.d-nvidia_icd.json
        implicit_layer.d/
            snap_test-nvidia-interfaces_vulkan-dl_usr-share-vulkan-implicit_layer.d-nvidia_layers.json
    test-nvidia-interfaces_vulkan-dl_33+comp1_12/
        icd.d/
            snap_test-nvidia-interfaces+comp1_vulkan-dl_vulkan-icd.d-extra_icd.json
    export.sources
```

**`export.sources`** — paths relative to the interface root, one per line:

```
test-nvidia-interfaces_vulkan-dl_33/icd.d/snap_test-nvidia-interfaces_vulkan-dl_usr-share-vulkan-icd.d-nvidia_icd.json
test-nvidia-interfaces_vulkan-dl_33/implicit_layer.d/snap_test-nvidia-interfaces_vulkan-dl_usr-share-vulkan-implicit_layer.d-nvidia_layers.json
test-nvidia-interfaces_vulkan-dl_33+comp1_12/icd.d/snap_test-nvidia-interfaces+comp1_vulkan-dl_vulkan-icd.d-extra_icd.json
```

**Pooled result** in the snap's mount namespace — two units collapse into one `icd.d/`:

```
/var/lib/snapd/lib/system/gpu/system/vulkan-driver-libs/
    icd.d/
        snap_test-nvidia-interfaces_vulkan-dl_usr-share-vulkan-icd.d-nvidia_icd.json   ← from $SNAP
        snap_test-nvidia-interfaces+comp1_vulkan-dl_vulkan-icd.d-extra_icd.json        ← from comp1
    implicit_layer.d/
        snap_test-nvidia-interfaces_vulkan-dl_usr-share-vulkan-implicit_layer.d-nvidia_layers.json
```

No collisions are possible: the encoded names already carry snap, component and slot.

#### Delivery is per-file bind mounts (Q7)

Each manifest entry becomes an individual **read-only bind mount**, mirroring what
`sc_mount_exported_paths()` already does for library directories:

```c
for each line in export.sources:
    rel = line;                          /* "<unit>/<subdir>/<file>" */
    sub = strchr(rel, '/') + 1;          /* "<subdir>/<file>"  — strip the unit */
    src = "/var/lib/snapd/export/system/<iface>/" + rel;
    dst = rootfs + "/var/lib/snapd/lib/system/gpu/system/<iface>/" + sub;

    if (exists(dst)) continue;           /* already mounted — skip */
    mkpath(dirname(dst));
    create_empty_file(dst);              /* placeholder: cannot bind a file onto a dir */
    sc_do_mount(src, dst, NULL, MS_BIND | MS_RDONLY, NULL);   /* skip on ENOENT */
```

Strip the first path component, keep the rest. Properties:

- **Same mechanism as libraries.** Libraries arrive as `MS_BIND|MS_RDONLY` mounts of
  revision-specific directories; metadata now arrives the same way. The
  frozen-for-the-namespace-lifetime property is therefore *uniform* rather than
  coincidental — see *Cross-component consistency*.
- **Pinning makes GC safe.** A bind mount holds the source inode, so collecting a unit
  cannot pull content out from under a running snap. This is strictly stronger than
  copying.
- **No `readdir` on the read path** — a unit collected mid-setup can only ever yield
  `ENOENT` on an individual file (skip it), never a partially-enumerated directory.
- **`snap-confine` needs no per-interface knowledge** — no hardcoded
  `icd.d`/`egl_vendor.d` table. New subdirectory kinds work with zero C changes.

**Implementation notes:**

- `sc_mount_exported_paths()` only ever creates *directories*
  (`sc_nonfatal_mkpath(dest_path, …)`). Files need `mkpath(dirname)` **plus an empty
  placeholder file** — you cannot bind-mount a file onto a directory.
- Ordering within the tmpfs: create placeholders → bind each file → remount the tmpfs
  read-only once at the end.
- Mount count scales with the number of exported files (small for ICDs — single digits
  per interface in practice).
- The "skip if target exists" check mirrors the existing dedup in
  `sc_mount_exported_paths()`; with encoded filenames a genuine collision shouldn't
  occur, so it acts as a safety net.

> **Superseded:** an earlier sketch had `snap-confine` *copy* files into the tmpfs
> (mirroring `sc_copy_glob_files()` on classic). Bind-mounting is better on both counts
> above and keeps a single delivery mechanism for libraries and metadata.

#### Classic precedent

This is the same shape classic already uses — flat pooled directories holding
encoded-name files from any provider, as asserted by the landed test:

```bash
/var/lib/snapd/lib/glvnd/egl_vendor.d/10_snap_test-nvidia-interfaces_egl-dl_usr-share-glvnd-egl_vendor.d-nvidia.json
/var/lib/snapd/lib/vulkan/icd.d/snap_test-nvidia-interfaces_vulkan-dl_usr-share-vulkan-icd.d-nvidia_icd.json
```

Note classic pools by **kind** (`glvnd/`, `vulkan/`), mirroring the standard system
layout rehomed under a snapd root, whereas the Core scheme pools by **interface**. Both
work; interface-level is more explicit about provenance and prevents two interfaces
silently merging into one pool, kind-level would keep Core and classic search paths
parallel. Decided **by interface** — see Q12.

#### Implementation constraint: tmpfs ordering

`sc_mount_exported_paths()` mounts a tmpfs at `gpu/`, does its bind mounts, then
**remounts it read-only** before returning (`sc_remount_ro(tmpfs_path)`).
`sc_copy_glob_files()` avoids this on classic by mounting its *own* tmpfs at `glvnd/` /
`vulkan/` — three independent tmpfs.

The Core pool target sits *inside* `gpu/`, so config pooling must happen **before** that
`sc_remount_ro()`, or use a separate tmpfs whose mountpoint is created beforehand.
Cleanest is to restructure so all writes into `gpu/` complete first, with a single
remount-ro at the end.

#### AppArmor

**Snap side:** already permitted — `interfaces/builtin/opengl.go` grants
`/var/lib/snapd/lib/system/gpu/{,**} rm`, and `**` covers the new subdirectories. **No
snap-side AppArmor change needed.**

**`snap-confine` side:** needs read access to the export tree at full depth plus a mount
rule for the new bind source. Note the scratch rootfs prefix is
`/tmp/snap-private-tmp/snap.rootfs_*`, **not** `/tmp/snap.rootfs_*`:

```
# read the manifest and the exported files
/var/lib/snapd/export/system/{,**} r,
# bind-mount exported files into the pooled tree
mount options=(ro bind) /var/lib/snapd/export/system/** -> /tmp/snap-private-tmp/snap.rootfs_*/var/lib/snapd/lib/system/gpu/system/**,
```

The existing `/tmp/snap-private-tmp/snap.rootfs_*/var/lib/snapd/lib/system/gpu/{,**} w,`
already covers creating the placeholder files and their parent directories inside the
tmpfs.

#### Why not reuse the classic targets

`/var/lib/snapd/lib/glvnd/egl_vendor.d/` and `/var/lib/snapd/lib/vulkan/icd.d/` are the
classic mechanism and on classic hold a *mix* of host content (via `sc_mount_egl` /
`sc_mount_vulkan` from `/usr/share/`) and snap content (via `sc_copy_glob_files`).
Reusing them on Core would blur provenance; Core gets its own target. Per Q1 the
consumer scans whichever location(s) apply, so this imposes no constraint on us.

#### Consumer contract — what the base snap must do

**snapd sets none of the relevant loader environment variables.** Verified: no
occurrence of `VK_ICD_FILENAMES`, `VK_DRIVER_FILES`, `VK_LAYER_PATH`, `VK_ADD_*` or
`__EGL_VENDOR_*` anywhere in the tree. On classic, discovery works because the symlinks
backend plants files in standard paths the loaders scan by default, and the classic snap
rootfs *is* the host filesystem. On Core neither holds, so the **consuming base snap
(e.g. mesa-2604) must point its loaders at the pooled directories.**

| Pooled directory | Loader variable | Takes |
|---|---|---|
| `…/system/<iface>/egl_vendor.d/` | `__EGL_VENDOR_LIBRARY_DIRS` | directories ✓ |
| `…/system/<iface>/icd.d/` | `VK_DRIVER_FILES` (was `VK_ICD_FILENAMES`), or `VK_ADD_DRIVER_FILES` | manifest **files**; directory support is loader-version-dependent |
| `…/system/<iface>/explicit_layer.d/` | `VK_LAYER_PATH` / `VK_ADD_LAYER_PATH` | directories ✓ |
| `…/system/<iface>/implicit_layer.d/` | `VK_IMPLICIT_LAYER_PATH` | directories, but **only on newer loaders** (≈1.3.234+) |

Note `VK_LAYER_PATH` covers *explicit layers only* and does **not** pick up ICDs — the
two are commonly conflated. EGL is the clean case: `__EGL_VENDOR_LIBRARY_DIRS` takes
directories, so pointing it at `…/egl_vendor.d/` works directly.

The two loader capabilities that need confirming against the base actually shipping are
tracked as **Q13**; (b) in particular could leave implicit layers undiscoverable on Core.

This is an external change to the base snap, not a snapd code change. The `opengl`
interface AppArmor already grants `gpu/{,**} rm` so the consuming snap can read the
files, and `library_path` remains a bare filename resolved via `dlopen` →
`SNAP_LIBRARY_PATH` → the `gpu/<snap>/<rev>/` bind-mounts, so no additional library
discovery mechanism is needed.

### Write sequence (crash- and reader-safe)

1. Compute the full desired state from the spec (all connections — see *Full-state
   reconciliation* below).
2. For each unit not already present, materialise into `<unit>.tmp/`, then `rename()`
   into place. **Directory rename is atomic**; writing N files into a directory is not,
   so a reader could otherwise observe a half-populated unit.
3. Flip `export.sources` atomically (write-temp-and-rename, as `EnsureFileState` does).
4. GC units not listed in the manifest.

Crash safety falls out: a crash between 2 and 3 leaves an unreferenced orphan collected
next pass; a crash between 3 and 4 leaves stale units collected next pass.

### GC

**Rule:** *any directory under `<iface>/` not listed in `export.sources` is garbage.*

This single rule is complete — it covers disconnect, snap removal, refresh, revert,
component refresh, and crashed-mid-operation leftovers, without needing to know why
something became unreferenced.

**Timing — decided: collection is immediate, and readers must be resilient.**

Collection happens in the same `Setup()` pass that flipped the manifest (step 3 of the
write sequence above), not deferred to a later pass and not held back by a grace period.

The consequence is a race a reader can observe: having read the manifest, it may find a
file it lists already gone, or a unit being removed while it walks it. **Readers are
therefore required to skip files that cannot be found or cannot be read, rather than
treating either as fatal.** This is a contract of the export tree, not an implementation
detail, and it is recorded as such in `ensureExportsForInterface`'s doc comment.

This is the right side to solve it on. A reader must be resilient anyway — the files
live under `/var/lib/snapd/`, snapd is free to rewrite them at any moment for reasons
unrelated to GC, and no amount of deferral makes a read of a mutable tree safe. Deferral
would only narrow a window that still has to be handled correctly, in exchange for
carrying grace-period state, a boot-time sweep, and unbounded disk usage in the interim.

Directly implied requirement for Phase 3: `snap-confine`'s reader must not `die()` on a
missing or unreadable file. This is satisfied by construction if delivery uses
`sc_do_optional_mount()` (`cmd/libsnap-confine-private/mount-opt.h:77`) instead of
`sc_do_mount()` for each bind mount — it is documented to *"silently fail when mount
fails with ENOENT... carry on as if nothing had happened"*, which is exactly the
contract this section requires. No change to `sc_copy_file()` is needed for this:
that function is reachable only via `sc_copy_glob_files()`, called exclusively with
classic-only globs over `/etc/glvnd/` and `/etc/vulkan/` — it is never on the path that
reads `/var/lib/snapd/export/system/<iface>/`, since that tree is consumed by bind
mount, not by copy. (`sc_copy_file()` not `die()`ing on a missing source is still worth
fixing — see *`sc_copy_file()` dies on a missing source* below — but as a standalone
classic-side robustness fix, not as a precondition for anything in this section.)

> Earlier drafts of this document recommended the opposite (deferred GC at the start of
> the next `Setup()`, an mtime grace period and a boot-time sweep) while the *Write
> sequence* section immediately above already described same-pass collection — the two
> sections contradicted each other. The implementation followed the write sequence; this
> section was the stale half. Reference-counting against live namespaces would be
> formally correct but is disproportionate.

### Unit granularity and naming **[OPEN, see Q6 — leading candidate identified]**

This is the most-iterated part of the design. Two questions are entangled and must be
answered together:

- **Granularity** — is a "unit" one *connection* (snap + all its components merged), or
  one *container contribution* (the snap, or one component)?
- **Naming** — what makes a unit name unique, bounded, and stable?

#### What determines a unit's content

Verified against the tree:

- **Static slot attributes cannot be shadowed dynamically.** `ConnectedSlot.SetAttr()`
  (`interfaces/connection.go:301`) refuses: *"cannot change attribute %q as it was
  statically specified in the snap details"*. Since `icd-source`, `priority` etc. are
  declared in snap.yaml, they are **pinned by the snap revision** — no hook can vary
  them per connection. This is what makes revision-derived naming viable at all.
- **The slot carries its own snap's AppSet** — `NewConnectedSlot` panics otherwise — so
  `slot.AppSet().Components()` yields every contributing `*snap.ComponentInfo` with its
  `.Revision`. Every contributing revision is therefore enumerable at `Setup()` time
  **without reading a single file**.
- **Component paths resolve through the component's own revision**
  (`snap_app_set.go:109`, via `MinimalComponentContainerPlaceInfo`); uninstalled
  components are dropped while their index is preserved.

So: `snaprev` fixes *which* paths are scanned and the `priority`/`dirIdx` baked into
output filenames; each `comprev` fixes the *bytes* of that component's contribution.

#### Container origin is already encoded in the filenames

`symlinksForSourceDir` marks each file with the container it came from, via `compSuffix`:

```
15_snap_foo_egl-slot_egl.d-vendor.json          ← from $SNAP
16_snap_foo+comp1_egl-slot_egl.d-vendor.json    ← from comp1
17_snap_foo+comp2_egl-slot_egl.d-vendor.json    ← from comp2
```

The "an `icd.d/` holds a mix of snap and component content" case is therefore already
accounted for at the file level, whatever the directory granularity turns out to be.

**Crucially, this convention references at most one container per name — never an
aggregate.** That property is what keeps the existing names bounded.

#### ❌ Ruled out: `<snap>_<slot>_<snaprev>` alone

A **component-only refresh** (snap stays at 33, comp1 goes 11→12) leaves the name
unchanged while the content changes → forces an in-place rewrite → **set atomicity is
lost exactly where it is needed**.

#### ❌ Ruled out: revision tuple `<snap>_<slot>_<snaprev>[_<comp>-<comprev>]…`

Correct on content, and attractive because it is computable without I/O (so an existing
unit can be `stat`-ed and skipped). But it **aggregates every component into one name**,
and that breaks the length bound:

- Component names are validated by `ComponentRef.Validate()` → `ValidateSnap()` →
  **max 40 chars** (`snap/naming/validate.go:86`).
- **No limit on the number of components per snap** exists anywhere in `snap/` or
  `overlord/snapstate/`.

Each segment `_<comp>-<comprev>` costs ~48 bytes against `NAME_MAX` = 255:

| Name lengths | Components before overflow |
|---|---|
| Worst case (40-char snap/component names) | **~4** |
| Modest (10-char names) | ~16 |

A driver snap with per-GPU-family components plausibly reaches 5–10, so this is a real
defect, not a theoretical one. It also departs from the one-container-per-name
convention above.

#### ✅ Leading candidate: per-container units

```
<snap>_<slot>_<snaprev>                      ← content from $SNAP
<snap>_<slot>_<snaprev>+<comp>_<comprev>     ← content from exactly one component
```

Each name references **at most one container**, mirroring `compSuffix`, so it is bounded
by construction: worst case ≈ 51 (instance) + 40 (slot) + 7 (rev) + 41 (comp) + 7
(comprev) + separators ≈ **149 bytes**.

```
/var/lib/snapd/export/system/egl-driver-libs/
    foo_egl-slot_34/
        egl_vendor.d/15_snap_foo_egl-slot_egl.d-vendor.json
    foo_egl-slot_34+comp1_12/
        egl_vendor.d/16_snap_foo+comp1_egl-slot_egl.d-vendor.json
    foo_egl-slot_34+comp2_22/
        egl_vendor.d/17_snap_foo+comp2_egl-slot_egl.d-vendor.json
    export.sources
```

Set atomicity is preserved: the manifest lists all units for a connection and the flip
swaps the whole set at once.

Grouping cost is small — `symlinksForSourceDir` already derives the container from the
path (`strings.HasPrefix(path, snap.ComponentsBaseDir(instance))` plus the `splitNum`
3-vs-6 branch), and that logic should be factored out for the shared filename helper
regardless.

**Known wrinkle:** `snaprev` must appear in *component* unit names too, because the snap
revision decides which subdirectory of the component is scanned and what
`priority`/`dirIdx` its files receive. So a snap-only refresh renames every unit, even
for components whose bytes are unchanged. No worse than per-connection, but it forfeits
the churn saving that might otherwise motivate the split. See *Optional variant* below.

#### ⚪ Still viable: per-connection + content hash

`<snap>_<slot>_<hash>` — bounded regardless of component count, one unit per connection,
simplest reconciliation, and future-proof if the backend is ever used for content that
varies without a revision change (impossible for these interfaces today, per the static
attribute finding above).

Costs: opaque names, and all sources must be read and hashed **before** discovering
there is nothing to do — the revision-derived schemes allow a cheap `stat`-and-skip.

#### ❌ Ruled out: generation counters

- **Derived via `readdir`** — unsafe. Two concurrent `Setup()`s (possible; see
  *Concurrent `Setup()`* below) both scan, both see max = N, and both create `<unit>_N+1`
  with *different* content. Numbers are also reused after GC, so a stale manifest
  reference can resolve to different content.
- **Persisted in snapd state** — safe, but introduces interface-owned persistent state
  that must never regress across restarts or rollbacks, and forfeits idempotence: a
  revert allocates a fresh generation and rewrites byte-identical content.

Both deterministic schemes (per-container, hash) make **reverts free**: the recomputed
name matches a unit still on disk (if not yet collected) and is reused with zero writes.

#### Optional variant — ordering in the manifest, truly immutable units

If component-refresh churn turns out to matter, ordering can move out of the filenames
and into the manifest, making unit content keyed by container revision *alone*:

```
foo_34/egl_vendor.d/vendor.json          ← raw name, no priority prefix
foo+comp1_11/egl_vendor.d/vendor.json

export.sources:
    15 foo_34 egl_vendor.d
    16 foo+comp1_11 egl_vendor.d
```

`foo+comp1_11` then becomes genuinely immutable and is **reused across snap refreshes
with zero copying**. The cost is that filename construction moves into `snap-confine` —
C is the worse place for that logic, and it would diverge from how classic builds its
symlink names today. Recommend keeping names in Go unless churn proves to matter.

#### Comparison

| | Per-container | Per-connection + hash | Revision tuple | Generation |
|---|---|---|---|---|
| Bounded length | ✓ (~149 worst case) | ✓ (fixed) | ✗ **breaks at ~4 comps** | ✓ |
| One container per name (matches `compSuffix`) | ✓ | ✗ aggregate | ✗ aggregate | ✗ |
| Readable | ✓ | ✗ | ✓ | ✓ |
| Skip-without-I/O | ✓ | ✗ | ✓ | ✗ |
| Idempotent / revert-free | ✓ | ✓ | ✓ | ✗ |
| Concurrency-safe | ✓ | ✓ | ✓ | ✗ (readdir) / ✓ (state) |
| Persisted state | none | none | none | required (state form) |
| Units per connection | 1 + N | 1 | 1 | 1 |

---

## Findings that hold regardless of which option is chosen

These were verified against the tree and are design-independent.

### Full-state reconciliation is free

`repo.SnapSpecification(backend, appSet, opts)` for the **system snap** walks *every*
connection of its plugs, so a single `Setup()` yields the **complete desired state**, not
a delta. The `ldconfig` backend states this explicitly:

> *"We only need one file per plug … the specification is recreated with all the
> information even if we are refreshing only one of the snaps providing slots."*

Pooling across multiple connected slots therefore needs no special machinery.

### `Setup()` fires on the system snap for all the relevant events

`setupAffectedSnaps` (`overlord/ifacestate/handlers.go:114`) triggers it on connect,
disconnect, **and** when a connected slot-provider refreshes
(`SnapSetupReasonConnectedSlotProviderUpdate`, `handlers.go:553`).

### Concurrent `Setup()` on the system snap appears possible

`setupSecurityByBackend` **drops the state lock** for the whole backend loop
(`overlord/ifacestate/helpers.go:712-714`: `st.Unlock()` / `defer st.Lock()`), and
`Repository.SnapSpecification()` holds `r.m` only while *building* the spec — released
before the backend writes anything.

Connect/disconnect are serialised by
`CheckChangeConflictMany(st, []string{plugSnap, slotSnap})`, which always includes the
system snap for these interfaces. But two independent snap **refreshes** both reach
`setupAffectedSnaps` → `setupSnapSecurity(system)`, and no conflict check or mutex was
found covering that path.

Implications:

- Deterministic unit naming (revision tuple / hash) makes this **benign** — identical
  inputs produce identical names, so a duplicate create is a no-op; differing inputs
  produce different names, so there is no collision.
- `readdir`-derived counters are **hazardous** under it.
- The residual "last writer wins with a stale spec" race is **pre-existing** and already
  affects `ldconfig`, `configfiles` and `symlinks`. Out of scope here, but worth knowing.

### `sc_copy_file()` dies on a missing source — latent bug, reachable today, classic-only

`cmd/snap-confine/mount-support-nvidia.c`:

```c
if ((fd_in = open(src, O_RDONLY)) == -1) {
    die("cannot open %s", src);
}
```

A file removed between `glob()` and `open()` **kills the snap launch**. This is
reachable on classic *today* if a `/etc/glvnd` symlink is removed mid-launch — those
symlinks are planted and removed by the symlinks backend on disconnect/refresh.

**Correction: this is not on the Core export-tree path and does not get "more likely
with export-tree churn".** `sc_copy_file()` is reachable only via `sc_copy_glob_files()`,
called exclusively with `SC_SNAP_EGL_VENDOR_GLOB`/`SC_SNAP_VULKAN_ICD_GLOB` — globs over
`/etc/glvnd/egl_vendor.d/` and `/etc/vulkan/icd.d/`, the classic symlinks backend's
output. `/var/lib/snapd/export/system/<iface>/` is consumed by **bind mount**
(`sc_do_optional_mount()`, see *GC* above), never by `sc_copy_file()`. An earlier draft
of this document conflated the two and described this as "a precondition for Phase 3";
it is not — fixing it is a good idea on its own classic-side merits, independent of
everything else in this phase.

### The NVIDIA guard inversion is a real production bug

`sc_mount_nvidia_driver()` opens with
`access(nvidia_driver_version_file(), F_OK) != 0 → return`, gating the **entire
function** — including `sc_mount_exported_paths()`, which is vendor-neutral. The whole
snap-exported-paths pipeline is also nested inside `#ifdef NVIDIA_MULTIARCH`.

`NVIDIA_MULTIARCH` **is** enabled in the shipped snapd snap
(`--enable-nvidia-multiarch` in `build-aux/snap/snapcraft.yaml`), so on any Core device
without an NVIDIA kernel module — i.e. nearly all of them — snap-exported libraries and
configs would **never be mounted**. This must be fixed for Core to work at all.

**Fix:** rename to `sc_mount_snap_gpu_driver()`, take `sc_distro distro` as a third
parameter from `config->distro`, and **invert** the structure so snap-exported paths run
first and unconditionally (outside the NVIDIA version check and outside
`#ifdef NVIDIA_*`), with the NVIDIA check guarding only NVIDIA-specific host scanning
(`sc_mount_nvidia_driver_multiarch/biarch`, `sc_mount_vulkan`, `sc_mount_egl`).

**Split into two commits** per this repo's "separate refactoring from behavior changes"
convention: (a) pure rename + parameter, verifiable as a no-op on classic; (b) the guard
inversion.

**Residual classic-side gap (documented, not fixed):** `sc_copy_glob_files` — the ICD
config copy for snap-provided libs on classic — remains behind the NVIDIA check even
after the inversion. A non-NVIDIA driver snap on classic gets libraries but not ICD
configs. Pre-existing; should be tracked separately.

### AppArmor: the scratch rootfs prefix is `/tmp/snap-private-tmp/snap.rootfs_*`

**Not** `/tmp/snap.rootfs_*`. Changed by commit `02cf64b882`; confirmed at
`mount-support.c:477` and across all 112 existing occurrences in
`snap-confine.apparmor.in`. Rules written with the old prefix silently grant nothing.

`snap-confine` will additionally need read access to the export tree at full depth — the
existing `/var/lib/snapd/export/{,*} r,` matches only one level deep (AppArmor `*` does
not cross `/`).

### Packaging

`packaging/ubuntu-26.10/` **does not exist** — the correct file is
`packaging/ubuntu-26.04/snapd.dirs`. Pre-creating the export directory is likely
**unnecessary**: `/var/lib/snapd/export` itself is in no distro's `.dirs` file today and
is created on demand by `os.MkdirAll`, which creates parents recursively.

### Vulkan explicit layers are supported on classic

`vulkan_driver_libs.go`'s `SymlinksConnectedPlug` handles `explicit-layer-source` →
`/etc/vulkan/explicit_layer.d/`. Deferring it on Core is a genuine feature *reduction*,
not parity. Recommend implementing all of `icd.d`, `implicit_layer.d`,
`explicit_layer.d`.

### The `mount` backend's delayed-effects precedent

`interfaces/mount/backend.go:118` defers a snap's mount-namespace update when it is
indirectly affected by a slot-provider refresh, via `DelayedSideEffect`. Directly
analogous machinery exists if the export backend ever needs it — though the export write
itself is cheap and should be immediate, so probably not.

---

## Design decisions (formerly open questions)

**All settled.** Kept with their original numbering so earlier discussion still resolves.

| # | Question | Decision |
|---|---|---|
| **Q1** | Consumer contract — must the consumer scan the classic path, the Core path, or both? | **Consumer's problem.** mesa-2604 will scan the relevant location(s), possibly both; its wrappers pick. Imposes no constraint on us. |
| **Q2** | Unit granularity — per-connection or per-container? | **Per-container.** Revisitable later if it proves awkward. |
| **Q3** | Manifest lists directories or files? | **Files.** Makes pooling a pure string transform, removes all `readdir` from the read path, and needs no per-interface knowledge in C. |
| **Q4** | `export/system/<iface>/` or `export/system_<iface>/`? | **Hierarchical `export/system/<iface>/`.** `system` is a reserved snap name (`overlord/snapstate/snap.go:828`) so it cannot collide. For the system snap the implicit plug is named after the interface — `snapInfo.Plugs[ifaceName] = makeImplicitPlug(...)` in `implicit.go` — so `<iface>` doubles as the plug name. If plug/slot ever need distinguishing, that can be layered on later. |
| **Q5** | Does `.library-source` migrate into the new tree? | **No, not at this point.** It stays flat at `/var/lib/snapd/export/system_<snap>_<slot>_<iface>.library-source`. **See the ownership boundary note below.** |
| **Q6** | Unit naming | **Per-container, revision-derived:** `<snap>_<slot>_<snaprev>` for `$SNAP` content, `<snap>_<slot>_<snaprev>+<comp>_<comprev>` for one component's content. `<slot>` is required for uniqueness — a snap exposing two slots of the same interface would otherwise produce two identically-named units with different content. |
| **Q6a** | Ordering in Go filenames or in the manifest? | **In Go filenames** — implied by Q7. Bind-mounting to encoded target names requires the priority prefix to be part of the filename. |
| **Q7 / Q9** | Pooling mechanism; files or whole directories? | **Enumerate and bind-mount individual files, skipping when the target already exists** — mirroring `sc_mount_exported_paths()`. Supersedes the earlier copy-into-tmpfs sketch; see *Delivery* for why this is strictly better. |
| **Q8** | New backend, or a helper on `configfiles`? | **New backend.** Wanted as a general "export files from a snap" primitive. |
| **Q10** | Hash input, if hashing were used | **N/A** — naming is container/revision-derived, no hashing. |
| **Q11** | Separator ambiguity from parallel installs (`foo_bar_egl-slot_34`) | **Latent and harmless; document only.** Nothing parses these names — every consumer globs (`mount-support-nvidia.c:511-517`, `snapenv.go:184`), `snap-confine` strips the leading path component, and GC is "absent from the manifest ⇒ garbage". The identical ambiguity already exists in `.library-source` today for the same reason. Document that names parse **right-to-left** (`_` occurs only inside instance names; slot names and revisions never contain it, and `+` splits the component part). |
| **Q12** | Pool by interface or by kind? | **By interface** — one exported data set per interface, surfaced at `gpu/system/<iface>/<subdir>/`. |
| **Q13** | Vulkan loader capabilities in the consuming base | **Deferred.** mesa-2604 confirms the current mechanism is fine for them; revisit if a gap appears. Note the earlier caveats still stand — `VK_LAYER_PATH` covers explicit layers only and does not pick up ICDs, and `VK_IMPLICIT_LAYER_PATH` needs a recent loader. |
| **Q14** | Namespace staleness on driver connect | **Keep current behaviour — no discarding.** Accepted limitation: a newly connected driver is invisible to an already-running snap and needs an app restart. Confirmed that snapd does not discard consumer namespaces today. Out of scope; the export mechanism is correct either way. |

### Ownership boundary — consequence of Q5

Because `.library-source` stays put, `/var/lib/snapd/export/` holds output from **two**
backends:

```
/var/lib/snapd/export/
    system_foo_egl-slot_egl-driver-libs.library-source   ← configfiles
    system/<iface>/…                                      ← export backend
```

There is no collision (`system_…` files vs a `system/` directory), but the export
backend must **not** claim `/var/lib/snapd/export/**` wholesale. **GC is scoped to
`export/system/<iface>/` subtrees**, otherwise it would delete configfiles' output.


### Unverified assumption — needs checking before implementation

**Is the slot snap's new revision guaranteed to be mounted before the system snap's
`setup-profiles` runs during a refresh?** The export backend reads snap content at
`Setup()` time, so if that ordering isn't guaranteed the copy could read from an
unmounted revision. `sourceDirsCheck` already reads snap content at connect time and
works, but the refresh path has not been traced through `overlord/snapstate` task
ordering (`link-snap` / `setup-profiles` / `unlink-current-snap` / `discard-snap`).

**Resolved (2c-2), by inheritance rather than by tracing the task graph:** the export
backend deliberately does not defend against this - it is a dumb backend, like
`symlinks`/`configfiles`, and trusts `Setup()` to see the complete, current desired
state. `interfaces/symlinks/backend.go`'s `ensureSymlinks` already reads the exact same
snap content (via the exact same `sourceDirsCheck` call, shared by both backends since
2b/2d) with **no such guard**, and has shipped that way since Phase 1 landed on classic.
So whatever this ordering guarantee turns out to be, export does not introduce a new
failure mode beyond one `symlinks` already has in production; tracing the task graph
would be informative but is no longer a blocker.

One related, and better, mitigation landed as a side effect of investigating a spread
test regression (see *Files status* / commit history): `sourceDirFilesCheck` now treats
a referenced-but-missing library as environment-dependent rather than fatal (wraps
`errLibraryNotFound`, logs via `logger.Noticef`, and skips just that one file - see
`interfaces/builtin/helpers.go`), instead of failing the whole connection. If the
snap-mount race above ever did occur in practice, this turns it from "the connect/refresh
task fails outright" into "that one file is missing for one `Setup()` pass, logged,
self-healing on the next `Setup()` once the mount catches up" - a strict improvement
regardless of whether the race is real.

---

## Implementation phases

### Suggested first PR — design-independent work

None of this depends on the export tree's layout, and two items are real bugs on their
own merits. Good candidate for a first, low-risk change:

1. **NVIDIA guard inversion** (two commits: rename + `sc_distro` param, then the
   inversion). `NVIDIA_MULTIARCH` *is* enabled in the shipped snapd snap, so on any Core
   device without an NVIDIA kernel module the entire snap-exported-paths pipeline never
   runs today.
2. **`sc_copy_file()` `ENOENT` hardening** — currently `die()`s on a missing source,
   killing the snap launch. Reachable on classic *now*.
3. **`sourceDirFileName` extraction** from `symlinksForSourceDir` — a pure refactor the
   backend will need, including the `dirs` package/local shadowing fix. No behaviour
   change.
4. **C unit tests** for `mount-support-nvidia.c` — zero coverage today, and Phase 3
   restructures exactly this code.
5. **Missing spread fixture** — `libs-provider-core26/egl.d/vendor.json`, so `icd-source`
   index 0 stops yielding nothing (see Phase 4b).

**Note on the current branch:** the tip commit `bbe4e46555` is a WIP that *comments out*
the classic `SC_DISTRO_CLASSIC` guard. Item 1 should **replace** it rather than build on
it — interactive rebase, or revert-and-redo.

### Phase 1 — Interface source files · ✅ DONE (upstream/master, PR #17348)

All six interfaces have:

- `implicitPlugOnCore: true`
- `LdconfigConnectedPlug` guarded with `if !release.OnClassic { return nil }`
- `SymlinksConnectedPlug` guarded likewise (egl, gbm, vulkan)
- `ConfigfilesConnectedPlug` unconditional (no `OnClassic` wrapper) — `.library-source`
  is written on classic *and* core
- Unit tests with `*OnCore` variants for every backend
- A dedicated spread test, `tests/core/interfaces-driver-libs/` (Phase 4a below)

### Phase 2 — `export` backend · ✅ DONE

A **new backend** builds the export tree. All layout decisions are settled — see
*Design decisions*.

Shape:

- New `interfaces/export/` package with `Backend` and `Specification`, registered in
  `interfaces/backends/backends.go`. It owns `/var/lib/snapd/export/**` outright.
  **✅ Done (2a):** package skeleton (`Backend`, `Specification`, connect/permanent
  plug/slot callbacks mirroring `configfiles`/`symlinks`), wired into
  `interfaces/core.go` (`SecurityExport`), `interfaces/backends/backends.go`, and
  `interfaces/ifacetest/testiface.go`.
- Reconciliation is **hand-rolled**, not `EnsureDirState`: materialise each unit into
  `<unit>.tmp/` → `rename()` into place (atomic for a directory) → flip
  `export.sources` atomically → GC units no longer listed. See *Write sequence*.
  **✅ Done (2c-2):** `ensureExports`/`ensureExportsForInterface` in the new
  `reconcile.go`, following exactly this sequence.
  **Correction from an earlier note in this doc:** the backend does **not** gate on
  `release.OnClassic` — it is dumb, like `symlinks`/`configfiles`: `Setup()` trusts
  spec to be the complete current desired state and does not defend against the
  snap not being mounted, the same way `symlinks.ensureSymlinks` already relies on
  `sourceDirsCheck` output without any such guard today. The classic/core policy
  belongs to each *interface*'s `ExportConnectedPlug` (2d), exactly where
  `LdconfigConnectedPlug`/`SymlinksConnectedPlug` already put it, with the polarity
  inverted (`if release.OnClassic { return nil }`, since export is Core's
  counterpart to those two, not classic's).
- Per-interface path accessors (`InterfaceRoot`, `UnitDir`, `UnitTmpDir`,
  `ManifestPath`), decided (2c-1) to live inside `interfaces/export` itself
  (`paths.go`), not `dirs.go`.
  **✅ Done (2c-1).**
- Helper in `interfaces/builtin/helpers.go` to declare exported source files, reusing
  `sourceDirsCheck` for validation and sharing the filename encoding with
  `symlinksForSourceDir`.
  **✅ Done (2b):** `sourceDirEncodedName` extracted from `symlinksForSourceDir`
  (fixing the `dirs` shadowing below), plus `export.UnitName` (pure, in
  `interfaces/export/naming.go`) and `exportUnitAndFileName` (glue, in
  `helpers.go`) which combine to produce the export backend's unit + file names.
  Cross-checked by test (`TestExportFileNameMatchesSymlinkName`) that the file name
  is byte-identical to what `symlinksForSourceDir` produces for the same connection.
  **✅ Done (2c-1):** `Specification.AddExportedFile(ifaceName, unit, relPath, state)`
  now records declared files (keyed by interface → unit → relative path), with
  validation (clean/relative path, path must be inside a subdirectory, duplicate
  detection) mirroring `symlinks.Specification.AddSymlink`.
  **✅ Done (2c-2):** GC scoping (which interfaces own an export subtree, so it can
  be swept to empty even with zero current connections) reuses the existing
  `export.ConnectedPlugCallback` marker via `repo.AllInterfaces()`, gathered in
  `Backend.Setup()` exactly like `symlinks.Backend.Setup` gathers `symlinkDirs` from
  `interfaces.SymlinksUser` — no separate `interfaces.ExportUser`-style marker in
  `interfaces/core.go` was needed, since the export subtree name is always just the
  interface's own `Name()`, with no extra per-interface metadata to carry.
- `egl_driver_libs.go` — export `icd-source` → `egl_vendor.d/`.
  **✅ Done (2d):** `ExportConnectedPlug`, gated `if release.OnClassic { return nil }`
  (inverted polarity vs. `LdconfigConnectedPlug`/`SymlinksConnectedPlug` above, noted
  in a comment), via the new shared `exportedFilesForSourceDir` helper (the
  export-backend counterpart of `symlinksForSourceDir`, added to `helpers.go`
  alongside `exportUnitAndFileName` in 2b/2d).
- `vulkan_driver_libs.go` — export `icd-source` → `icd.d/`, `implicit-layer-source` →
  `implicit_layer.d/`, `explicit-layer-source` → `explicit_layer.d/`.
  **✅ Done (2d):** same shape as egl, looping over the three source attributes like
  `SymlinksConnectedPlug` already does.
- `gbm_driver_libs.go` — `// TODO(core):` comment; the GBM backend `.so` delivery path
  on Core is undetermined. **Gap:** GBM-connected snaps on Core will get library paths
  but no GBM driver discovery until this is resolved.
  **✅ Done (2d):** comment added, explaining GBM has no "vendor JSON listing" to
  enumerate (unlike egl/vulkan's icd-source), so there is nothing equivalent to feed
  into the export backend yet; no `ExportConnectedPlug` was added.

**Implementation note — `dirs` shadowing trap.** ~~If the filename encoding is
factored out of `symlinksForSourceDir`, note that~~ `dirs` referred to two different
things in that function: the **package** (`filepath.Rel(dirs.SnapMountDir, …)`) and
a **local `[]string`** (`dirs := strings.SplitN(…)`) that shadowed it for the rest of
the block. It only compiled because the package use preceded the local declaration.
**Resolved in 2b:** the local was renamed to `pathSegments` in the extracted
`sourceDirEncodedName` helper. The helper returns `(name, component string,
componentRev snap.Revision, err error)` — the existing code can fail with
`internal error: wrong file path`.

**Do not use `snap-update-ns` for delivery** — see Options D/E. Live mount-based
delivery breaks consistency with the frozen, bind-mounted libraries.

**✅ Done (2e) — closing pass:**

- **Behavior change found and fixed while chasing a spread test regression:**
  `filePathInLibDirs` (in `helpers.go`) used to fail the *entire connection* whenever a
  single ICD/layer file referenced a library that could not be found under
  `library-source` - reachable on classic since Phase 1, and newly reachable on Core
  once `ExportConnectedPlug` started running the same validation there (2d). Now wraps
  a distinguishable `errLibraryNotFound`; `sourceDirFilesCheck` logs via
  `logger.Noticef` and skips just that one file, matching how a missing *source
  directory* a few lines above was already tolerated, and how other interfaces
  (`gpio`, `uio`, `pwm`) already treat a missing runtime resource as non-fatal rather
  than blocking snapd updates (LP: 1866424). Malformed JSON stays fatal - that is a
  static packaging bug, not an environment-dependent condition.
- This same fix **resolved gbm-driver-libs' own pre-existing TODO**: a `client-driver`
  library entirely provided by an uninstalled component used to fail the connection
  outright; now it is a silent no-op (still just Phase 1's classic-only
  `SymlinksConnectedPlug`, since gbm has no `ExportConnectedPlug` - see below).
- Added `logger.Debugf` for the missing-source-*directory* case (previously silent),
  to help diagnose why nothing got exported/symlinked.
- Added a `TODO` on `interfaces/symlinks/backend.go`'s write path: it discards the
  error from `osutil.EnsureFileState` entirely, unlike the removal path a few lines
  above (which at least logs via `logger.Noticef`). Found by comparison while working
  on export's write path; not fixed, since it is pre-existing, unrelated behavior.
- **Spread test regression, investigated and found already resolved by the fix
  above, not by touching the fixture:** `tests/core/interfaces-driver-libs`'s
  `libs-provider-core26` ICD files (`comp1`/`comp2` `egl.d/vendor.json`) reference
  `libsquare.so`, which the fixture never provides. Before the fix this would have
  made `ExportConnectedPlug` fail the `egl-driver-libs` connect on UC26 (proven with a
  dedicated unit test, `TestExportSpecMissingLibraryIsNotFatal`, reproducing the exact
  fixture shape). After the fix it is a silent no-op: the connect succeeds, but
  `egl-driver-libs` exports nothing.

  **Subsequently fixed in the 2f review follow-up** (see below): initially left as-is
  on the grounds that export-content assertions are 4b's job, but review found the
  consequence was sharper than that framing implied — the test was passing *because of*
  the missing-library tolerance, so the export write path was never exercised at all on
  UC26, and 4b would have had to discover and fix the fixture before it could add any
  assertion. Two empty stub libraries now make the happy path real.
- Corrected `# TODO: verify export/data/ ICD files...` in that same `task.yaml` to the
  real path, `/var/lib/snapd/export/system/egl-driver-libs/` (`export/data/` was a
  path from a superseded draft of this doc).
- Corrected `interfaces/export/backend.go`'s package doc, which claimed the tree is
  "meaningful on both classic and Ubuntu Core" - true of the mechanism, but as of 2d no
  interface actually uses the backend on classic.
- Added a `TODO` to `cmd/snap-mgmt/snap-mgmt.sh.in`: it cleans neither
  `/var/lib/snapd/export` nor the symlink directories tracked by
  `interfaces.SymlinksUser` (`/etc/glvnd/egl_vendor.d`,
  `/etc/vulkan/{icd,implicit_layer,explicit_layer}.d`,
  `/usr/lib/<arch>-linux-gnu/gbm`), despite `interfaces.SymlinksUser`'s doc comment
  (`interfaces/core.go`) explicitly calling for tracked directories to be added here.
  Pre-existing Phase 1 gap, classic-only, found while auditing 2d's blast radius;
  `tests/main/postrm-purge` stays green since it only checks `$SNAP_MOUNT_DIR` and
  `/var/snap`. Left as a follow-up, not fixed here.

#### 2f — review follow-ups

A review of the Phase 2 commits (`056eecd8a1..HEAD`) raised three findings plus one the
review itself missed. Dispositions:

- **GC timing (M1) — resolved by decision, not by code.** The reviewer flagged that
  collection is immediate while this document recommended deferral. The document was
  wrong (and self-contradictory — see the *GC* section). Decision: collection stays
  immediate; **readers must be resilient**. Recorded in three places: the *GC* section
  above, an explicit Phase 3 requirement, and a contract comment on
  `ensureExportsForInterface` so a future reader implementation cannot miss it.
- **Concurrent-`Setup()` tmp-dir collision (M2) — `TODO` only.** `UnitTmpDir` is a
  deterministic path shared by racing `Setup()` calls, so one can `RemoveAll` another's
  in-progress directory, or both reach `AtomicRename` and the loser gets `ENOTEMPTY`
  (it is `rename(2)`, which will not replace a non-empty directory). The result is a
  spurious transient `Setup()` failure that heals on retry, not corrupt state — racing
  writers of one unit write identical content, since the unit name derives from
  immutable revisions. A related interleaving can leave the manifest referencing a unit
  a concurrent stale-spec writer collected, which likewise resolves next `Setup()`.
  Documented in `materialiseUnit` with candidate fixes (unique temporary directories, or
  tolerating `ENOTEMPTY`/`EEXIST` after re-verifying the target); deferred deliberately.
- **Vacuous `OnClassic` tests (M3) — fixed.** `TestExportSpecOnClassic` in both
  `egl_driver_libs_test.go` and `vulkan_driver_libs_test.go` created no source fixtures,
  so `Files()` was empty whether or not the `release.OnClassic` gate existed — deleting
  the gate would not have failed either test. The vulkan variant additionally never
  mocked `OnClassic`, relying on the suite-level default. Both now write fixtures and
  assert emptiness against content that *is* present on disk.

  Note the fix is subtler than it looks: writing only the ICD files is **not** enough,
  because a missing `library_path` target makes the export skip every file anyway (that
  is precisely what `TestExportSpecMissingLibraryIsNotFatal` asserts), leaving the test
  vacuous for a different reason. The fixtures must also provide the libraries the ICD
  files name.
- **Spread fixture had no libraries (missed by the review) — fixed.** See the entry
  above; `comp1/libs/libsquare.so` and `comp2/libs/libmultiply.so` are now present as
  empty stubs. `filePathInLibDirs` only checks existence, so content is irrelevant, and
  a comment in `task.yaml` says so. This makes `snap connect` on UC26 actually run the
  export materialise-and-manifest path, giving the existing test real smoke coverage
  ahead of 4b's explicit assertions.

Not addressed, recorded for later: a revert-reuse test and a reconcile error-path test
(the code is correct — materialise failures abort before the manifest flip — but nothing
asserts it), and `logger.Noticef` firing on every `Setup()` for a permanently missing
library, which is noisy for what is usually a static packaging bug.

### Phase 3 — `snap-confine` · ⏳ IN PROGRESS

Ordered step list. Each step lands as its own commit; commits are sequenced so every
intermediate state builds and, where the step is a pure refactor, is behaviourally a
no-op on classic. Steps 2–4 are the only ones that change what runs on Core.

**Reader resilience — required of step 4, satisfied by an existing primitive.**

Because export-tree collection is immediate (see *GC* above), the reader is the side
that has to absorb the race. Every consumer of `export.sources` must:

- skip a manifest entry whose file is **missing** (`ENOENT`) — it was collected between
  the manifest read and the mount attempt;
- never `die()` on it — a snap launch must not fail because a driver snap was refreshed
  or disconnected concurrently. Losing one ICD degrades gracefully; failing the launch
  does not;
- tolerate the manifest itself being absent (no connections).

This is satisfied by construction, at no extra cost: `sc_do_optional_mount()`
(`cmd/libsnap-confine-private/mount-opt.h:77`) already implements exactly this contract
— *"silently fail when mount fails with ENOENT... carry on as if nothing had
happened"* — so step 4 simply has to use it instead of `sc_do_mount()`. No separate
hardening work is required before step 4; see the corrected note below.

#### Preliminary, independent fix — `sc_copy_file()`: tolerate a vanishing source · ✅ DONE

**Correction to an earlier draft of this plan:** this was previously listed as "step 1"
and described as a precondition for the steps below. It is not, and the corresponding
commit message overstated its relevance. `sc_copy_file()` is reachable only via
`sc_copy_glob_files()`, called exclusively with classic-only globs over
`/etc/glvnd/egl_vendor.d/` and `/etc/vulkan/icd.d/` — the symlinks backend's output.
`/var/lib/snapd/export/system/<iface>/` is never read by `sc_copy_file()`; it is
consumed by bind mount (step 4), and reader resilience there comes from
`sc_do_optional_mount()`, not from anything in this file.

The fix itself is still worth having, on its own classic-side merits: a
`/etc/glvnd`/`/etc/vulkan` symlink target removed between `glob()` and the copy
currently kills the snap launch, reachable on classic today, independent of anything
else in this document. Landed as commit `75bd0c9b5e` (message needs amending to drop
the "precondition for Phase 3" framing). Kept as its own commit; no other step depends
on it.

#### Step 1 — rename `sc_mount_nvidia_driver` → `sc_mount_snap_gpu_driver`, thread `sc_distro` · ✅ DONE

Pure rename + parameter addition, verifiable as a no-op on classic. `mount-support-nvidia.h`
gains `#include "../libsnap-confine-private/classic.h"`; the function becomes
`sc_mount_snap_gpu_driver(rootfs_dir, base_snap_name, distro)`. `mount-support.c`'s call
site is updated to pass `config->distro`.

**Supersedes, does not build on, the current branch tip.** The current tip commit
(`25715557c3`, message "TODO") comments out the `SC_DISTRO_CLASSIC` guard rather than
threading it through properly. This step replaces that WIP marker with the real
`sc_distro` parameter; it is not layered on top of it.

**Verified:** builds warning-free (`-Wall -Wextra -Werror`) under all three configure
variants — plain, `--enable-nvidia-biarch`, and `--enable-nvidia-multiarch` (the latter
is what the shipped snapd snap uses). The existing 47 `snap-confine` unit tests continue
to pass unchanged (this build links `mount-support-test.c`, which `#include`s
`mount-support-nvidia.c` directly, so it exercises this exact change). Landed as commit
`ec13faf1b4`.

#### Step 2 — invert the NVIDIA guard

The behaviour change that makes Core work at all. `sc_mount_snap_gpu_driver()` currently
opens with `access(nvidia_driver_version_file(), F_OK) != 0 → return`, which gates the
**entire function** — including `sc_mount_exported_paths()`, which is vendor-neutral.
`NVIDIA_MULTIARCH` *is* enabled in the shipped snapd snap
(`--enable-nvidia-multiarch`, `build-aux/snap/snapcraft.yaml:320`), so today the whole
snap-exported-paths pipeline never runs on a Core device without an NVIDIA kernel
module — i.e. nearly all of them.

New structure — trigger condition is **"skip only when `classic && !nvidia_present`"**:

```c
const bool is_classic     = (distro == SC_DISTRO_CLASSIC);
const bool nvidia_present = access(nvidia_driver_version_file(), F_OK) == 0;
if (is_classic && !nvidia_present) {
    return;
}
```

Host-driver scanning (`sc_mount_nvidia_driver_multiarch/biarch`, `sc_mount_vulkan`,
`sc_mount_egl`, and the `globs`/`globs_len` setup they need) moves inside
`if (is_classic) { ... }`. `sc_mount_vulkan`/`sc_mount_egl` read `/usr/share/{vulkan,glvnd}`
— host-driver paths that on Core resolve to the base snap and would only mount empty
tmpfses — so they become classic-only rather than merely skipped when
`exported_paths > 0`.

**`#ifdef NVIDIA_MULTIARCH` stays where it is** — decided: `sc_mount_exported_paths()`
is not hoisted out of it. Costs nothing on Core (the snapd snap is built
`--enable-nvidia-multiarch`); the pre-existing BIARCH-classic gap (Arch/Fedora/openSUSE
get no driver-libs exports at all) is unchanged and recorded as a follow-up, not fixed
here.

Classic behaviour is preserved *by construction*, not just by this guard: the export
backend returns early on `release.OnClassic` (Phase 2, 2d), so no `export.sources`
manifest ever exists on classic, and step 4's manifest-consuming code is an inherent
no-op there regardless of this guard.

#### Step 3 — AppArmor: read the export tree, allow the new bind mounts

Lands **before** step 4's code needs it — permitting reads/mounts nothing yet performs
is a safe no-op. Two corrections to note before editing:

- the existing `/var/lib/snapd/export/{,*} r,` matches only **one level deep** (AppArmor
  `*` does not cross `/`), so the manifest and unit files are unreadable as-is; needs
  `/var/lib/snapd/export/system/{,**} r,`;
- the scratch rootfs prefix is `/tmp/snap-private-tmp/snap.rootfs_*`, **not**
  `/tmp/snap.rootfs_*` (changed by commit `02cf64b882`).

```
# read the manifest and the exported files
/var/lib/snapd/export/system/{,**} r,
# bind-mount exported files into the pooled tree
mount options=(ro bind) /var/lib/snapd/export/system/** -> /tmp/snap-private-tmp/snap.rootfs_*/var/lib/snapd/lib/system/gpu/system/**,
```

The existing `/tmp/snap-private-tmp/snap.rootfs_*/var/lib/snapd/lib/system/gpu/{,**} w,`
already covers creating the placeholder files and their parent directories inside the
tmpfs — no change needed there. No snap-side AppArmor change is needed either —
`interfaces/builtin/opengl.go` already grants `/var/lib/snapd/lib/system/gpu/{,**} rm`.

#### Step 4 — consume `export.sources`; pool into the snap's mount namespace

Read the manifest for each hardcoded GPU interface (mirroring the existing
`.library-source` glob list — all seven interface names, even though only
`egl-driver-libs` and `vulkan-driver-libs` currently produce a manifest; the rest are
`ENOENT` no-ops, future-proofing `gbm-driver-libs`). For each line:

- `<unit>/<subdir>/<file>` relative to `/var/lib/snapd/export/system/<iface>/`;
- strip the leading `<unit>/` component (a pure string operation — factor into a small,
  independently testable helper, since this is the one piece of step 4 with no I/O or
  mount dependency);
- source = interface root + line; destination =
  `<rootfs>/var/lib/snapd/lib/system/gpu/system/<iface>/<subdir>/<file>`;
- skip if the destination already exists (mirrors the existing dedup in
  `sc_mount_exported_paths()`; with encoded filenames a genuine collision shouldn't
  occur in practice, so this is a safety net, not the primary defence);
- `mkpath(dirname(dst))`, create an **empty placeholder file** (you cannot bind-mount a
  file onto a directory — `sc_mount_exported_paths()` only ever creates directories
  today, this is new), then **`sc_do_optional_mount(src, dst, NULL, MS_BIND | MS_RDONLY,
  NULL)`** — not `sc_do_mount()` — which is what supplies the reader-resilience contract
  described above: it silently no-ops if `src` has disappeared since the manifest was
  read, rather than `die()`ing.

**Ordering constraint, forced by existing code:** `sc_mount_exported_paths()` currently
ends with `sc_remount_ro(tmpfs_path)` before returning — the `gpu/` tmpfs is read-only
once that call returns. Config pooling must happen **before** that remount, so the
function is restructured to: create tmpfs → bind-mount library dirs from
`.library-source` (existing) → bind-mount config files from `export.sources` (new) →
single `sc_remount_ro()` at the end.

Delivery is **bind-mount, not copy** — see *Delivery is per-file bind mounts (Q7)* above
for the full rationale (superseded an earlier copy-into-tmpfs sketch). The short version:
pinning the source inode makes GC safe by construction (a collected unit cannot pull
content out from under an already-mounted snap), and it keeps one delivery mechanism
for both libraries and metadata rather than two.

Existing local infrastructure that helps: `/var/lib/snapd` is already in the
scratch-rootfs bind-mount list, so reading `/var/lib/snapd/export/...` from inside the
scratch rootfs works on Core unchanged, with no new mount rule needed for the *read*
side (only for the new *bind-mount* destinations, added in step 3).

#### Step 5 — unit tests

`cmd/snap-confine/mount-support-test.c` already `#include`s `mount-support-nvidia.c`
directly, so its `static` functions are reachable in-process; currently it has three
tests and none touch this code. **Scope deliberately limited**, per direction: tests
that perform real mount operations would affect the host machine running the test
suite, so this step only covers the pure, I/O-free helper extracted in step 4 (manifest
line validation and unit-prefix stripping) — e.g. a valid line, an absolute path, a
path containing `..`, a line with no `/`, and an empty line. The mount- and
copy-dependent code paths remain untested here, consistent with the rest of this file.

#### Step 6 — spread test (4b)

See *Phase 4* below. Tracked separately since it is being run and verified directly
rather than as part of this commit sequence.

### Phase 4 — Spread tests

#### 4a. Phase 1 scope · ✅ DONE — `tests/core/interfaces-driver-libs/`

`systems: [ubuntu-core-26-64]`, using `libs-provider-core26` (base: core26) plus
`comp1`/`comp2` component fixtures. **Confirmed executing on UC26 in CI**, not merely
merged. Flat linear script (no shell helper functions) that installs the provider and
components, connects `cuda-driver-libs` and `egl-driver-libs`, verifies `.library-source`
content, asserts `/etc/ld.so.conf.d/snap.system.conf` and `/etc/glvnd/egl_vendor.d/*snap*`
are **absent**, and checks cleanup on disconnect and on snap remove. Carries a
placeholder: `# TODO: verify export/data/ ICD files once the support is added`.

#### 4b. Extend once Phase 2 lands · ⏳ TODO

**Fixture gap to fix first:** `libs-provider-core26/` contains only `meta/snap.yaml` —
there is **no `egl.d/` directory**, even though its snap.yaml lists `$SNAP/egl.d` as the
first `icd-source` entry. That entry (dir index 0) therefore yields **zero** files today.

Add `libs-provider-core26/egl.d/vendor.json` with a distinct `library_path` (e.g.
`libsnaplevel.so`) so the test exercises snap-level *and* component-level sources, which
is what actually validates multi-provider pooling.

**Filename derivation** — `<priority + dirIdx>_snap_<instance>[+<comp>]_<slot>_<escapedPath>`,
with `priority: 15`:

| dirIdx | source entry | prefix | file present? |
|---|---|---|---|
| 0 | `$SNAP/egl.d` | 15 | only after the fixture is added |
| 1 | `$SNAP_COMPONENT(comp1)/egl.d` | 16 | yes |
| 2 | `$SNAP_COMPONENT(comp2)/egl.d` | 17 | yes |

`dirIdx` is the **position in the `icd-source` list**, counting entries whose directory
is missing or whose component is not installed (`interfaces/snap_app_set.go:86`,
`for idx, dir := range paths`; cross-checked against the landed classic comps test,
whose four-entry list with `comp0` at index 1 yields `17_…comp1…`/`18_…comp2…`).

> An earlier draft of this plan asserted a `15_…` file *without* adding the snap-level
> fixture — that check would have failed.

Assertions to add: the pooled directory exists; the expected files are present with
verbatim content; the file count is exact; and everything is cleaned up on disconnect
and on snap remove. **Verify names against real output before merging** — they are
derived from reading `symlinksForSourceDir`, not observed.

#### 4c. Sequencing constraint

Phase 3's AppArmor rules must land **with or before** the C changes that need them —
never after, or `snap-confine` hits denials. AppArmor-first is safe (permitting a mount
from a path nothing creates yet is a no-op).

---

## Files status

| File | Status | Phase |
|---|---|---|
| `interfaces/builtin/{cuda,opengl,opengles}_driver_libs.go` | ✅ Done | 1 |
| `interfaces/builtin/{egl,vulkan}_driver_libs.go` | ✅ Done — `ExportConnectedPlug` wired (2d) | 1, 2 |
| `interfaces/builtin/gbm_driver_libs.go` | ✅ Done — Phase 1, plus `TODO(core)` comment (2d) | 1, 2 |
| `interfaces/builtin/*_driver_libs_test.go` (all 6) | ✅ Done — Phase 1, plus export tests for egl/vulkan (2d) | 1, 2 |
| `interfaces/export/` (new backend) | ✅ Done — skeleton, naming, paths, `Specification.AddExportedFile`, and full reconciliation (`ensureExports`/`ensureExportsForInterface`/GC) in `reconcile.go` | 2 |
| `interfaces/backends/backends.go` | ✅ Registered | 2 |
| `dirs/dirs.go` | Not needed — decided (2c) the per-interface path accessor lives inside `interfaces/export` (`paths.go`), not `dirs.go` | 2 |
| `interfaces/builtin/helpers.go` | ✅ Filename encoding extracted (`sourceDirEncodedName`); `exportUnitAndFileName` added | 2 |
| `cmd/snap-confine/mount-support-nvidia.{c,h}` | ✅ preliminary fix (`sc_copy_file` hardening) done; ✅ Step 1 (rename + `sc_distro`) done. ⏳ Step 2: guard inversion. Step 4: manifest reader + bind-mount pooling | 3 |
| `cmd/snap-confine/mount-support.c` | ✅ Step 1 (call site updated) done | 3 |
| `cmd/snap-confine/mount-support-test.c` | ⏳ Step 5: tests for the pure manifest-line helper only (no mount-dependent coverage — would affect the host running the suite) | 3 |
| `cmd/snap-confine/snap-confine.apparmor.in` | ⏳ Step 3: deep read on export tree; new bind-mount rule; correct `/tmp/snap-private-tmp/` prefix | 3 |
| `packaging/ubuntu-26.04/snapd.dirs` (+ others) | Optional / probably unnecessary | — |
| `tests/core/interfaces-driver-libs/task.yaml` | ✅ Phase 1 version; runs on UC26 in CI | 4a |
| `tests/core/interfaces-driver-libs/libs-provider-core26/meta/snap.yaml` | ✅ Done | 4a |
| `tests/core/interfaces-driver-libs/libs-provider-core26/egl.d/vendor.json` | ⏳ Missing fixture — blocks index-0 coverage | 4b |
| `tests/core/interfaces-driver-libs/comp{1,2}/…` | ✅ Done | 4a |
| `tests/core/interfaces-driver-libs/task.yaml` (extended) | ⏳ Replace placeholder | 4b |

---

## Revision history

Recorded so the reasoning isn't lost and the same ground isn't re-covered.

### Pass 11 — corrected a false dependency between `sc_copy_file()` and the export tree (current)

- **Caught by direct question, not by review:** Pass 10 listed `sc_copy_file()`
  hardening as "step 1" and described it as "a precondition for step 5 trusting a
  manifest that can be concurrently rewritten". This was wrong. `sc_copy_file()` is
  reachable only via `sc_copy_glob_files()`, called exclusively with classic-only globs
  over `/etc/glvnd/egl_vendor.d/` and `/etc/vulkan/icd.d/` — it is never on the path
  that reads `/var/lib/snapd/export/system/<iface>/`, since that tree is consumed by
  **bind mount**, not by copy.
- **Root cause:** stale text in the *GC* section, left over from the superseded
  copy-into-tmpfs sketch (Q7's earlier draft), that still said `sc_copy_file()`
  "becomes a precondition" under the current bind-mount design. That paragraph was
  read and trusted without re-deriving it against the (already-settled, already
  documented) bind-mount decision, and the error was then propagated into the Phase 3
  step list and the commit message for the already-landed fix.
- **Correction:** the reader-resilience contract Phase 3 actually needs is already
  satisfied by `sc_do_optional_mount()` (`cmd/libsnap-confine-private/mount-opt.h:77`),
  an existing primitive documented to silently no-op on `ENOENT` — this is what the
  bind-mount step (now step 4, was step 5) uses, with no dependency on anything in
  `sc_copy_file()`.
- **Disposition of the already-landed fix** (commit `75bd0c9b5e`): kept, since it fixes
  a real bug reachable on classic today (an `/etc/glvnd`/`/etc/vulkan` symlink target
  removed between `glob()` and the copy currently kills the snap launch) — but
  reclassified from "step 1 of the ordered sequence" to a standalone, independent fix
  with no downstream dependents, and the commit message amended to drop the false
  "precondition for Phase 3" framing.
- Renumbered the remaining Phase 3 steps 1–6 (previously 2–7) to close the gap left by
  demoting the `sc_copy_file()` item out of the numbered sequence.
- Fixed the two places this error had propagated to: the *GC* section's "Directly
  implied requirement for Phase 3" paragraph, and the *`sc_copy_file()` dies on a
  missing source* finding (retitled to make the classic-only, non-Core-path scope
  explicit).

### Pass 10 — Phase 3 broken into an ordered, commit-per-step plan

- Confirmed Phase 2 complete and reflected as such throughout (it already was, this
  pass only re-verified against the tree — `interfaces/export/` fully implemented,
  registered, wired into egl/vulkan, 2f review follow-ups applied).
- Rewrote the Phase 3 section from a two-bucket ("design-independent" /
  "design-dependent") sketch into seven ordered steps, each an independent commit:
  1. `sc_copy_file()` reader-resilience hardening
  2. rename + thread `sc_distro` (pure refactor)
  3. NVIDIA guard inversion (the behaviour change)
  4. AppArmor (lands before the code that needs it)
  5. consume `export.sources`, bind-mount into the pooled tree
  6. unit tests (scoped to the pure helper only — see below)
  7. spread test (tracked separately; being run and verified directly rather than as
     part of this commit sequence)
- **Scoped step 6 down** in response to direction: tests that perform real mount
  operations would affect the host machine running the test suite, so C unit test
  coverage is limited to the pure, I/O-free manifest-line-parsing helper factored out
  of step 5. The mount- and copy-dependent code stays untested here, matching the
  existing state of `mount-support-test.c`.
- Re-derived the *why* behind reader resilience being step 1 rather than a footnote:
  it fixes a bug reachable on classic *today*, and step 5 cannot be trusted to read a
  concurrently-rewritable manifest without it.

  **Corrected in Pass 11: this "why" was itself wrong** — see above. `sc_copy_file()`
  has no relationship to step 5/4's manifest reading.
- No design changes from Pass 9 — this pass is purely about sequencing the already-
  settled design into an actionable, reviewable commit series.


### Pass 9 — Phase 2 review; GC timing decided

- Reviewed the Phase 2 commits (`056eecd8a1..HEAD`). Implementation found faithful to the
  design: set atomicity via tmp-dir + `AtomicRename` before the manifest flip, GC scoped
  strictly to `InterfaceRoot` (never `/var/lib/snapd/export/` itself), per-container
  units, manifest listing files, and byte-identical filenames between the classic
  symlink path and the export path.
- **GC timing decided: immediate collection, resilient readers.** This document had
  recommended deferral (next-`Setup()` GC, mtime grace, boot sweep) in its *GC* section
  while its *Write sequence* section already described same-pass collection — the two
  contradicted each other, and the implementation followed the write sequence. Resolved
  in favour of the implementation: a reader of a mutable tree under `/var/lib/snapd/`
  has to be resilient regardless, so deferral would only narrow a window that still
  needs correct handling, at the cost of grace-period state and unbounded interim disk
  usage. Reader resilience is now an explicit Phase 3 requirement and a documented
  contract in `ensureExportsForInterface`.
- **Concurrent-`Setup()` tmp-dir collision recorded as a `TODO`.** Racing `Setup()`
  calls share `UnitTmpDir`; the failure mode is a transient, self-healing `Setup()`
  error rather than corruption, because unit names derive from immutable revisions so
  racing writers write identical content. Deferred with candidate fixes noted.
- **Two vacuous tests fixed.** Both `TestExportSpecOnClassic` variants would have passed
  with the `release.OnClassic` gate deleted, because they created no source fixtures.
  Fixing them required writing the referenced *libraries* as well as the ICD files —
  otherwise the missing-library skip keeps `Files()` empty and the test stays vacuous
  for a different reason.
- **Spread fixture gap closed.** `tests/core/interfaces-driver-libs` referenced
  `libsquare.so`/`libmultiply.so` that the fixture never provided, so on UC26 every ICD
  file was skipped and the export write path was never exercised — the test was passing
  *because of* the missing-library tolerance. Two empty stub libraries make the happy
  path real. Also corrected the 2d entry above, which had recorded this as deliberately
  deferred to 4b.

### Pass 8 — all design questions settled; ready to implement

Every open question answered. Two of the answers changed the design, and one exposed a
correction to an earlier claim.

**Decisions taken** (full table in *Design decisions*): per-container units (Q2/Q6) named
`<snap>_<slot>_<snaprev>[+<comp>_<comprev>]`, hierarchical `export/system/<iface>/` (Q4),
`.library-source` stays where it is (Q5), delivery by per-file bind mount (Q7/Q9),
pooling by interface (Q12), no hashing (Q10), consumer handles both paths (Q1), loader
capabilities deferred (Q13), namespace staleness accepted (Q14).

**Q7 changed the delivery mechanism — copy → bind mount.** Earlier passes assumed
`snap-confine` would *copy* files into a tmpfs, mirroring `sc_copy_glob_files()` on
classic. Delivering by per-file `MS_BIND|MS_RDONLY` mount instead is better on two
counts: it is the *same* mechanism already used for libraries, making the
frozen-for-the-namespace-lifetime property uniform rather than coincidental; and the
mount pins the source inode, so GC cannot pull content out from under a running snap.
Implementation consequence: `sc_mount_exported_paths()` only creates directories, so the
new code must also create an **empty placeholder file** before binding — a file cannot be
bind-mounted onto a directory.

**Q5 corrected the ownership claim.** Earlier passes said the export backend "owns
`/var/lib/snapd/export/**` outright, enabling safe GC of unrecognised entries". With
`.library-source` staying put, that directory holds output from *two* backends. GC must
be scoped to **`export/system/<iface>/` subtrees only**, or it would delete configfiles'
output. No collision — `system_…` files versus a `system/` directory — but the boundary
now has to be explicit.

**Q6a resolved by implication**, not independently: binding to encoded target names
requires the priority prefix to be part of the filename, so ordering stays in Go.

**Q11 confirmed harmless.** Verified that nothing parses these names — every consumer
globs (`mount-support-nvidia.c:511-517`, `snapenv.go:184`), `snap-confine` strips the
leading path component, GC is set-membership. The same latent ambiguity already exists in
`.library-source` today, for the same reason. Documented as a right-to-left parse
convention; no structural change.

**Q4 verified against the code:** the implicit plug on the system snap really is named
after the interface — `snapInfo.Plugs[ifaceName] = makeImplicitPlug(snapInfo, ifaceName)`
in `overlord/ifacestate/implicit.go` — so `<iface>` doubling as the plug name is
consistent, and leaves room to distinguish plug/slot later.

One assumption remains unverified: whether the slot snap's new revision is guaranteed
mounted before the system snap's `setup-profiles` runs during a refresh. Flagged as
"verify early in Phase 2" rather than a blocker — worst case it changes error handling,
not the design.

### Pass 7 — `snap-update-ns` explored; consistency settles the direction

Explored whether `snap-update-ns` could deliver export content instead of
`snap-confine`, and whether classic's non-atomic behaviour lets us relax the atomicity
requirement. Outcome: **Option C confirmed as the direction**, with two properties now
established as requirements.

**`snap-update-ns` is capable** (Option D) — verified `x-snapd.kind=file` bind-mounts
individual files, `x-snapd.ignore-missing` makes absent sources a silent no-op,
non-layout entries fail soft, `UpdateSnapNamespace` reaches already-running snaps, and
`ExtraLayouts` bypasses snap.yaml validation (journal quota already binds from
`/run/systemd/…`). Also found that **`/var/lib/snapd` is already bind-mounted
recursive-slave into every namespace** (`mount-support.c:913`), so the export tree is
visible at the same path with no new mount at all — that became Option E.

**Both rejected on cross-component consistency.** Libraries arrive as
`MS_BIND|MS_RDONLY` mounts of *revision-specific* directories, so they are **frozen for
the namespace's lifetime** — and that is correct, since a shared object cannot be safely
hot-swapped under a running process. Live metadata over frozen libraries is not a
transient window but a **persistent mismatch until the namespace is rebuilt**. That it
rarely bites, because `library_path` is a stable SONAME, is luck rather than design.

**The liveness argument was also weaker than it first appeared** — an initial claim that
Option D "fixes the `snap-discard-ns` TODO" was **withdrawn**: a newly connected driver's
*libraries* aren't bind-mounted into the running namespace either, so the loader reads
the new ICD, `dlopen`s a missing library, and skips it. Different failure path, same
outcome.

**On classic's precedent.** Confirmed that `sc_populate_libgl_with_hostfs_symlinks()`
creates symlinks into `/var/lib/snapd/hostfs`, and that `sc_mkdir_and_mount_and_glob_files()`
— tmpfs + symlink farm — serves the **primary** multiarch branch *and*
`sc_mount_vulkan()`/`sc_mount_egl()`. So on classic with host-packaged NVIDIA, libraries
*and* configs are symlinks into live host state that `apt upgrade` can dangle mid-flight.
An intermediate framing that this "removes set atomicity as a requirement" was
**corrected**: it calibrates severity, it does not dispose of the requirement. Core
plausibly warrants a higher bar — unattended auto-refresh, a transactional platform
model, snapd owning the whole stack (classic's behaviour is a constraint, not a choice),
and unattended-appliance failure surfaces. Recorded in *On atomicity*.

**Consequences for the design:**

- **Q8 resolved: new backend.** Direction committed — build the export tree in a
  dedicated backend and expose it via `snap-confine`.
- Atomicity and liveness are **in tension**: atomic means build-then-flip and therefore
  frozen; live means mutate-in-place and therefore observable mid-update. The frozen
  branch is the one consistent with library delivery.
- The **namespace is the unit of consistency** — everything in it comes from one
  generation; picking up a new one means rebuilding it.
- **Q14 added and scoped out**: snapd does **not** discard a consumer's namespace when
  the driver set changes (confirmed with the user). That is the real defect behind the
  `snap-discard-ns` TODO, and the correct fix is namespace rebuild — precedent exists in
  `should_discard_current_ns()`, which already discards when the base snap changes. Out
  of scope here; without it, newly connected drivers need an app restart.

### Pass 6 — pooling mechanics worked through, `system/` confirmed safe

Concentrated on how `export.sources` is actually consumed by `snap-confine`, driven by a
concrete request: files at
`export/system/<iface>/<unit>/icd.d/<encoded>.json` should surface pooled at
`lib/system/gpu/system/<iface>/icd.d/<encoded>.json`.

- **Added a full worked example** — export tree, manifest contents, transform, and
  pooled result — using the real encoded names asserted by the landed classic test
  (`tests/main/nvidia-userspace-libs/test-nvidia-libs/test`).
- **Pooling is a one-line string transform**: strip the first path component from each
  manifest entry. This **resolves Q3 in favour of listing files**, because it removes
  all `readdir` from the read path (a collected unit can then only ever yield `ENOENT`
  on an individual file) and means `snap-confine` needs no hardcoded table of
  subdirectory kinds — new kinds work with zero C changes.
- **Confirmed the design matches classic**, which already pools into flat
  encoded-name directories (`lib/glvnd/egl_vendor.d/`, `lib/vulkan/icd.d/`). Raised
  **Q12**: classic pools by *kind*, the Core sketch pools by *interface*.
- **`system` is a reserved snap name — verified.** `checkInstallPreconditions` rejects
  it outright (`overlord/snapstate/snap.go:828`: *"cannot install reserved snap name
  'system'"*), checked against `InstanceName()` so `system_foo` is blocked too, and it
  is additionally an API-level nickname remapped to `snapd`/`core`
  (`overlord/ifacestate/helpers.go:1476`, `:1523`). Consequences: `gpu/system/<iface>/`
  **cannot** collide with `gpu/<snap-instance>/<rev>/`; the underscore form
  `gpu/system_<iface>/` is unnecessary; and the `gpu/libs/` + `gpu/data/` split proposed
  in Pass 4 as collision-avoidance was **dropped as unfounded**. The reserved name is
  itself the namespace separator, which also strengthens Q4's forward-compat argument.
- **Restored the consumer-contract detail** lost in the Pass 4 restructure (snapd sets
  *none* of the loader environment variables — re-verified), now with an explicit
  directory → env-var mapping table.
- **Raised Q13** on loader capabilities: whether `VK_DRIVER_FILES` accepts a directory
  or only a file list, and whether `VK_IMPLICIT_LAYER_PATH` is supported. Corrected a
  common conflation: `VK_LAYER_PATH` is for *explicit layers* and does not pick up ICDs.
  If (b) is unsupported in the shipping base, implicit layers exported from snaps are
  **undiscoverable on Core** — a functional gap worth knowing before implementation.
- **Noted a tmpfs lifecycle constraint**: `sc_mount_exported_paths()` remounts `gpu/`
  read-only before returning, so config pooling must complete before that, or use a
  separate tmpfs whose mountpoint is pre-created.

### Pass 5 — unit granularity and naming worked through

Focused entirely on nailing down the unit naming, driven by the scenarios: snap-only
refresh, snap + component refresh, and the fact that one `icd.d/` holds a mix of `$SNAP`
and `$SNAP_COMPONENT(...)` content.

Verified in the tree:

- **Static slot attributes cannot be shadowed dynamically** — `ConnectedSlot.SetAttr()`
  (`interfaces/connection.go:301`) refuses to override a statically-declared attribute.
  So `icd-source` / `priority` are pinned by the snap revision, which is the
  precondition for any revision-derived naming.
- **`slot.AppSet()` is the slot snap's own AppSet** (`NewConnectedSlot` panics
  otherwise), so every contributing component revision is enumerable at `Setup()` time
  **without reading any file** — enabling `stat`-and-skip.
- **Container origin is already encoded per-file** via `compSuffix`
  (`snap_foo+comp1_…`), so the "mixed `icd.d/`" case is handled at the file level
  regardless of directory granularity.

Rulings made:

- **`<snap>_<slot>_<snaprev>` alone — ruled out.** A component-only refresh changes
  content without changing the name → in-place rewrite → set atomicity lost.
- **Revision tuple `…_<snaprev>[_<comp>-<comprev>]…` — ruled out.** Component names are
  capped at 40 chars (`ComponentRef.Validate()` → `ValidateSnap()`,
  `snap/naming/validate.go:86`) but the **number of components is unbounded** — no limit
  found in `snap/` or `overlord/snapstate/`. At ~48 bytes per segment against
  `NAME_MAX` = 255, this overflows at **~4 components worst case** (~16 with modest
  names). An earlier draft claimed it was "bounded in practice"; that was wrong and is
  now corrected with the arithmetic. It also aggregates components into one name,
  departing from the one-container-per-name convention.
- **Generation counters — ruled out.** The `readdir`-derived form is unsafe under
  concurrent `Setup()` (both invocations pick N+1 for different content) and reuses
  numbers after GC; the state-persisted form is safe but adds interface-owned persistent
  state and forfeits revert idempotence.
- **Per-container units adopted as the leading candidate** —
  `<snap>_<slot>_<snaprev>` and `<snap>_<slot>_<snaprev>+<comp>_<comprev>`. At most one
  container per name, matching `compSuffix`, bounded by construction (~149 bytes worst
  case). This reverses an earlier recommendation against per-container, which had been
  argued on churn grounds that did not survive scrutiny: a snap-only refresh renames
  every unit under *either* granularity, so per-container costs nothing there and only
  differs on component-only refreshes.
- **Per-connection + content hash retained as viable** — bounded, one unit, simplest
  reconciliation, but opaque and forfeits `stat`-and-skip.

New open questions raised: **Q6a** (ordering in Go filenames vs. in the manifest — the
latter would make component units truly immutable and reusable across snap refreshes, at
the cost of moving filename construction into C) and **Q11** (parallel-install instance
names contain `_`, which is also the field separator, making names ambiguous to parse
left-to-right — this affects the existing `.library-source` naming too).

### Pass 4 — pivot to a dedicated `export` backend

- Explored replacing the `configfiles`-based approach with a **new `export` security
  backend**, per-interface directory grouping, per-unit content directories and a
  `.sources` manifest as an atomic commit point.
- **Set atomicity** identified as the property Option B could not provide, and the
  reason to prefer a manifest-based design.
- **Deferred GC** established as the answer to the recursive-removal race — safe
  precisely *because* the manifest already excludes stale units.
- Naive `<snap>_<snaprev>` naming **ruled out**: `icd-source` aggregates `$SNAP` and
  `$SNAP_COMPONENT(...)` content, and component revisions move independently.
- **Revision tuple** proposed as a third candidate alongside hash and counter — snap and
  component revisions are immutable, so the tuple determines content exactly as a hash
  does, while staying readable and stateless.
- **Concurrent `Setup()` on the system snap found to be possible** (state lock dropped
  in `setupSecurityByBackend`; conflict checks cover connect/disconnect but not two
  independent refreshes). This makes `readdir`-derived generation counters unsafe and
  favours deterministic naming.
- Reversed the earlier suggestion of reusing `/var/lib/snapd/lib/glvnd/egl_vendor.d/` on
  Core — that path is classic-specific and mixes host with snap content.

### Pass 3 — per-connection → fixed per-interface directories (Option A → B)

- Option A's per-connection directories + `connectedPathPatterns` **rejected**: could not
  clean up on disconnect, because the pattern is only registered while connected.
- Established that fixed, always-registered patterns are what make cleanup work — the
  same guarantee the symlinks backend relies on.
- Consequence at the time: no `interfaces/configfiles/` changes needed at all. (Now
  superseded again by Option C, which needs its own backend for a different
  reconciliation model.)

### Pass 2 — factual corrections against the tree

- **AppArmor prefix** — drafts used `/tmp/snap.rootfs_*/…`; the real prefix is
  `/tmp/snap-private-tmp/snap.rootfs_*/…` (commit `02cf64b882`). Rules with the old
  prefix grant nothing.
- **Packaging path** — `packaging/ubuntu-26.10/` does not exist; it is `ubuntu-26.04`.
  Also downgraded from required to optional, since `/var/lib/snapd/export` is not
  pre-created by packaging either.
- **Vulkan explicit layers** — a draft claimed deferring them kept "parity with
  classic"; classic already supports them, so deferring is a feature reduction.
- **Spread test structure** — a draft described `check_config_ok`/`check_no_config`
  helpers; those exist in the older classic comps test, not in the landed Core test,
  which is a flat script.
- **Spread test filename** — a draft asserted a `15_…` ICD file that cannot exist,
  because the `libs-provider-core26` fixture has no `egl.d/` directory.

### Pass 1 — constraints and traps discovered

- `EnsureDirStateGlobs` **hard-validates** content basenames against the registered glob
  and aborts setup on mismatch (`osutil/syncdir.go:132-142`).
- `dirs` package/local **shadowing** inside `symlinksForSourceDir`.
- `sc_copy_file()` **`die()`s on `ENOENT`**, killing snap launches — reachable on
  classic today.
- The **NVIDIA guard bug is real in production** — `NVIDIA_MULTIARCH` is enabled in the
  shipped snapd snap, so the entire snap-exported-paths pipeline is dead on Core devices
  without an NVIDIA module.
- The **`configfiles` backend runs on Core** (`interfaces/backends/backends.go:58`,
  registered unconditionally) — which is what makes the Phase 1 `.library-source`
  behaviour work at all.
- The landed Phase 1 spread test **executes on UC26 in CI**.
- Recommended **splitting the snap-confine rename from the behaviour change**.
- Documented the **residual classic-side gap**: ICD config copying stays behind the
  NVIDIA check even after the guard inversion.
