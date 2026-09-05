#!/bin/sh
set -e

# Filed albums land in the user's own library, so they have to belong to the
# user and not to root. PUID/PGID is what every NAS image is expected to read.
# Only /config is taken over: /input and /output are the user's media and are
# left exactly as they are.
uid="${PUID:-0}"
gid="${PGID:-0}"

if [ "$uid" != "0" ] || [ "$gid" != "0" ]; then
	chown -R "$uid:$gid" /config 2>/dev/null || true
	exec su-exec "$uid:$gid" gordi "$@"
fi

exec gordi "$@"
