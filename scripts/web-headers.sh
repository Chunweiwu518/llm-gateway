#!/usr/bin/env bash
# Prints cache and compression headers and transfer sizes of the served console (default http://127.0.0.1:5173).
BASE=${1:-http://127.0.0.1:5173}
JS=$(curl -s "$BASE/login" | grep -o '/assets/[^"]*\.js' | head -1)
echo "== index.html"
curl -s -D - -o /dev/null "$BASE/login" | grep -i -E "^HTTP|cache-control|content-type"
echo "== main script $JS (Accept-Encoding: gzip)"
curl -s -D - -o /dev/null -H "Accept-Encoding: gzip" "$BASE$JS" | grep -i -E "^HTTP|cache-control|content-encoding"
echo "bytes uncompressed: $(curl -s -H 'Accept-Encoding: identity' "$BASE$JS" | wc -c)"
echo "bytes with gzip:    $(curl -s -H 'Accept-Encoding: gzip' "$BASE$JS" | wc -c)"
echo "== scripts and styles referenced by /login"
curl -s "$BASE/login" | grep -o '/assets/[^"]*'
echo "== robots.txt"
curl -s -D - "$BASE/robots.txt" | grep -i -E "^HTTP|content-type|^User-agent|^<!doctype" | head -4
echo "== Accept-Encoding: br"
curl -s -D - -o /dev/null -H "Accept-Encoding: br" "$BASE$JS" | grep -i content-encoding || echo "no content-encoding returned"
