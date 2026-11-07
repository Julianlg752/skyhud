# Security

SkyHUD runs on your computer inside DaVinci Resolve, reads your video files and flight logs, and can store a DJI API key. If you find a way it could run code you didn't ask for, leak a key, or expose data from someone's flights, please tell us privately.

## Report a problem

Use **[Report a vulnerability](https://github.com/Julianlg752/skyhud/security/advisories/new)** on this repository (Security tab). Only the maintainer sees the report.

Please don't open a public issue for security problems.

Include what you found, how to trigger it, and which SkyHUD version, operating system and Resolve version you used. You'll get an answer within a week.

## Supported versions

Only the latest release gets fixes.

## What we care about most

- Code from a design file, a video, a flight log or the panel running commands it shouldn't.
- The DJI key ending up anywhere other than where you saved it (the key file or the system keychain).
- GPS positions or other personal data being written out when you didn't ask for them.
