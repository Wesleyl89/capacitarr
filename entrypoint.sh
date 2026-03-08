#!/bin/sh
set -e

# PUID/PGID pattern — allows container to run as any UID/GID
# Defaults to 1000:1000 if not specified
PUID=${PUID:-1000}
PGID=${PGID:-1000}

# Create group if it doesn't exist
if ! getent group capacitarr >/dev/null 2>&1; then
    addgroup -g "$PGID" capacitarr
fi

# Create user if it doesn't exist
if ! getent passwd capacitarr >/dev/null 2>&1; then
    adduser -D -u "$PUID" -G capacitarr -h /app -s /sbin/nologin capacitarr
fi

# Ensure config directory exists and is owned by the target user
mkdir -p /config
chown "$PUID:$PGID" /config

# Normalize BASE_URL for healthcheck — ensure it starts and ends with /
HEALTH_BASE="${BASE_URL:-/}"
case "$HEALTH_BASE" in
    /*) ;; # already starts with /
    *)  HEALTH_BASE="/$HEALTH_BASE" ;;
esac
case "$HEALTH_BASE" in
    */) ;; # already ends with /
    *)  HEALTH_BASE="$HEALTH_BASE/" ;;
esac
export CAPACITARR_HEALTH_URL="http://localhost:${PORT:-2187}${HEALTH_BASE}api/v1/health"

echo "Starting Capacitarr as UID=$PUID GID=$PGID"

# Drop privileges and exec the application
exec su-exec "$PUID:$PGID" /app/capacitarr "$@"
