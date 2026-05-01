#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
OUTPUT="$SCRIPT_DIR/reManager User Guide.epub"

COVER_ARGS=()
if [[ -f "$SCRIPT_DIR/images/cover.png" ]]; then
  COVER_ARGS=(--epub-cover-image="$SCRIPT_DIR/images/cover.png")
fi

# Generate QR codes for ![qr](URL) references
GUIDE="$SCRIPT_DIR/guide.md"
PROCESSED=$(mktemp /tmp/guide-XXXXX.md)
python3 -c "
import re, hashlib, qrcode, sys
md = open('$GUIDE').read()
def gen_qr(m):
    url = m.group(1)
    name = 'qr-' + hashlib.md5(url.encode()).hexdigest()[:8] + '.png'
    path = '$SCRIPT_DIR/images/' + name
    qr = qrcode.QRCode(border=1, box_size=3)
    qr.add_data(url)
    qr.make(fit=True)
    qr.make_image().save(path)
    return '![](' + 'images/' + name + ')'
md = re.sub(r'!\[qr\]\(([^)]+)\)', gen_qr, md)
open('$PROCESSED', 'w').write(md)
"

pandoc \
  --metadata-file="$SCRIPT_DIR/metadata.yaml" \
  --resource-path="$SCRIPT_DIR" \
  --split-level=1 \
  --epub-title-page=false \
  "${COVER_ARGS[@]}" \
  -o "$OUTPUT" \
  "$PROCESSED"
rm -f "$PROCESSED"

TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT
unzip -q -o "$OUTPUT" -d "$TMPDIR"
for f in "$TMPDIR"/EPUB/text/*.xhtml; do
  [ -f "$f" ] && perl -pi -e '
    s|<figure>|<div class="figure" style="margin-top:1em;text-align:center">|g;
    s|</figure>|</div>|g;
    s|<figcaption[^>]*>.*?</figcaption>||g;
    s|<body([^>]*)>|<body$1 style="font-family:EBGaramond-Regular,Georgia,serif;line-height:1.6">|g;
  ' "$f"
done
perl -pi -e 's|page-break-before: always;|page-break-before: avoid;|g' "$TMPDIR"/EPUB/styles/stylesheet1.css
(cd "$TMPDIR" && zip -q -0 "$OUTPUT" mimetype && zip -q -r "$OUTPUT" . -x mimetype)

echo "Built: $OUTPUT"
