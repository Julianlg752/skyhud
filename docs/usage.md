# Using SkyHUD

## Your first HUD in five minutes

1. In Resolve, put your clips on a timeline (video track 1, say), edit and grade as usual. Use the camera's original files, or Gyroflow exports with the original next to them (see [GoPro and Gyroflow](#gopro-and-gyroflow)).
2. Open **Workspace > Scripts > SkyHUD**. The panel opens in its own window; it works on any page.
3. Under **Shots on video track**, pick the track with your clips.
4. Under **Design**, pick Goggles, Race, Fighter jet or Clean.
5. Press **Add HUD to all shots**.

SkyHUD reads each clip's flight data and puts one HUD overlay above every clip of that track, on a new track named **SkyHUD**. Play the timeline: the HUD moves with the footage. Your grade stays on the video and never touches the HUD.

To change anything (design, pieces, size, height reference), change it in the panel and press **Add HUD to all shots** again: the overlays are rebuilt and leftovers removed.

## What the panel does

| Control | What it does |
|---|---|
| **Shots on video track** | The track whose clips get a HUD. The SkyHUD track always sits above it. |
| **Read the shot under the playhead** | Shows what SkyHUD finds in the clip under the playhead: camera, length, which data it has, Gyroflow original, flight log. |
| **Design** | The HUD's look. Your own designs appear here too. |
| **HUD size** | Scales the whole HUD, 50 % to 200 %. |
| **Units** | "km/h, m" or "mph, ft" for speed and height. HUDs already on the timeline update. |
| **Colors** | One color picker per color of the chosen design (Main, Accent, …), a **Font** choice (the design's own, Mono or Sans) and **Reset colors**. Each design keeps its own colors; the built-in design itself never changes. HUDs already on the timeline update half a second after you pick a color. |
| **Show** | Turn pieces on or off: Speed, Height, G-force, Horizon, Flight path, Stick positions, Battery, Flip badge. "(not in this design)" means the design has no such piece. |
| **Height** | "Above takeoff" (from the start of the recording, never below 0) or "Above sea level". GoPro height comes from GPS and is rough (about ±10 m). |
| **Choose logs folder…** | DJI Avata 2 only: the folder with your `DJIFlightRecord_*.txt` files, for stick positions and battery. |
| **Save key** | Stores your DJI API key in the system keychain. |
| **Add HUD to all shots** | Builds or rebuilds every overlay on the track. |
| **Update this shot** | Rebuilds only the overlay of the clip under the playhead. |
| **Use this shot's layout for all** | Takes the piece positions you dragged on one overlay and applies them to all (see [Moving pieces](#moving-pieces)). |
| **Sync nudge for this file (s)** | Moves one file's data in time if it looks out of step: if the HUD reacts late, enter a positive number of seconds; if it reacts early, a negative one. |
| **Remove all HUDs** | Deletes every SkyHUD overlay on the timeline. |
| **Save as new design** | Saves the current design with the colors, font and layout you picked under a new name (see the [design guide](designs.md)). |
| **Scan a flight folder…** | Lists every video in a folder with its data, and what's missing (see [Scanning a folder](#scanning-a-folder)). |

## GoPro and Gyroflow

GoPro HERO cameras with GPS record speed, GPS height, the camera's gravity direction (horizon and flips), accelerations (g-force) and the flight path. GPS needs a lock: where the camera had none, speed reads `--`.

If you stabilize with Gyroflow, keep the original file next to the export, with its original name:

```
GX010123.MP4               <- original: holds the flight data
GX010123_stabilized.mp4    <- the Gyroflow export you edit
```

Only the original holds the flight data. SkyHUD finds it by name and works out where the export starts inside it. If the HUD looks a little early or late, use **Sync nudge for this file**.

## DJI Avata 2 and flight logs

The Avata 2's own video file holds speed, height above takeoff, attitude (horizon, pitch, flips) and position (flight path), and SkyHUD estimates g-force from the speed changes; it's labelled **G (EST)**. No key or internet needed.

Stick positions and battery live in the DJI Fly flight logs on your phone:

1. Copy the `DJIFlightRecord_*.txt` files into one folder on your computer ([where to find them](install.md#getting-your-flight-logs-avata-2-only)).
2. In the panel, **Choose logs folder…** and pick that folder.
3. Store your DJI API key once (**Save key**, or during install).
4. Press **Add HUD to all shots**.

SkyHUD picks the log that belongs to each clip and lines it up by ground speed, then fine-tunes it by matching the video's roll with the aileron stick; the panel says which log it used and the offset it found, e.g. `flight log DJIFlightRecord_2025-06-01_[10-30-00].txt (offset -0.55 s)`.

## Designs

Four designs come built in:

- **Goggles**: white monospace text in the corners, like the pilot's view.
- **Race**: italic numbers, a speed arc, an edge glow that grows with speed, a battery bar and a FLIP! badge.
- **Fighter jet**: green speed and height tapes, a pitch ladder and an INVERTED warning.
- **Clean**: small thin labels along the bottom, for footage that should speak for itself.

Each design remembers its own layout. To make your own, see the [design guide](designs.md).

## Moving pieces

1. Click any SkyHUD overlay on the timeline and open the **Fusion** page.
2. Each piece has a Transform named after it (`Speed`, `Height`, `Horizon`, `Path`, …). Select it and drag it in the viewer, or change its Size.
3. Put the playhead on that overlay and press **Use this shot's layout for all**.

The layout is saved for the current design, so switching designs keeps each one's own layout. Pieces can only be dragged on the Fusion page; on the Edit page the overlay is a normal clip you can trim, fade or disable.

## Speed ramps

Every overlay follows its clip: trims, constant speed changes and speed ramps (retime curves). After you change a clip's speed or ramp, press **Add HUD to all shots** again. Smooth (curved) ramps are followed as straight lines between their points.

## Flip markers

SkyHUD puts a red **Flip** marker on the timeline at every flip it finds, so you can jump between them. They are replaced on every full rebuild and never replace your own markers (where one of yours sits on a flip frame, SkyHUD leaves it).

## Scanning a folder

**Scan a flight folder…** (or `skyhud scan <folder> --logs <logs folder>` in a terminal) lists, for every `.mp4` and `.mov` in a folder: the camera, length, recorded time, which HUD pieces have data, the Gyroflow original, and the matched flight log and offset. Then a list of gaps: videos with no flight data, Gyroflow exports whose original is missing, and flight logs from the same days that match no video. It never shows positions.

## Troubleshooting

| The panel says | What to do |
|---|---|
| Open a timeline first. | Open the timeline you want the HUD on. |
| No clips with a file on video track N. | Pick the track your clips are on under **Shots on video track**. |
| The SkyHUD track must be above the shots' track | Move the SkyHUD track above your clips, or delete it and press **Add HUD to all shots**. |
| skipped …: no GoPro or DJI flight data in this file | The clip has no flight data (a phone clip, an export, or a GoPro without GPS). If it's a Gyroflow export, put the original next to it. |
| …the original camera file … was not found next to the stabilized clip | Copy the original GoPro or DJI file into the same folder as the Gyroflow export, with its original name. |
| no matching flight log | No log in the folder covers this clip. Check the folder holds the log from that flight. |
| no DJI key stored | Store your key with **Save key**. |
| flight log … could not be unlocked | The key was refused, or there's no internet. Check the key and try again; once a log is unlocked it works offline. |
| flight log … does not line up with the video | The log is from that time but the heights don't match; it may be another flight. |
| flight log has no stick positions (motion controller?) | Flights with the motion controller record no sticks. Battery still shows. |
| design … was refused: … | A custom design failed the safety check; SkyHUD used Goggles instead. Run `skyhud design check <folder>` to see why. |
| Put the playhead over a SkyHUD overlay first. | For **Use this shot's layout for all**, the playhead must be on an overlay. |
| Speed shows `--` | The GoPro had no GPS lock there. |

Still stuck? Open an issue with the panel's message, your camera model and Resolve version. Never attach original videos or flight logs: they contain GPS positions.
