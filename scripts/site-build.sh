#!/bin/sh
# scripts/site-build.sh — Build the documentation site for deployment.
#
# Used by:
#   - Cloudflare Pages (build command: scripts/site-build.sh)
#     Root directory: / (repo root)
#     Build output: site/dist
#     Required env vars: PNPM_VERSION (set in Cloudflare Pages settings)
#
# Output: site/dist/ (static HTML/CSS/JS)

set -eux

echo "=== Site Build ==="

# Install site dependencies
cd site
pnpm install --frozen-lockfile

# Sync documentation from docs/ and root-level project files into content/
node scripts/sync-docs.mjs

# Copy favicon from frontend
cp ../frontend/public/favicon.ico public/favicon.ico

# Generate static site
pnpm generate

echo "=== Site build complete ==="
