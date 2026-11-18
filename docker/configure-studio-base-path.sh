#!/bin/sh
set -eu

base=${1:-/}
config=${2:-/etc/nginx/conf.d/default.conf}
base=${base%/}
test -n "$base" || exit 0
case "$base" in
    /*) ;;
    *) echo 'Studio base path must start with /' >&2; exit 1 ;;
esac
case "$base" in
    *[!a-zA-Z0-9/_-]*|*//*|*/) echo 'Invalid Studio base path' >&2; exit 1 ;;
esac

# Internal rewriting preserves the external URL and the original Host/Origin.
# The existing API, WebSocket, SSE and hashed-asset locations remain authoritative.
temporary=$(mktemp)
trap 'rm -f "$temporary"' EXIT
awk -v base="$base" '
    /# STUDIO_BASE_PATH/ {
        print "    location = " base " { return 308 " base "/; }"
        print "    rewrite ^" base "/(.*)$ /$1 last;"
        next
    }
    { print }
' "$config" > "$temporary"
cat "$temporary" > "$config"
