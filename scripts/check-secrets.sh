#!/bin/sh
set -eu

# Every path below is relative to the repository root, so establish it before reading any argument.
cd "$(git rev-parse --show-toplevel)"

scan_mode=${1:-staged}
case "$scan_mode" in
  staged|history) ;;
  *) echo 'Usage: sh scripts/check-secrets.sh [staged|history]' >&2; exit 2 ;;
esac

# .gitignore does not protect files already added to the index, including git add -f.
ignored_and_tracked=$(git ls-files --cached --ignored --exclude-standard)
if [ -n "$ignored_and_tracked" ]; then
  echo 'Ignored files must not be tracked. Remove these paths from the Git index:' >&2
  printf '%s\n' "$ignored_and_tracked" >&2
  exit 1
fi

if [ -x .tools/gitleaks ]; then
  gitleaks_command=./.tools/gitleaks
elif [ -x .tools/gitleaks.exe ]; then
  gitleaks_command=./.tools/gitleaks.exe
elif command -v gitleaks >/dev/null 2>&1; then
  gitleaks_command=gitleaks
else
  echo 'Gitleaks is required. Run: sh scripts/install-gitleaks.sh' >&2
  exit 1
fi

case "$scan_mode" in
  staged)
    exec "$gitleaks_command" git --staged --redact --no-banner --config .gitleaks.toml .
    ;;
  history)
    exec "$gitleaks_command" git --log-opts=--all --redact --no-banner --config .gitleaks.toml .
    ;;
esac
