#!/bin/sh
# cyber-tui installer
#
#   curl -fsSL https://raw.githubusercontent.com/ArmadilloBrillo/cyber-tui/dev/install.sh | sh
#
# Environment overrides:
#   CYBER_TUI_VERSION      tag to install (default: latest release)
#   CYBER_TUI_INSTALL_DIR  install directory (default: /usr/local/bin, else ~/.local/bin)
#   CYBER_TUI_REPO         owner/repo to install from (default: this repo)

set -eu

REPO="${CYBER_TUI_REPO:-ArmadilloBrillo/cyber-tui}"
BINARY="cyber-tui"

# Everything lives in main() so a truncated download can never execute a
# half-read script.
main() {
	version="${CYBER_TUI_VERSION:-}"
	tmpdir=""
	trap 'if [ -n "$tmpdir" ]; then rm -rf "$tmpdir"; fi' EXIT INT TERM

	need_cmd uname
	need_cmd mkdir
	need_cmd chmod

	target=$(detect_target)
	install_dir=$(resolve_install_dir)

	if [ "$target" = "unsupported" ]; then
		say "no prebuilt binary is published for $(uname -s)/$(uname -m)"
		install_from_source "$install_dir" "$version"
		return
	fi

	if [ -z "$version" ]; then
		version=$(latest_version) || version=""
		if [ -z "$version" ]; then
			say "no published release found for $REPO"
			install_from_source "$install_dir" ""
			return
		fi
	fi
	asset="$BINARY-$target"
	case "$target" in
	windows-*) asset="$asset.exe" ;;
	esac

	base="https://github.com/$REPO/releases/download/$version"
	tmpdir=$(mktemp -d)

	say "downloading $BINARY $version ($target)"
	# A missing asset is not fatal: the tag exists in git, so it can be built.
	if ! fetch "$base/$asset" "$tmpdir/$asset"; then
		say "release $version has no '$asset' asset"
		rm -rf "$tmpdir"
		tmpdir=""
		install_from_source "$install_dir" "$version"
		return
	fi

	verify_checksum "$base/SHA256SUMS" "$tmpdir" "$asset"

	chmod +x "$tmpdir/$asset"
	install_binary "$tmpdir/$asset" "$install_dir"

	say "installed $install_dir/$BINARY"
	"$install_dir/$BINARY" --version 2>/dev/null || true
	warn_path "$install_dir"
}

say() { printf '  %s\n' "$*" >&2; }
err() {
	printf 'error: %s\n' "$*" >&2
	exit 1
}

need_cmd() {
	command -v "$1" >/dev/null 2>&1 || err "required command not found: $1"
}

# Print the release-asset suffix for this machine, or "unsupported" when the
# release workflow publishes no binary for it. Anything "unsupported" — and any
# target whose asset is missing from the chosen release — falls back to a source
# build, so this list only needs to match release.yml.
detect_target() {
	os=$(uname -s)
	arch=$(uname -m)

	case "$os" in
	Linux) os="linux" ;;
	Darwin) os="darwin" ;;
	MINGW* | MSYS* | CYGWIN*) os="windows" ;;
	*)
		echo "unsupported"
		return
		;;
	esac

	case "$arch" in
	x86_64 | amd64) arch="amd64" ;;
	aarch64 | arm64) arch="arm64" ;;
	*)
		echo "unsupported"
		return
		;;
	esac

	case "$os-$arch" in
	linux-amd64 | linux-arm64 | darwin-amd64 | darwin-arm64 | windows-amd64) echo "$os-$arch" ;;
	*) echo "unsupported" ;;
	esac
}

resolve_install_dir() {
	if [ -n "${CYBER_TUI_INSTALL_DIR:-}" ]; then
		echo "$CYBER_TUI_INSTALL_DIR"
	elif [ -w /usr/local/bin ] 2>/dev/null; then
		echo "/usr/local/bin"
	else
		echo "$HOME/.local/bin"
	fi
}

# fetch URL DEST — returns non-zero on HTTP errors; callers report the reason,
# so the downloader's own stderr is suppressed.
fetch() {
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL "$1" -o "$2" 2>/dev/null
	elif command -v wget >/dev/null 2>&1; then
		wget -q "$1" -O "$2" 2>/dev/null
	else
		err "need curl or wget to download files"
	fi
}

# Print the latest release tag, or return non-zero when the repo has none —
# callers fall back to a source build rather than failing outright.
latest_version() {
	tag=$(fetch_stdout "https://api.github.com/repos/$REPO/releases/latest" 2>/dev/null |
		sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' |
		head -n 1) || true
	[ -n "$tag" ] || return 1
	echo "$tag"
}

fetch_stdout() {
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL "$1"
	elif command -v wget >/dev/null 2>&1; then
		wget -qO- "$1"
	else
		err "need curl or wget to download files"
	fi
}

# Verify the downloaded asset against the release SHA256SUMS. Skipped with a
# warning when the machine has no sha256 tool, rather than blocking the install.
verify_checksum() {
	sums_url="$1"
	dir="$2"
	file="$3"

	if command -v sha256sum >/dev/null 2>&1; then
		sha_cmd="sha256sum"
	elif command -v shasum >/dev/null 2>&1; then
		sha_cmd="shasum -a 256"
	else
		say "warning: no sha256sum/shasum found — skipping checksum verification"
		return
	fi

	if ! fetch "$sums_url" "$dir/SHA256SUMS"; then
		say "warning: SHA256SUMS not published for this release — skipping verification"
		return
	fi

	expected=$(awk -v f="$file" '$2 == f || $2 == "*" f { print $1 }' "$dir/SHA256SUMS" | head -n 1)
	if [ -z "$expected" ]; then
		say "warning: $file missing from SHA256SUMS — skipping verification"
		return
	fi

	actual=$($sha_cmd "$dir/$file" | cut -d' ' -f1)
	[ "$expected" = "$actual" ] || err "checksum mismatch for $file (expected $expected, got $actual)"
	say "checksum ok"
}

install_binary() {
	src="$1"
	dir="$2"

	mkdir -p "$dir" 2>/dev/null || true
	if [ -w "$dir" ]; then
		mv -f "$src" "$dir/$BINARY"
	elif command -v sudo >/dev/null 2>&1; then
		say "$dir is not writable — installing with sudo"
		sudo mkdir -p "$dir"
		sudo mv -f "$src" "$dir/$BINARY"
		sudo chmod +x "$dir/$BINARY"
	else
		err "cannot write to $dir — set CYBER_TUI_INSTALL_DIR to a writable directory"
	fi
}

# No prebuilt binary for this platform: clone and build with the local Go
# toolchain. The module path is not a fetchable import path, so `go install
# <path>@version` cannot be used here — the source has to be cloned first.
install_from_source() {
	dir="$1"
	ref="${2:-}"
	manual="git clone https://github.com/$REPO.git && cd ${REPO##*/} && make build"

	command -v go >/dev/null 2>&1 ||
		err "cannot install a prebuilt binary, and Go is not installed.
       Install Go 1.25+ (https://go.dev/dl) and re-run, or build manually:
       $manual"
	command -v git >/dev/null 2>&1 || err "git is required to build from source"

	say "building from source with Go${ref:+ ($ref)}"
	tmpdir=$(mktemp -d)

	git clone --depth 1 ${ref:+--branch "$ref"} \
		"https://github.com/$REPO.git" "$tmpdir/src" >/dev/null 2>&1 ||
		err "clone failed — build manually: $manual"

	# Inject the same version metadata the Makefile and release workflow use, so
	# a source-built binary still reports something useful to `--version`.
	# Read the module path from go.mod so a rename cannot silently break this.
	mod=$(sed -n 's/^module[[:space:]][[:space:]]*//p' "$tmpdir/src/go.mod" | head -n 1)
	mod="${mod:-github.com/ragnar/cyber-tui}/internal/version"
	commit=$(cd "$tmpdir/src" && git rev-parse --short HEAD 2>/dev/null || echo none)
	built=$(date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || echo unknown)
	ldflags="-X $mod.Version=${ref:-source} -X $mod.Commit=$commit -X $mod.Date=$built"

	(cd "$tmpdir/src" && go build -ldflags "$ldflags" -o "$tmpdir/$BINARY" ./cmd/cyber-tui) ||
		err "build failed — build manually: $manual"

	install_binary "$tmpdir/$BINARY" "$dir"
	say "installed $dir/$BINARY"
	"$dir/$BINARY" --version 2>/dev/null || true
	warn_path "$dir"
}

warn_path() {
	case ":$PATH:" in
	*":$1:"*) ;;
	*) say "note: $1 is not on your PATH — add it with: export PATH=\"$1:\$PATH\"" ;;
	esac
}

main "$@"
