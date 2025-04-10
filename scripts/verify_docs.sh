#!/bin/bash
set -e

echo "Verifying documentation..."

make generate-docs

# Check for changes only in the docs folder for .md files
if git diff --name-only | grep -q "^docs/.*\.md$"; then
   echo "ERROR: Documentation in the 'docs' folder is out of date"
   echo "Run 'make generate-docs' locally and commit the changes"
   git diff -- 'docs/*.md'
   exit 1
fi

echo "Documentation in the 'docs' folder is up to date"
exit 0