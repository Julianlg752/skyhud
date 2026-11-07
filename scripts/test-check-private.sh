#!/bin/sh
# Tests for scripts/check-private.sh. Run: sh scripts/test-check-private.sh
set -u
ROOT=$(cd "$(dirname "$0")/.." && pwd)
CHECK="$ROOT/scripts/check-private.sh"
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
fail=0

# expect <exit code> <description>; runs the check in the temp repo
expect() {
	want=$1; desc=$2
	out=$(cd "$TMP/repo" && SKYHUD_KEY_FILE="$TMP/fake_key" SKYHUD_PRIVATE_WORDS="$TMP/words" sh "$CHECK" 2>&1)
	got=$?
	if [ "$got" -ne "$want" ]; then
		echo "FAIL: $desc (exit $got, want $want): $out"; fail=1
	fi
	LAST_OUT=$out
}

reset_repo() {
	rm -rf "$TMP/repo"; mkdir "$TMP/repo"
	(cd "$TMP/repo" && git init -q && git config user.email 1+t@users.noreply.github.com && git config user.name t)
}

[ -f "$CHECK" ] || { echo "FAIL: $CHECK missing"; exit 1; }
printf 'a1b2c3d4e5f60718293a4b5c6d7e8f9\n' > "$TMP/fake_key"
printf '# private words, one per line\n\nSecret Lake\nSN-0042\n' > "$TMP/words"

reset_repo; echo x > "$TMP/repo/clip.MP4"; (cd "$TMP/repo" && git add -f clip.MP4)
expect 1 "video file blocked"
case $LAST_OUT in *clip.MP4*) ;; *) echo "FAIL: message should name clip.MP4"; fail=1;; esac

reset_repo; echo x > "$TMP/repo/DJIFlightRecord_2025-06-01_[10-30-00].txt"; (cd "$TMP/repo" && git add -f .)
expect 1 "DJI flight record blocked"

reset_repo; dd if=/dev/zero of="$TMP/repo/big.bin" bs=1048576 count=6 2>/dev/null; (cd "$TMP/repo" && git add big.bin)
expect 1 "file over 5 MB blocked"

reset_repo; printf 'config\nkey = a1b2c3d4e5f60718293a4b5c6d7e8f9\n' > "$TMP/repo/settings.txt"; (cd "$TMP/repo" && git add settings.txt) # fake key for the test; gitleaks:allow
expect 1 "DJI key in content blocked"
case $LAST_OUT in *a1b2c3d4e5f60718293a4b5c6d7e8f9*) echo "FAIL: output must not contain the key"; fail=1;; esac

reset_repo; echo x > "$TMP/repo/notes.txt"; (cd "$TMP/repo" && git add notes.txt && git commit -qm base && git mv notes.txt flight.MP4)
expect 1 "renamed into a video file blocked"

reset_repo; echo x > "$TMP/repo/vuelo_año.MP4"; (cd "$TMP/repo" && git add -f .)
expect 1 "non-ASCII video name blocked"

reset_repo; printf 'key = A1B2C3D4E5F60718293A4B5C6D7E8F9\n' > "$TMP/repo/upper.txt"; (cd "$TMP/repo" && git add upper.txt) # gitleaks:allow
expect 1 "DJI key in other letter case blocked"

reset_repo; printf 'bin\000a1b2c3d4e5f60718293a4b5c6d7e8f9\000' > "$TMP/repo/blob.bin"; (cd "$TMP/repo" && git add blob.bin)
expect 1 "DJI key inside a binary file blocked"

reset_repo; (cd "$TMP/repo" && git config user.email someone@gmail.com); echo "SkyHUD" > "$TMP/repo/README.md"; (cd "$TMP/repo" && git add README.md)
expect 1 "personal commit email blocked"
case $LAST_OUT in *noreply*) ;; *) echo "FAIL: message should point at the noreply address"; fail=1;; esac

reset_repo; echo "We flew over the secret lake" > "$TMP/repo/notes.md"; (cd "$TMP/repo" && git add notes.md)
expect 1 "private word in content blocked (any letter case)"
case $LAST_OUT in *notes.md*) ;; *) echo "FAIL: message should name notes.md"; fail=1;; esac

reset_repo; echo x > "$TMP/repo/battery-SN-0042.txt"; (cd "$TMP/repo" && git add .)
expect 1 "private word in a file name blocked"

if command -v gitleaks >/dev/null 2>&1; then
	tok="ghp_$(printf 'aB3dE6gH9jK2mN5pQ8sT1vW4yZ7cF0hJ3kL6' )"
	reset_repo; printf 'token = "%s"\n' "$tok" > "$TMP/repo/config.txt"; (cd "$TMP/repo" && git add config.txt)
	expect 1 "token found by gitleaks blocked"
	case $LAST_OUT in *"$tok"*) echo "FAIL: output must not contain the token"; fail=1;; esac
else
	echo "note: gitleaks not installed, its check was not tested"
fi

reset_repo; mkdir -p "$TMP/repo/site/media"; dd if=/dev/zero of="$TMP/repo/site/media/demo.mp4" bs=1048576 count=12 2>/dev/null; (cd "$TMP/repo" && git add -f site/media/demo.mp4)
expect 0 "approved public video under site/media allowed"

reset_repo; mkdir -p "$TMP/repo/site/media"; dd if=/dev/zero of="$TMP/repo/site/media/huge.mp4" bs=1048576 count=26 2>/dev/null; (cd "$TMP/repo" && git add -f site/media/huge.mp4)
expect 1 "site/media file over 25 MB blocked"

reset_repo; mkdir -p "$TMP/repo/site"; echo x > "$TMP/repo/site/clip.MP4"; (cd "$TMP/repo" && git add -f site/clip.MP4)
expect 1 "video outside site/media still blocked"

reset_repo; mkdir -p "$TMP/repo/site/media"; echo x > "$TMP/repo/site/media/DJIFlightRecord_2025-06-01_[10-30-00].txt"; (cd "$TMP/repo" && git add -f site/media)
expect 1 "flight record under site/media blocked"

reset_repo; mkdir -p "$TMP/repo/site/media"; echo x > "$TMP/repo/site/media/flight.LRV"; (cd "$TMP/repo" && git add -f site/media)
expect 1 "camera file other than mp4 under site/media blocked"

reset_repo; mkdir -p "$TMP/repo/site/media"; echo x > "$TMP/repo/site/media/SN-0042.jpg"; (cd "$TMP/repo" && git add -f site/media)
expect 1 "private word in a site/media file name blocked"

reset_repo; mkdir -p "$TMP/repo/site/media"; echo "over the secret lake" > "$TMP/repo/site/media/caption.vtt"; (cd "$TMP/repo" && git add -f site/media)
expect 1 "private word inside a site/media file blocked"

reset_repo; echo "SkyHUD" > "$TMP/repo/README.md"; (cd "$TMP/repo" && git add README.md)
expect 0 "normal file allowed"

# --range BASE HEAD checks every commit of a pull request (CI), without the local-only lists.
# range_repo makes a base commit; commit <file> <content> adds one commit on top.
range_repo() {
	reset_repo; (cd "$TMP/repo" && echo base > README.md && git add README.md && git commit -qm base && git tag base)
}
commit() {
	mkdir -p "$TMP/repo/$(dirname "$1")"; printf '%s\n' "$2" > "$TMP/repo/$1"; (cd "$TMP/repo" && git add -f "$1" && git commit -qm "add $1")
}
expect_range() {
	want=$1; desc=$2
	out=$(cd "$TMP/repo" && SKYHUD_KEY_FILE=/nonexistent SKYHUD_PRIVATE_WORDS=/nonexistent sh "$CHECK" --range base HEAD 2>&1)
	got=$?
	if [ "$got" -ne "$want" ]; then
		echo "FAIL: $desc (exit $got, want $want): $out"; fail=1
	fi
	LAST_OUT=$out
}

range_repo; commit docs/a.md "hello"; commit internal/b.go "package b"
expect_range 0 "range: normal commits allowed"

range_repo; commit clip.MP4 x; (cd "$TMP/repo" && git rm -q clip.MP4 && git commit -qm "remove it again")
expect_range 1 "range: video added in one commit and removed in the next blocked"
case $LAST_OUT in *clip.MP4*) ;; *) echo "FAIL: range message should name clip.MP4"; fail=1;; esac

range_repo; commit "logs/DJIFlightRecord_2025-06-01_[10-30-00].txt" x
expect_range 1 "range: flight record blocked"

range_repo; dd if=/dev/zero of="$TMP/repo/big.bin" bs=1048576 count=6 2>/dev/null; (cd "$TMP/repo" && git add big.bin && git commit -qm big)
expect_range 1 "range: file over 5 MB blocked"

range_repo; commit site/media/demo.mp4 x
expect_range 0 "range: video under site/media allowed"

range_repo; (cd "$TMP/repo" && git config user.email someone@example.com); commit docs/c.md "hi"
expect_range 0 "range: contributors may use any email"

if command -v gitleaks >/dev/null 2>&1; then
	range_repo; commit config.txt "token = \"$tok\""
	expect_range 1 "range: token found by gitleaks blocked"
	case $LAST_OUT in *"$tok"*) echo "FAIL: range output must not contain the token"; fail=1;; esac
fi

[ $fail -eq 0 ] && echo "all checks passed"
exit $fail
