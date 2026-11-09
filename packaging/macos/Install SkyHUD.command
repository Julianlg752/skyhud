#!/bin/bash
# Installs SkyHUD for DaVinci Resolve. Double-click this file in Finder
# (the first time: System Settings > Privacy & Security > Open Anyway,
# see docs/install.md).
set -e
cd "$(dirname "$0")"

# The zip came from the internet; let macOS run the skyhud program in it.
xattr -d com.apple.quarantine skyhud 2>/dev/null || true
chmod +x skyhud

echo "Installing SkyHUD..."
./skyhud install
echo

if ./skyhud key status | grep -q "not stored"; then
	echo "Flying a DJI Avata 2? SkyHUD can add sticks and battery from your"
	echo "flight logs with your own DJI API key (see docs/install.md)."
	read -r -s -p "Paste your DJI API key and press Return (or just press Return to skip): " key || key=""
	echo
	if [ -n "$key" ]; then
		printf '%s\n' "$key" | ./skyhud key set
	fi
	unset key
fi

echo
echo "Done. Restart DaVinci Resolve, then open Workspace > Scripts > SkyHUD."
read -r -p "Press Return to close this window." _ || true
