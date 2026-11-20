#!/usr/bin/env bash
# Stitches the clips recorded by `go run ./cmd/demo` into one MP4 (for viewing)
# and one GIF (for the README). Requires ffmpeg.
set -euo pipefail
cd "$(dirname "$0")/.."
raw=docs/demo/raw
out=docs/demo
clip() { ffmpeg -y -loglevel error -f concat -safe 0 -i "$raw/$1/frames.txt" -vsync cfr -r 25 -pix_fmt yuv420p -c:v libx264 -crf 18 "$raw/$1.mp4"; echo "$raw/$1.mp4"; }
desktop=$(clip 1-desktop)
mobile=$(clip 2-mobile)
report=""; [[ -f "$raw/3-report/frames.txt" ]] && report=$(clip 3-report)

inputs=(-i "$desktop" -i "$mobile")
filter="[0:v]fps=25,setsar=1[a];[1:v]fps=25,pad=1280:800:(ow-iw)/2:0:color=0xe9ecf1,setsar=1[b];"
concat="[a][b]"; n=2
if [[ -n "$report" ]]; then
  inputs+=(-i "$report")
  filter+="[2:v]fps=25,setsar=1[c];"; concat+="[c]"; n=3
fi
filter+="${concat}concat=n=${n}:v=1:a=0[v]"

ffmpeg -y -loglevel error "${inputs[@]}" -filter_complex "$filter" -map "[v]" \
  -c:v libx264 -preset slow -crf 24 -pix_fmt yuv420p -movflags +faststart "$out/mini-checkout-demo.mp4"
ffmpeg -y -loglevel error -i "$out/mini-checkout-demo.mp4" \
  -vf "fps=6,scale=800:-1:flags=lanczos,split[s0][s1];[s0]palettegen=max_colors=80[p];[s1][p]paletteuse=dither=bayer:bayer_scale=5" \
  "$out/mini-checkout-demo.gif"
ls -lh "$out"/mini-checkout-demo.*
ffprobe -v error -show_entries format=duration -of csv=p=0 "$out/mini-checkout-demo.mp4"
