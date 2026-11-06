# CLAUDE.md

SkyHUD: a DaVinci Resolve plugin that turns flight telemetry from drone and action camera files into an animated, editable HUD. A Go core reads the files and writes HUD data; the Resolve side (panel and Fusion designs) uses that data.

## Commands

- Tests: `go test ./...` and `sh scripts/test-check-private.sh`
- Install the commit check after cloning: `sh scripts/check-private.sh --install` (install gitleaks too: the check uses it for secrets).
- Real-file tests are skipped unless env vars point at local samples (`SKYHUD_GOPRO`, with its Gyroflow export next to it; `SKYHUD_AVATA`, `SKYHUD_DJI_LOGS`, `SKYHUD_DJI_DECODED`). Optional expected values: `SKYHUD_GOPRO_TRIM`, `SKYHUD_AVATA_LOG`, `SKYHUD_AVATA_OFFSET`.

## Rules

- Spec and plans live in `docs/superpowers/` (kept out of the public repo, see .gitignore). The spec is the authority.
- Go standard library only unless a dependency is approved first. Never use or install `axios`. Any network call needs approval first.
- Never commit real videos, flight logs, decoded logs, `.DAT` exports, keys, or GPS coordinates from real flights. Tests use synthetic data built in code.
- Never print or log a user's DJI key.
- HUD data files leave out absolute GPS coordinates unless the user passes `--keep-location`.
- Label estimated values as estimated.
- The HUD must stay editable in Resolve, not burned into the video.
- Go code builds without cgo for macOS (arm64, amd64) and Windows (amd64).

## Commits (this repo is public)

Every commit and push is public and stays public. Be extra careful:

- Before each commit, read the whole staged diff (`git diff --cached`) and the commit message. The pre-commit check (`scripts/check-private.sh`) must pass; never skip it (`--no-verify`).
- Commit only with a GitHub noreply email (`<id>+<user>@users.noreply.github.com`); the check refuses other addresses.
- No real data from anyone's flights anywhere in git (code, tests, docs, commit messages): no real file or log names, flight dates or times, places, serial numbers, coordinates, usernames or home paths. Tests use made-up names, dates and data.
- Private words (places, names, serials) go in `~/.config/open-hud/private-words`, one per line, outside the repo; the check refuses commits that contain them.
- Planning notes, research and spikes stay out of git (`docs/superpowers/`, `docs/research/`, `spikes/` are ignored).
- Never push, force-push, retag or rewrite published history without the owner's OK.
