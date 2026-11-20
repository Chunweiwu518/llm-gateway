#!/usr/bin/env bash
# Usage: scripts/lighthouse.sh <label> [url] [runs]   (stack up; image "lh" built from scripts/lighthouse.Dockerfile)
# Runs Lighthouse (mobile preset, default throttling) <runs> times and writes a score summary
# per run to docs/lighthouse/<label>-<n>.json. Full reports are not kept.
set -euo pipefail
LABEL=${1:?label}
URL=${2:-http://127.0.0.1:5173/login}
RUNS=${3:-3}
mkdir -p docs/lighthouse
for n in $(seq 1 "$RUNS"); do
  docker run --rm --network host --user "$(id -u):$(id -g)" lh "$URL" --quiet --output=json --output-path=stdout \
    --only-categories=performance,seo,accessibility,best-practices \
    --chrome-flags="--headless --no-sandbox --disable-gpu --disable-dev-shm-usage" \
    | python3 -c '
import json, sys
r = json.load(sys.stdin)
a = r["audits"]
out = {
  "url": r["finalDisplayedUrl"], "lighthouse": r["lighthouseVersion"],
  "scores": {k: round(v["score"] * 100) for k, v in r["categories"].items()},
  "fcp_ms": round(a["first-contentful-paint"]["numericValue"]),
  "lcp_ms": round(a["largest-contentful-paint"]["numericValue"]),
  "tbt_ms": round(a["total-blocking-time"]["numericValue"]),
  "cls": a["cumulative-layout-shift"]["numericValue"],
  "transfer_bytes": a["total-byte-weight"]["numericValue"],
  "failed_audits": sorted(k for k, v in a.items() if v["score"] is not None and v["score"] < 1 and v["scoreDisplayMode"] in ("binary", "numeric"))[:40],
}
print(json.dumps(out, indent=1))' > "docs/lighthouse/$LABEL-$n.json"
  python3 -c "import json;d=json.load(open('docs/lighthouse/$LABEL-$n.json'));print('$LABEL run $n', d['scores'], 'LCP', d['lcp_ms'], 'FCP', d['fcp_ms'], 'TBT', d['tbt_ms'], 'bytes', d['transfer_bytes'])"
done
