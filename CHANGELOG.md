# Changelog

## Unreleased

**Fixes**
- The SkyHUD panel now uses two columns and fits on a laptop screen. Before, the window was taller than a 14-inch MacBook screen: the "Scan a flight folder…" button and the message box at the bottom were cut off, and the hint under "Use this shot's layout for all" was squashed.

## 0.1.1 (2026-11-06)

**Fixes**
- DJI flight logs line up much better: the offset comes from ground speed and is fine-tuned by matching the video's roll with the aileron stick, instead of from height alone (which could be seconds off).
- Logs whose time stamps lag or jump (seen on version 14 logs) no longer leave holes in sticks and battery: frame times come from the log's flight time counter.
- A log that starts a few seconds after the video (the goggles started recording first) is now used.
- A log that only lines up at the edge of the ±5 s search is no longer used with a wrong offset; the panel says why.
- Goggles and Race no longer show an empty box where the flip badge goes, and their stick and battery backgrounds fade out cleanly when there's no data.

## 0.1.0 (2026-11-06)

First release: GoPro and DJI Avata 2 footage in DaVinci Resolve (free and Studio) on macOS and Windows.

**HUD on the timeline**
- One HUD overlay per shot on its own SkyHUD track, above the graded footage; never burned in.
- Follows trims, speed changes and speed ramps.
- Pieces: speed, height, g-force, horizon (with pitch), flight path, stick positions, battery and a flip badge.
- A red Flip marker on the timeline at every flip.

**Designs**
- Four built-in designs: Goggles, Race, Fighter jet and Clean.
- Each design remembers its own layout; drag pieces on the Fusion page and apply the layout to all shots.
- HUD size slider, per-piece toggles, and color pickers and font per design that update the HUD live.
- Your own designs: save from the panel with your colors, font and layout, or build one in Fusion (see docs/designs.md). Designs are checked so they can't run code.
- `skyhud design list|check|save|preview`.
- The download zips include copies of the four designs to start your own from.

**Cameras**
- GoPro HERO with GPS (tested on HERO10 Black): speed, GPS height, horizon, g-force, flight path. Gyroflow exports read their original.
- DJI Avata 2 from the video alone: speed, height above takeoff, attitude, flight path, estimated g-force (labelled EST).
- DJI flight logs (optional, with your own DJI API key): stick positions and battery, matched and lined up automatically. The key lives in the system keychain.

**Tools**
- `skyhud scan <folder>`: what each video holds and what's missing.
- Install scripts for macOS and Windows; `skyhud install` keeps its own copy so the download can be deleted.
- `skyhud version`.

**Privacy**
- HUD data never holds GPS coordinates (unless `--keep-location` on `skyhud export`); the flight path uses relative meters.

**Known limits**
- Not signed: macOS and Windows show a warning the first time (see docs/install.md).
- Windows and free Resolve are still being tested.
- Smooth (curved) speed ramps are followed as straight lines between their points.
- GoPro height comes from GPS and is rough (about ±10 m).
- Betaflight blackbox logs come in a later release.
