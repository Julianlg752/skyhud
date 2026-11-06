# SkyHUD

A DaVinci Resolve plugin that turns the flight data hidden in your drone and action camera files into an animated, editable HUD: speed, height, flight path, horizon, flips, g-force, sticks and battery.

Most pilots post their goggles' OSD as it is. SkyHUD lets you edit and grade your footage first, then add a polished HUD on its own track, in one of four designs or your own. The grade never touches the HUD, and the HUD is never burned into the video: you can move, resize, restyle or remove it at any time.

- **Cameras:** GoPro (HERO with GPS; tested on a HERO10 Black, Gyroflow exports included) and DJI Avata 2. Betaflight blackbox logs come after the first release.
- **Resolve:** free and Studio on macOS (Apple silicon and Intel) and Windows. Tested so far on Resolve Studio 21 on macOS; Windows and free Resolve checks are in progress.
- **Designs:** Goggles, Race, Fighter jet and Clean, plus your own, with or without Fusion.

## Quick start

1. Download the zip for your computer and run the install script inside it ([install guide](docs/install.md)).
2. Restart Resolve, edit and grade as usual.
3. Open **Workspace > Scripts > SkyHUD**, pick a design and press **Add HUD to all shots**.

Your first HUD in five minutes, Avata 2 flight logs, layouts, speed ramps and troubleshooting: [usage guide](docs/usage.md).

Make your own design: [design guide](docs/designs.md).

## What each camera gives

| | GoPro | DJI Avata 2 | Avata 2 + flight log |
|---|---|---|---|
| Speed | GPS | aircraft | aircraft |
| Height | GPS, rough (±10 m) | aircraft, above takeoff | aircraft |
| Horizon, flips | camera gravity | aircraft attitude | aircraft attitude |
| G-force | measured | estimated (labelled EST) | estimated |
| Flight path | GPS | aircraft | aircraft |
| Sticks, battery | – | – | from the log |

## Privacy

Original GoPro and DJI files and DJI flight logs contain GPS positions and serial numbers. Never attach them to issues or pull requests. SkyHUD's HUD data leaves out GPS coordinates; the flight path is drawn from relative meters, but its shape can still show where you flew, so turn it off when that matters.

Your DJI API key stays in your system keychain (macOS Keychain, Windows Credential Manager) and is only sent to DJI's own key service, once per encrypted log.

## Building from source

Go 1.26 or later, standard library only, no cgo.

```sh
go test ./...
go build -o skyhud ./cmd/skyhud
./skyhud install --here   # point Resolve's panel at this build
sh scripts/release.sh 0.1.0   # release zips in dist/
```

See [CHANGELOG.md](CHANGELOG.md) for what changed. License: MIT.
