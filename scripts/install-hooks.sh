#!/bin/bash

set -e

HOOK_DIR=".git/hooks"
SCRIPTS_DIR="scripts/"

mkdir -p $HOOK_DIR
mkdir -p $SCRIPTS_DIR

cat > "$SCRIPTS_DIR/pre-push" << 'EOL'
#!/bin/bash

set -e

echo "Running pre-push hook to check documentation..."

if [[ -f "osdctl_commands.md" ]]; then
    PRE_MD5=$(md5sum "osdctl_commands.md" | cut -d ' ' -f 1)
else
    PRE_MD5=""
fi

echo "Generating documentation..."
if make generate-docs; then
    echo "Documentation generated successfully."
else
    echo "Failed to generate documentation. Fix errors before pushing."
    exit 1
fi

if [[ -f "osdctl_commands.md" ]]; then
    POST_MD5=$(md5sum "osdctl_commands.md" | cut -d ' ' -f 1)
    
    if [[ "$PRE_MD5" != "$POST_MD5" ]]; then
        echo "ERROR: Documentation is out of date. The pre-push hook updated it."
        echo "Please add and commit the updated documentation before pushing:"
        echo "    git add osdctl_commands.md"
        echo "    git commit -m \"Update documentation\""
        exit 1
    else
        echo "Documentation is up to date."
    fi
else
    echo "ERROR: Documentation file osdctl_commands.md was not generated."
    exit 1
fi

echo "Pre-push hook completed successfully."
exit 0
EOL

chmod +x "$SCRIPTS_DIR/pre-push"

ln -sf "../../$SCRIPTS_DIR/pre-push" "$HOOK_DIR/pre-push"

echo "Git hooks installed successfully!"
echo "The pre-push hook will now check documentation before each push."
