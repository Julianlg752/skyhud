# Contributing

Thanks for helping. Bug reports, ideas and pull requests are all welcome.

## Before you start

For anything bigger than a small fix, open an issue first so we can agree on the approach.

## Making a change

1. Fork the repository and make a branch.
2. Install the commit check once: `sh scripts/check-private.sh --install` (install [gitleaks](https://github.com/gitleaks/gitleaks) too; the check uses it to find secrets).
3. Make your change with tests. Run `go test ./...` and `sh scripts/test-check-private.sh`.
4. Open a pull request against `main` and fill in the template.

## Rules

- **Never commit real flight data.** No videos, flight logs, decoded logs, `.DAT` files, DJI keys, GPS coordinates, serial numbers, or real file names, dates or places, from you or anyone else. Tests build made-up data in code.
- **Go standard library only.** A new dependency needs a good reason, discussed first.
- **No network calls** without discussing them first.
- The HUD stays editable in Resolve: never burned into the video.
- Go code builds without cgo for macOS (arm64, amd64) and Windows (amd64).

## What happens to your pull request

- The checks run (tests, builds, and a scan for flight data and secrets). On your first pull request they wait until the maintainer starts them.
- The maintainer reviews every change before it's merged. Changes to code that runs commands, installs files, or checks designs get the closest look.
- Commits are rebased onto `main` when merged, so keep each commit tidy.

By contributing, you agree your work is released under the [MIT license](LICENSE).
