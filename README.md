# BranchKit System

Application registry, app launching, and audio device control for
[BranchKit](https://branchkit.dev). MIT licensed.

## What it provides

**Actions** (`system.*`): `launch`, `new_window`, `open`, `volume_up`,
`volume_down`, `mute`, `unmute`, `set_output`, `set_input`.

**Collections**: `apps` (the installed-application registry other plugins read
to resolve a spoken app name to a bundle id), `audio_outputs`, `audio_inputs`,
`plugin.system.device_aliases`, and `plugin.system.config`.

**Settings tabs**: Apps, Sound, Devices.

It subscribes to `_platform.audio_devices.changed`,
`_platform.system.did_wake`, and `_platform.collection.updated` — device lists
are re-read on wake because macOS reports stale devices otherwise.

Requires `apps`, `apps.control`, `audio`, `filesystem`, `input`, and `display`.
macOS.

## Reading this as an example

This is a **reference implementation, not a tutorial.** It is a real shipped
plugin, so it carries what real plugins carry: OS quirk workarounds, comments
about bugs that took a day to find, and decisions that only make sense against
a specific failure. Read it to see how the platform is actually used.

Worth reading here specifically: `src/config.go` shows the settings-preset
pattern — the manifest declares fields with defaults, the platform materializes
the composed view, and the plugin only ever reads it. A plugin never writes its
own settings collection; user gestures relay through `overrides.apply`.

For idiom, read
[branchkit-plugin-helloworld-go](https://github.com/branchkit/branchkit-plugin-helloworld-go)
or scaffold with `branchkit-cli dev init`. Those are curated to teach. This is
not.

## Build

```bash
cd src && go build -o ../system-plugin .
```

Install into a running BranchKit:

```bash
branchkit-cli plugin install . --build
```

## Platform documentation

```bash
branchkit-cli docs sync
grep -rl "collection" "$(branchkit-cli docs path)"
```
