#!/bin/sh
# Run with Git Bash on Windows or sh on Linux (x86_64).
set -eu

cd "$(git rev-parse --show-toplevel)"
version=8.30.1
case "$(uname -s):$(uname -m)" in
  Linux:x86_64)
    platform=linux_x64.tar.gz
    checksum=551f6fc83ea457d62a0d98237cbad105af8d557003051f41f3e7ca7b3f2470eb
    binary=gitleaks
    ;;
  MINGW*:x86_64|MSYS*:x86_64)
    platform=windows_x64.zip
    checksum=d29144deff3a68aa93ced33dddf84b7fdc26070add4aa0f4513094c8332afc4e
    binary=gitleaks.exe
    ;;
  *)
    echo 'Automatic installation supports Linux x86_64 and Windows x64 (Git Bash).' >&2
    echo "Install Gitleaks $version from https://github.com/gitleaks/gitleaks/releases into PATH." >&2
    exit 1
    ;;
esac

mkdir -p .tools
download_dir=$(mktemp -d .tools/gitleaks-download.XXXXXX)
archive="$download_dir/gitleaks.$platform"
trap 'rm -f "$archive"; rmdir "$download_dir"' EXIT

curl --fail --silent --show-error --location --retry 3 \
  --connect-timeout 15 --max-time 180 \
  "https://github.com/gitleaks/gitleaks/releases/download/v$version/gitleaks_${version}_$platform" \
  --output "$archive"
printf '%s  %s\n' "$checksum" "$archive" | sha256sum --check --status

case "$platform" in
  *.zip) unzip -p "$archive" "$binary" > ".tools/$binary" ;;
  *.tar.gz) tar -xzf "$archive" -C .tools "$binary" ;;
esac
chmod +x ".tools/$binary"
"./.tools/$binary" version
