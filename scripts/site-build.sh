#!/bin/sh
# scripts/site-build.sh — Build the documentation site for deployment.
#
# Used by:
#   - Cloudflare Pages (build command: scripts/site-build.sh)
#   - GitHub Actions pages.yml workflow
#
# Output: site/.output/public/ (static HTML/CSS/JS)
#
# NOTE: Uses /bin/sh (not bash) for compatibility with Cloudflare's build environment.

set -eux

echo "=== Site Build ==="

# Ensure pnpm is available. Install via npm if not present.
# Cloudflare Pages uses asdf shims that exist but fail with exit code 126,
# so we test with an actual execution and suppress errors.
PNPM_VER=$(pnpm --version 2>/dev/null) || true
if [ -z "$PNPM_VER" ]; then
  echo "pnpm not available, installing via npm..."
  npm install -g pnpm
  PNPM_VER=$(pnpm --version)
fi
echo "pnpm version: $PNPM_VER"

# Install dependencies
cd site
pnpm install --frozen-lockfile

# Sync documentation from docs/ and root-level project files into content/
node scripts/sync-docs.mjs

# Copy favicon from frontend
cp ../frontend/public/favicon.ico public/favicon.ico 2>/dev/null || true

# Generate static site
pnpm generate

echo "=== Site build complete → site/.output/public/ ==="
