#!/bin/bash

set -e

HOOK_DIR=".git/hooks"
SCRIPTS_DIR="scripts/"

mkdir -p $HOOK_DIR
mkdir -p $SCRIPTS_DIR

cat > "$SCRIPTS_DIR/pre-commit" << 'EOL'
#!/bin/bash

set -e

echo "Running pre-commit hook to check documentation..."

# Check if there are any changes to Go code files that might affect documentation
if git diff --staged --name-only | grep -E '\.go$'; then
    echo "Generating documentation..."
    if make generate-docs; then
        echo "Documentation generated successfully."
    else
        echo "Failed to generate documentation. Fix errors before committing."
        exit 1
    fi

    # Check if there are any changes to the documentation file
    if git diff --name-only --cached | grep -q "osdctl_commands.md"; then
        echo "Documentation file is already staged for commit."
    elif git diff --name-only | grep -q "osdctl_commands.md"; then
        echo "ERROR: Documentation is out of date."
        echo "Please stage the updated documentation before committing:"
        echo "    git add osdctl_commands.md"
        exit 1
    else
        echo "Documentation is up to date."
    fi
fi

echo "Pre-commit hook completed successfully."
exit 0
EOL

chmod +x "$SCRIPTS_DIR/pre-commit"

ln -sf "../../$SCRIPTS_DIR/pre-commit" "$HOOK_DIR/pre-commit"

echo "Git hooks installed successfully!"
echo "The pre-commit hook will check documentation when Go files are modified."