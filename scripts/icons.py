"""The app's icons (PWA, iPhone, favicon), one set per theme color (#59).

The logo's leaf (#i-leaf in base.html), white on the accent color.
Regenerate them only when the logo or the colors change; they need Python 3 and Pillow:

    python scripts/icons.py

Writes to internal/web/static/icons/ (embedded in the binary):
    icon-<color>-192.png, icon-<color>-512.png     rounded corners ("any")
    maskable-<color>-512.png                       full bleed, leaf in the safe zone
    apple-<color>-180.png                          full bleed: iOS makes the corners
    favicon-<color>-32.png
"""
from pathlib import Path

from PIL import Image, ImageDraw

ACCENTS = {
    "bosco": "#2F7A55",
    "mare": "#1D6FA5",
    "tramonto": "#B4511A",
    "grafite": "#3A4150",
}
OUT = Path(__file__).resolve().parent.parent / "internal" / "web" / "static" / "icons"
SS = 4  # supersampling: draw at 4x and scale down, for smooth edges


def bezier(p0, p1, p2, p3, n=48):
    pts = []
    for i in range(n + 1):
        t = i / n
        u = 1 - t
        pts.append((
            u**3 * p0[0] + 3 * u * u * t * p1[0] + 3 * u * t * t * p2[0] + t**3 * p3[0],
            u**3 * p0[1] + 3 * u * u * t * p1[1] + 3 * u * t * t * p2[1] + t**3 * p3[1],
        ))
    return pts


# Path of the leaf in the 24x24 box: M5 19c0-8 6-14 14-14 0 8-6 14-14 14z M5 19l7-7
LEAF = [
    bezier((5, 19), (5, 11), (11, 5), (19, 5)) + bezier((19, 5), (19, 13), (13, 19), (5, 19))[1:],
    [(5, 19), (12, 12)],
]


def stroke(draw, pts, width, color):
    """A stroke with rounded ends and joins, like stroke-linecap/linejoin round."""
    r = width / 2
    draw.line(pts, fill=color, width=round(width))
    for x, y in pts:
        draw.ellipse((x - r, y - r, x + r, y + r), fill=color)


def icon(size, color, glyph, radius):
    """glyph: the leaf's side (24 box) as a fraction of the icon."""
    s = size * SS
    img = Image.new("RGBA", (s, s), (0, 0, 0, 0))
    d = ImageDraw.Draw(img)
    d.rounded_rectangle((0, 0, s - 1, s - 1), radius=round(radius * s), fill=color)
    box = glyph * s
    k = box / 24
    off = (s - box) / 2
    for path in LEAF:
        stroke(d, [(off + x * k, off + y * k) for x, y in path], 2.1 * k, "#FFFFFF")
    return img.resize((size, size), Image.LANCZOS)


def main():
    OUT.mkdir(parents=True, exist_ok=True)
    for name, color in ACCENTS.items():
        # The mockup's proportions: leaf 84/148 of the icon, corners 34/148.
        icon(192, color, 0.57, 0.23).save(OUT / f"icon-{name}-192.png", optimize=True)
        icon(512, color, 0.57, 0.23).save(OUT / f"icon-{name}-512.png", optimize=True)
        # Maskable: Android crops down to a circle of radius 40%; the leaf
        # (58% of its box) stays well inside.
        icon(512, color, 0.5, 0).save(OUT / f"maskable-{name}-512.png", optimize=True)
        icon(180, color, 0.57, 0).save(OUT / f"apple-{name}-180.png", optimize=True)
        icon(32, color, 0.7, 0.22).save(OUT / f"favicon-{name}-32.png", optimize=True)


if __name__ == "__main__":
    main()
