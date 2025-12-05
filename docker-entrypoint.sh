#!/bin/sh
set -e

# Get PUID and PGID from environment variables (default to 1000)
PUID=${PUID:-1000}
PGID=${PGID:-1000}

echo "Starting with UID: $PUID, GID: $PGID"

# Update user and group IDs if they differ from default
if [ "$PUID" != "1000" ] || [ "$PGID" != "1000" ]; then
    echo "Updating appuser UID to $PUID and GID to $PGID"

    # Remove existing user/group
    userdel appuser 2>/dev/null || true
    groupdel appuser 2>/dev/null || true

    # Create new user/group with specified IDs
    groupadd -g "$PGID" appuser
    useradd -u "$PUID" -g appuser -m appuser
fi

# Create and fix permissions for mounted data directories
mkdir -p /app/data /app/subscribes /app/rule_templates

echo "Fixing permissions for mounted volumes..."
chown -R appuser:appuser /app/data /app/subscribes /app/rule_templates

# Use gosu to drop privileges and run the application as appuser
echo "Starting application as appuser..."
exec gosu appuser "$@"
