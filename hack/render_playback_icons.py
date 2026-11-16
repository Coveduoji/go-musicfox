"""Regenerate embedded PNGs after editing SVGs. Requires CairoSVG; not needed to build Go."""
from pathlib import Path

import cairosvg

root = Path(__file__).resolve().parents[1] / "internal/ui/assets/playback"
for svg in sorted(root.glob("*.svg")):
    cairosvg.svg2png(
        bytestring=svg.read_bytes(),
        write_to=str(svg.with_suffix(".png")),
        output_width=96,
        output_height=96,
    )
