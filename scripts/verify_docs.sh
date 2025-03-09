#!/bin/bash

set -e

echo "Verifying documentation..."

cp -f osdctl_commands.md /tmp/original_docs.md 2>/dev/null || touch /tmp/original_docs.md

make generate-docs

if ! diff -q /tmp/original_docs.md osdctl_commands.md &>/dev/null; then
    echo "ERROR: Documentation is out of date"
    diff -u /tmp/original_docs.md osdctl_commands.md || true
    exit 1
fi

echo "Documentation is up to date"
exit 0