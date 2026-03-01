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

echo "Starting Capacitarr as UID=$PUID GID=$PGID"

# Drop privileges and exec the application
exec su-exec "$PUID:$PGID" /app/capacitarr "$@"
