# Release checklist

Publishing is Julian's call. Nothing here pushes or publishes on its own.

## Before the first public release

- [ ] **Secret and privacy scan of the whole history**, not just the current tree:
  - `git log --all -p | grep -iE 'api[-_ ]?key|secret|password|token'` and look at every hit.
  - `git log --all --name-only --format= | sort -u` and check no video, flight log, decoded log, `.DAT`, key or sample file was ever committed.
  - `git log --all -p | grep -E '[0-9]{1,3}\.[0-9]{5,}'` and check no real GPS coordinates appear (tests use made-up ones).
  - `sh scripts/check-private.sh` passes.
  - If anything turns up: rewrite history before the repo goes public, or start a fresh repo from a clean tree.
- [ ] No personal paths or names in docs or code (`git grep -n -e "$USER" -e "/Users/" -e "/Volumes/"`, then look at every hit).
- [ ] The DJI key never appears in logs, errors or test output (`skyhud key status` prints only "stored").

## Every release

- [ ] `go test ./...` and `sh scripts/test-check-private.sh` pass.
- [ ] Real-file tests pass locally with the env vars listed in CLAUDE.md set.
- [ ] Cross builds: `sh scripts/release.sh <version>` writes three zips in `dist/`, each with a `designs` folder of four design copies.
- [ ] Unzip the macOS zip on a Mac, run Install SkyHUD.command, add a HUD in Resolve.
- [ ] Unzip the Windows zip on Windows, run Install SkyHUD.bat, add a HUD in Resolve (free Resolve too).
- [ ] Fill each built-in design on a GoPro and an Avata 2 timeline, play a speed ramp and a flip.
- [ ] Update `CHANGELOG.md`: version, date, changes, known limits.
- [ ] Tag: `git tag -a v<version> -m "SkyHUD <version>"`.
- [ ] Release notes: the CHANGELOG section, the three zips, links to docs/install.md and docs/usage.md, the unsigned-app note.
- [ ] Sample stills or videos in the release only from footage that is fine to publish (no home, no faces, no locations).
- [ ] Push the tag and publish the release (Julian).
- [ ] Landing page: set `VERSION` near the bottom of `site/index.html` to the new version and push to main (the Pages workflow republishes it). The Download button links straight to that version's zips.
