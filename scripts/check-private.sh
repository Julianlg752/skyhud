#!/bin/sh
# Blocks private flight data, the DJI key, private words, secrets and a
# personal commit email from being committed.
# Install as a pre-commit hook: sh scripts/check-private.sh --install
set -u

if [ "${1:-}" = "--install" ]; then
	hook="$(git rev-parse --git-dir)/hooks/pre-commit"
	printf '#!/bin/sh\nexec sh "$(git rev-parse --show-toplevel)/scripts/check-private.sh"\n' > "$hook"
	chmod +x "$hook"
	echo "installed $hook"
	exit 0
fi

MAX_BYTES=5242880
blocked=0
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

# Commits are public: only a GitHub noreply address may sign them.
email=$(git var GIT_AUTHOR_IDENT 2>/dev/null | sed -n 's/.*<\(.*\)>.*/\1/p')
case $email in
*@users.noreply.github.com) ;;
*) echo "blocked: commit email $email is not your GitHub noreply address (git config user.email <id>+<user>@users.noreply.github.com)"; blocked=1;;
esac

# Private words (places, names, serials), one per line; # starts a comment.
words_file=${SKYHUD_PRIVATE_WORDS:-$HOME/.config/open-hud/private-words}
words="$tmp/words"
: > "$words"
[ -s "$words_file" ] && tr -d '\r' < "$words_file" | grep -v -e '^[[:space:]]*#' -e '^[[:space:]]*$' > "$words"

# Raw names (no quoting of non-ASCII), including renames and type changes.
files=$(git -c core.quotePath=false diff --cached --name-only -z --diff-filter=ACMRT | tr '\000' '\n')
IFS='
'
for f in $files; do
	lower=$(printf '%s' "$f" | tr '[:upper:]' '[:lower:]')
	case $lower in
	*.mp4|*.mov|*.lrv|*.thm|*.bbl|*.bfl|*.dat|*.insv|*djiflightrecord_*.txt|*.decoded.json|*dji_key|*.key)
		echo "blocked: $f looks like private flight data or a key"; blocked=1; continue;;
	esac
	if [ -s "$words" ]; then
		w=$( { printf '%s\n' "$f"; git cat-file blob ":$f" 2>/dev/null; } | grep -a -i -o -F -f "$words" | head -n 1)
		if [ -n "$w" ]; then
			echo "blocked: $f contains the private word \"$w\""; blocked=1
		fi
	fi
	size=$(git cat-file -s ":$f" 2>/dev/null || echo 0)
	if [ "$size" -gt "$MAX_BYTES" ]; then
		echo "blocked: $f is larger than 5 MB"; blocked=1
	fi
done
unset IFS

key_file=${SKYHUD_KEY_FILE:-$HOME/.config/open-hud/dji_key}
if [ -s "$key_file" ]; then
	pattern="$tmp/key"
	tr -d ' \t\r\n' < "$key_file" > "$pattern"
	if [ -s "$pattern" ] && git diff --cached --text -U0 | grep -a -F -i -q -f "$pattern"; then
		echo "blocked: a DJI key was found in staged changes"; blocked=1
	fi
fi

# Secrets of any kind (tokens, private keys...), when gitleaks is installed.
if command -v gitleaks >/dev/null 2>&1; then
	if ! gitleaks git --pre-commit --staged --no-banner --redact --log-level error . >/dev/null 2>&1; then
		echo "blocked: gitleaks found a secret in staged changes (see: gitleaks git --pre-commit --staged --redact -v)"; blocked=1
	fi
else
	echo "note: gitleaks is not installed, so secrets other than the DJI key are not checked"
fi

[ $blocked -eq 0 ] || echo "commit stopped by scripts/check-private.sh"
exit $blocked
