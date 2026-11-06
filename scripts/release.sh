#!/bin/sh
# Builds the release zips: sh scripts/release.sh 0.1.0
# Writes dist/SkyHUD-<version>-macOS-arm64.zip, -macOS-x64.zip and
# -Windows-x64.zip. Publishing them is done by hand (docs/release-checklist.md).
set -eu
version=${1:?usage: sh scripts/release.sh <version>}
root=$(cd "$(dirname "$0")/.." && pwd)
dist="$root/dist"
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT
mkdir -p "$dist"

# Copies of the four built-in designs under their own names, to edit or
# learn from (the built-in ones are inside the program).
designs="$stage/designs"
for d in goggles:Goggles race:Race fighter:"Fighter jet" clean:Clean; do
	(cd "$root" && go run ./cmd/skyhud design save --from "${d%%:*}" --name "${d#*:} copy" --dir "$designs" >/dev/null)
done
cp "$root/packaging/designs-README.txt" "$designs/README.txt"

build() { # goos goarch label
	name="SkyHUD-$version-$3"
	dir="$stage/$name"
	mkdir -p "$dir"
	bin=skyhud
	[ "$1" = windows ] && bin=skyhud.exe
	(cd "$root" && CGO_ENABLED=0 GOOS=$1 GOARCH=$2 go build -trimpath \
		-ldflags "-s -w -X main.version=$version" -o "$dir/$bin" ./cmd/skyhud)
	sed "s/{{VERSION}}/$version/" "$root/packaging/README.txt" >"$dir/README.txt"
	cp "$root/LICENSE" "$dir/LICENSE"
	cp -R "$designs" "$dir/designs"
	if [ "$1" = windows ]; then
		for f in "Install SkyHUD.bat" install.ps1; do
			sed 's/$/\r/' "$root/packaging/windows/$f" >"$dir/$f"
		done
		for f in README.txt LICENSE designs/README.txt; do
			sed 's/$/\r/' "$dir/$f" >"$dir/$f.crlf" && mv "$dir/$f.crlf" "$dir/$f"
		done
	else
		cp "$root/packaging/macos/Install SkyHUD.command" "$dir/"
		chmod +x "$dir/Install SkyHUD.command" "$dir/$bin"
	fi
	rm -f "$dist/$name.zip"
	(cd "$stage" && zip -qr "$dist/$name.zip" "$name")
	echo "$dist/$name.zip"
}

build darwin arm64 macOS-arm64
build darwin amd64 macOS-x64
build windows amd64 Windows-x64
