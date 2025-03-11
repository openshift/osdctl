#!/bin/bash
set -e

echo "Verifying documentation..."

make generate-docs

if git diff --name-only | grep -q "\.md$"; then
   echo "ERROR: Documentation is out of date"
   echo "Run 'make generate-docs' locally and commit the changes"
   git diff -- '*.md'
   exit 1
fi

echo "Documentation is up to date"
exit 0