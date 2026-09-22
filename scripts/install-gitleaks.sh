#!/bin/sh
# Runs with Git Bash on Windows or sh on Linux (x86_64).
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"
gitleaks_version=8.30.1

case "$(uname -s):$(uname -m)" in
  Linux:x86_64)
    release_asset=linux_x64.tar.gz
    expected_checksum=551f6fc83ea457d62a0d98237cbad105af8d557003051f41f3e7ca7b3f2470eb
    binary_name=gitleaks
    ;;
  MINGW*:x86_64|MSYS*:x86_64)
    release_asset=windows_x64.zip
    expected_checksum=d29144deff3a68aa93ced33dddf84b7fdc26070add4aa0f4513094c8332afc4e
    binary_name=gitleaks.exe
    ;;
  *)
    echo 'Automatic installation supports Linux x86_64 and Windows x64 (Git Bash).' >&2
    echo "Install Gitleaks $gitleaks_version from https://github.com/gitleaks/gitleaks/releases into PATH." >&2
    exit 1
    ;;
esac

mkdir -p .tools
download_directory=$(mktemp -d .tools/gitleaks-download.XXXXXX)
downloaded_archive="$download_directory/gitleaks.$release_asset"
release_url="https://github.com/gitleaks/gitleaks/releases/download/v$gitleaks_version"
trap 'rm -f "$downloaded_archive"; rmdir "$download_directory"' EXIT

curl --fail --silent --show-error --location --retry 3 \
  --connect-timeout 15 --max-time 180 \
  "$release_url/gitleaks_${gitleaks_version}_$release_asset" \
  --output "$downloaded_archive"
printf '%s  %s\n' "$expected_checksum" "$downloaded_archive" | sha256sum --check --status

case "$release_asset" in
  *.zip) unzip -p "$downloaded_archive" "$binary_name" > ".tools/$binary_name" ;;
  *.tar.gz) tar -xzf "$downloaded_archive" -C .tools "$binary_name" ;;
esac

chmod +x ".tools/$binary_name"
"./.tools/$binary_name" version
