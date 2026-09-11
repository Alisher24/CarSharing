#!/bin/sh
set -eu

cd "$(git rev-parse --show-toplevel)"
mode=${1:-staged}
case "$mode" in
  staged|history) ;;
  *) echo 'Usage: sh scripts/check-secrets.sh [staged|history]' >&2; exit 2 ;;
esac

# .gitignore does not protect files already added to the index, including git add -f.
ignored_tracked=$(git ls-files --cached --ignored --exclude-standard)
if [ -n "$ignored_tracked" ]; then
  echo 'Ignored files must not be tracked. Remove these paths from the Git index:' >&2
  printf '%s\n' "$ignored_tracked" >&2
  exit 1
fi

if [ -x .tools/gitleaks ]; then
  scanner=./.tools/gitleaks
elif [ -x .tools/gitleaks.exe ]; then
  scanner=./.tools/gitleaks.exe
elif command -v gitleaks >/dev/null 2>&1; then
  scanner=gitleaks
else
  echo 'Gitleaks is required. Run: sh scripts/install-gitleaks.sh' >&2
  exit 1
fi

case "$mode" in
  staged)
    exec "$scanner" git --staged --redact --no-banner --config .gitleaks.toml .
    ;;
  history)
    exec "$scanner" git --log-opts=--all --redact --no-banner --config .gitleaks.toml .
    ;;
esac
