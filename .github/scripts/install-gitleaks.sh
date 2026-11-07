#!/bin/sh
# Installs the pinned gitleaks release on a Linux CI runner, after checking its SHA-256.
set -eu
f="gitleaks_${GITLEAKS_VERSION}_linux_x64.tar.gz"
curl -fsSLo "$RUNNER_TEMP/$f" "https://github.com/gitleaks/gitleaks/releases/download/v${GITLEAKS_VERSION}/$f"
echo "$GITLEAKS_SHA256  $RUNNER_TEMP/$f" | sha256sum -c -
tar -xzf "$RUNNER_TEMP/$f" -C "$RUNNER_TEMP" gitleaks
sudo install "$RUNNER_TEMP/gitleaks" /usr/local/bin/gitleaks
