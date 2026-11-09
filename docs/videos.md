# Tutorial video scripts

Five short screen recordings for the release page and the docs. Videos 4 and 5 already exist as simplified, captioned versions (built from drawn screens, not real recordings); record real ones with these scripts if the simplified screens drift from DJI's. Each script lists the shots in order with what to say. Record at 1920×1080 or larger, with Resolve's UI at normal size.

**Before recording:** use footage you're happy to publish. Your own sample clips show your home, locations and a person; don't use them. Hide the Resolve project list and recent files if they show private names. Never show your DJI key: paste it from the clipboard while the field is off screen, or blur it afterwards.

## 1. Install SkyHUD (about 2 minutes)

| # | Shot | Say |
|---|---|---|
| 1 | Finished video with the Race HUD playing (5 s) | "This is SkyHUD: a HUD for your FPV and action camera footage, made from the flight data already inside your files. Let's install it." |
| 2 | Browser on the release page, click the macOS zip | "Download the zip for your computer. On a Mac with an M chip, take the arm64 one." |
| 3 | Finder: double-click the zip, open the folder | "Extract the zip and open the folder." |
| 4 | Double-click Install SkyHUD.command, macOS blocks it, Done; System Settings > Privacy & Security, scroll down, Open Anyway, Open Anyway again, password | "SkyHUD isn't signed by Apple yet, so the first time macOS blocks it. Open System Settings, Privacy & Security, scroll down and click Open Anyway." |
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

## 4. Get your flight logs from DJI Fly (about 2 minutes)

For DJI Avata 2 pilots who want sticks and battery. No narration needed: the "Say" column works as on-screen captions.

**Privacy:** log names hold the date and time of each flight, and DJI Fly's flight list shows maps and places. Don't film the flight list in the app; blur file names in Finder or Explorer, or show only a folder of logs you're happy to publish.

| # | Shot | Say |
|---|---|---|
| 1 | Avata 2 clip with the Goggles HUD, sticks moving (5 s) | "Stick positions and battery come from your DJI flight logs. Here's how to get them onto your computer." |
| 2 | Phone: DJI Fly open, connected to the goggles after a flight | "DJI Fly saves a log for every flight. Open it with your goggles connected after flying, so the latest flights reach your phone." |
| 3 | iPhone: Mac Finder, iPhone in the sidebar, Files tab | "iPhone: connect it to your computer with a cable and open it in Finder (or the Apple Devices app on Windows). Go to Files." |
| 4 | Expand DJI Fly, drag the FlightRecords folder to Documents | "Open DJI Fly and drag the FlightRecords folder to your computer." |
| 5 | Android: Windows File Explorer, phone > Internal storage > Android > data > dji.go.v5 > files > FlightRecord, copy the folder | "Android: connect it, allow file transfer, and copy the FlightRecord folder from Android, data, dji.go.v5, files." |
| 6 | Finder: the folder full of DJIFlightRecord_….txt files (names blurred) | "Keep all your logs in one folder. Copy it again after new flights; SkyHUD finds the right log for each clip." |
| 7 | SkyHUD panel: Choose logs folder…, pick the folder, Add HUD to all shots, sticks moving | "In the SkyHUD panel, choose that folder and add the HUD. Newer logs are encrypted: you'll need your DJI key (next video)." |

## 5. Get your DJI API key (about 2 minutes)

Newer DJI logs are encrypted. A free DJI developer key unlocks them; you set it up once.

**Privacy:** never show the key. Blur the SDK key, your email and your account name, or cover them while recording.

| # | Shot | Say |
|---|---|---|
| 1 | Title over an Avata 2 clip with sticks (5 s) | "Newer DJI flight logs are encrypted. A free DJI developer key unlocks them. You only do this once." |
| 2 | Browser: developer.dji.com, Log in with the DJI account | "Go to developer.dji.com and log in with your DJI account. The first time, DJI may ask you to fill in a short developer profile." |
| 3 | User Center > Apps > Create App | "Open your user center, go to Apps and click Create App." |
| 4 | Form: App Type Open API, name SkyHUD, a category and a short description, Create | "Choose Open API as the app type. Any name and description will do." |
| 5 | Email: DJI's activation email, click the link | "DJI emails you an activation link. Click it." |
| 6 | Apps list: open the app, copy the SDK key (blurred) | "Open your new app and copy its SDK key." |
| 7 | SkyHUD panel: paste in the key field, Save key, status says the key is stored | "Paste it in the SkyHUD panel and press Save key. It stays in your system keychain, and SkyHUD sends it only to DJI's key service, once per log." |
| 8 | Add HUD to all shots on an Avata 2 clip with a log folder, sticks and battery moving | "That's it: sticks and battery, from your own logs." |
