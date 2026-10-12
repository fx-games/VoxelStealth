#!/bin/sh
# Re-runnable: skips existing files unless --force.
exec "${KOKORO_PY:-$HOME/.kokoro-de/venv312/bin/python}" "$(dirname "$0")/generate.py" "$@"
