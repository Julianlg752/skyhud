# Tutorial video scripts

Three short screen recordings for the release page and the docs. Each script lists the shots in order with what to say. Record at 1920×1080 or larger, with Resolve's UI at normal size.

**Before recording:** use footage you're happy to publish. Your own sample clips show your home, locations and a person; don't use them. Hide the Resolve project list and recent files if they show private names. Never show your DJI key: paste it from the clipboard while the field is off screen, or blur it afterwards.

## 1. Install SkyHUD (about 2 minutes)

| # | Shot | Say |
|---|---|---|
| 1 | Finished video with the Race HUD playing (5 s) | "This is SkyHUD: a HUD for your FPV and action camera footage, made from the flight data already inside your files. Let's install it." |
| 2 | Browser on the release page, click the macOS zip | "Download the zip for your computer. On a Mac with an M chip, take the arm64 one." |
| 3 | Finder: double-click the zip, open the folder | "Unzip it and open the folder." |
| 4 | Right-click Install SkyHUD.command > Open > Open | "SkyHUD isn't signed by Apple yet, so the first time, right-click the installer and choose Open." |
| 5 | Terminal: install output, key prompt, press Return | "It copies SkyHUD into place and adds the panel to Resolve. If you fly a DJI Avata 2, it can store your DJI key here; I'll skip that for now." |
| 6 | Windows: Extract All, Install SkyHUD.bat, SmartScreen More info > Run anyway (10 s) | "On Windows it's the same: extract the zip, run Install SkyHUD, and if SmartScreen appears, choose More info, then Run anyway." |
| 7 | Resolve: Workspace > Scripts > SkyHUD, panel opens | "Restart Resolve and open Workspace, Scripts, SkyHUD. That's it." |

## 2. Your first HUD (about 4 minutes)

| # | Shot | Say |
|---|---|---|
| 1 | Edit page: timeline with a few GoPro or Avata 2 clips, already graded | "Edit and grade your video first, as you always do." |
| 2 | Media folder: GX…MP4 next to GX…_stabilized.mp4 | "If you use Gyroflow, keep the original file next to the stabilized one. The original holds the flight data." |
| 3 | Open the SkyHUD panel, pick the track, pick Race, press Add HUD to all shots | "Open SkyHUD, choose the track with your shots, a design, and press Add HUD to all shots." |
| 4 | Timeline: new SkyHUD track appears; play | "SkyHUD adds one overlay per shot on its own track. Your grade never touches it, and it's not burned in." |
| 5 | Switch designs: Goggles, Fighter jet, Clean, each followed by Add HUD to all shots | "Four designs come built in. Switching takes a click." |
| 6 | Show toggles and HUD size | "Turn pieces on or off, and make the whole HUD bigger or smaller." |
| 7 | Fusion page: drag the Speed transform; back in the panel press Use this shot's layout for all | "To move a piece, drag it on the Fusion page, then press Use this shot's layout for all." |
| 8 | Speed ramp a clip, press Add HUD to all shots, play the ramp | "Speed ramps? The HUD follows them." |
| 9 | Timeline markers at a flip, play it | "Every flip gets a red marker, so you can find your best moments." |
| 10 | Avata 2 only: Choose logs folder…, Save key, Add HUD to all shots, sticks moving | "With a DJI Avata 2, add your flight logs folder for stick positions and battery. SkyHUD finds the right log and lines it up by itself." |
| 11 | Deliver page render, then the result | "Render as usual." |

## 3. Make your own design (about 4 minutes)

| # | Shot | Say |
|---|---|---|
| 1 | Panel: pick Race, set HUD size, move a piece, Use this shot's layout for all | "Start from any design and set it up the way you like." |
| 2 | Save as new design: name, main color, accent, font, Save | "Under Save as new design, give it a name, colors and a font, and save. No Fusion needed." |
| 3 | Design menu shows the new design "(yours)"; Add HUD to all shots | "It's now in your design list." |
| 4 | Finder: the designs folder with design.json and design.setting | "Every design is a folder: design.json for names, places and colors, and design.setting for the Fusion tools." |
| 5 | Fusion page: a Text+ node, its expression reading HUDData.NumberIn1 | "In Fusion, any tool can read SkyHUD's data. Speed is HUDData's first input." |
| 6 | Copy nodes, paste into design.setting, run skyhud design check | "Copy your nodes into design.setting, name them after the piece, and check the design." |
| 7 | Design guide page in the browser | "The design guide lists every value and the rules. Share your designs with other pilots: a design is just a folder." |
