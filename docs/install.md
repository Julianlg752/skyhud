# Installing SkyHUD

SkyHUD has two parts: a small program (`skyhud`) that reads your camera files, and a panel inside DaVinci Resolve. The install script puts both in place. It takes about a minute.

## macOS

1. Download the zip for your Mac:
   - Apple silicon (M1 and later): `SkyHUD-<version>-macOS-arm64.zip`
   - Intel: `SkyHUD-<version>-macOS-x64.zip`

   Not sure? Apple menu > About This Mac: "Chip: Apple M…" means Apple silicon.
2. Double-click the zip to unpack it, then open the `SkyHUD-<version>-…` folder.
3. Double-click **Install SkyHUD.command**.

   SkyHUD isn't signed by Apple yet, so the first time macOS says it "cannot be opened because the developer cannot be verified" or "Apple could not verify…". Then:
   - **Right-click** (or Control-click) **Install SkyHUD.command** and choose **Open**, then **Open** again in the dialog.
   - On macOS 15 and later, if there is no Open button: click **Done**, open **System Settings > Privacy & Security**, scroll down to the message about "Install SkyHUD.command" and click **Open Anyway**, then double-click the file again.
4. A Terminal window opens and installs SkyHUD. If you fly a DJI Avata 2, it asks for your DJI API key (see [DJI key](#dji-api-key-avata-2-only)); press Return to skip.
5. Restart DaVinci Resolve. The panel is at **Workspace > Scripts > SkyHUD**.

You can delete the zip and the folder afterwards. SkyHUD keeps its own copy in `~/Library/Application Support/SkyHUD/bin/`.

## Windows

1. Download `SkyHUD-<version>-Windows-x64.zip`.
2. Right-click the zip > **Extract All…**, then open the extracted folder. (Running the installer from inside the zip without extracting does not work.)
3. Double-click **Install SkyHUD.bat**.

   SkyHUD isn't signed yet, so Windows SmartScreen may say "Windows protected your PC". Click **More info**, then **Run anyway**.
4. A window opens and installs SkyHUD. If you fly a DJI Avata 2, it asks for your DJI API key; press Enter to skip.
5. Restart DaVinci Resolve. The panel is at **Workspace > Scripts > SkyHUD**.

SkyHUD keeps its own copy in `C:\ProgramData\SkyHUD\bin\`, so you can delete the download afterwards.

## DJI API key (Avata 2 only)

The HUD works from the Avata 2's own video without any key: speed, height, horizon, flight path and an estimated g-force. Stick positions and battery come from the DJI Fly flight logs on your phone, and newer logs are encrypted. To unlock them SkyHUD needs your own DJI developer API key:

1. Sign in at [developer.dji.com](https://developer.dji.com/) with your DJI account.
2. Click **Create App**, choose **Open API** as the app type, and fill in a name, category and description (anything sensible).
3. Activate the app with the link DJI emails you.
4. On your developer page, open the app and copy its **App Key**.
5. Paste it when the install script asks, or later in the SkyHUD panel (**Save key**).

Where it goes: your system keychain (macOS Keychain, Windows Credential Manager), never a file. SkyHUD sends it only to DJI's key service, once per encrypted log; the unlocked keys are cached on your computer so a log is never sent twice. From a terminal: `skyhud key status` says whether a key is stored, and `skyhud key set` reads a new one from standard input.

Getting your flight logs onto the computer: they are the `DJIFlightRecord_*.txt` files in the DJI Fly app's folder on your phone (on Android: `Android/data/dji.go.v5/files/FlightRecord`; on iPhone: connect it to your computer and copy the FlightRecords folder from DJI Fly's files in Finder or the Apple Devices app). Put them all in one folder and choose that folder in the panel.

## Updating

Download the new zip and run its install script again. It replaces the program and the panel; your designs, settings and key stay.

## Uninstalling

Delete these, then restart Resolve:

| | macOS | Windows |
|---|---|---|
| Panel | `~/Library/Application Support/Blackmagic Design/DaVinci Resolve/Fusion/Scripts/Utility/SkyHUD.lua` | `%APPDATA%\Blackmagic Design\DaVinci Resolve\Support\Fusion\Scripts\Utility\SkyHUD.lua` |
| Program, designs, settings | `~/Library/Application Support/SkyHUD/` | `C:\ProgramData\SkyHUD\` |
| Unlocked log keys (cache) | `~/Library/Caches/SkyHUD/` | `%LOCALAPPDATA%\SkyHUD\` |
| DJI key | Keychain Access, item "SkyHUD" | Credential Manager > Windows Credentials, "SkyHUD/dji-api-key" |

HUD overlays already in your projects stay until you remove them (**Remove all HUDs** in the panel, or delete the SkyHUD track).

## Building from source

With Go 1.26 or later: `go build -o skyhud ./cmd/skyhud`, then `./skyhud install` (copies the program into place) or `./skyhud install --here` (points the panel at this build, for development).
