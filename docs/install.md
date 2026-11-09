# Installing SkyHUD

SkyHUD has two parts: a small program (`skyhud`) that reads your camera files, and a panel inside DaVinci Resolve. The install script puts both in place. It takes about a minute.

## macOS

1. Download the zip for your Mac:
   - Apple silicon (M1 and later): `SkyHUD-<version>-macOS-arm64.zip`
   - Intel: `SkyHUD-<version>-macOS-x64.zip`

   Not sure? Apple menu > About This Mac: "Chip: Apple M…" means Apple silicon.
2. Double-click the zip to extract it, then open the `SkyHUD-<version>-…` folder.
3. Double-click **Install SkyHUD.command**.

   SkyHUD isn't signed by Apple yet, so the first time macOS blocks it with a message like "Apple could not verify "Install SkyHUD.command" is free of malware" or "cannot be opened because the developer cannot be verified". To allow it:
   1. Click **Done** (or **OK**) to close the message.
   2. Open **System Settings > Privacy & Security**.
   3. Scroll down to the **Security** section. Next to "Install SkyHUD.command was blocked…", click **Open Anyway**.
   4. In the message that appears, click **Open Anyway** again and enter your Mac password if asked.

   You only do this once. On macOS 14 and earlier there is a shortcut: right-click (or Control-click) the file, choose **Open**, then **Open** again.
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

Video guide (1 minute): [Get your free DJI API key](https://julianlg752.github.io/skyhud/#guides).

The HUD works from the Avata 2's own video without any key: speed, height, horizon, flight path and an estimated g-force. Stick positions and battery come from the DJI Fly flight logs on your phone, and newer logs are encrypted. To unlock them SkyHUD needs your own DJI developer API key:

1. Sign in at [developer.dji.com](https://developer.dji.com/) with your DJI account.
2. Click **Create App**, choose **Open API** as the app type, and fill in a name, category and description (anything sensible).
3. Activate the app with the link DJI emails you.
4. On your developer page, open the app and copy its key (DJI labels it **SDK key** or App Key).
5. Paste it when the install script asks, or later in the SkyHUD panel (**Save key**).

Where it goes: your system keychain (macOS Keychain, Windows Credential Manager), never a file. SkyHUD sends it only to DJI's key service, once per encrypted log; the unlocked keys are cached on your computer so a log is never sent twice. From a terminal: `skyhud key status` says whether a key is stored, and `skyhud key set` reads a new one from standard input.

## Getting your flight logs (Avata 2 only)

Video guide (1 minute): [Get your flight logs from DJI Fly](https://julianlg752.github.io/skyhud/#guides).

DJI Fly saves a `DJIFlightRecord_*.txt` log for every flight. Open DJI Fly with your goggles connected after flying, so the latest flights reach your phone. Then copy them to your computer:

- **iPhone:** connect it with a cable and open it in Finder (on Windows: the Apple Devices app). Go to **Files**, open **DJI Fly** and drag the **FlightRecords** folder to your computer.
- **Android:** connect it and allow file transfer, then copy `Android/data/dji.go.v5/files/FlightRecord` from the phone's internal storage.

Keep all the logs in one folder and choose it in the panel (**Choose logs folder…**). Copy the folder again after new flights: SkyHUD finds the log that belongs to each clip.

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
