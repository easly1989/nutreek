"""Icone dell'app (PWA, iPhone, favicon), una serie per colore del tema (#59).

La foglia del logo (#i-leaf in base.html), bianca sul colore d'accento.
Si rigenerano solo se cambiano il logo o i colori; servono Python 3 e Pillow:

    python scripts/icons.py

Scrive in internal/web/static/icons/ (embedded nel binario):
    icon-<colore>-192.png, icon-<colore>-512.png   angoli arrotondati ("any")
    maskable-<colore>-512.png                      a tutto campo, foglia nella zona sicura
    apple-<colore>-180.png                         a tutto campo: gli angoli li fa iOS
    favicon-<colore>-32.png
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
SS = 4  # supersampling: si disegna a 4x e si riduce, per bordi morbidi


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


# Path della foglia nel riquadro 24x24: M5 19c0-8 6-14 14-14 0 8-6 14-14 14z M5 19l7-7
LEAF = [
    bezier((5, 19), (5, 11), (11, 5), (19, 5)) + bezier((19, 5), (19, 13), (13, 19), (5, 19))[1:],
    [(5, 19), (12, 12)],
]


def stroke(draw, pts, width, color):
    """Tratto con estremi e giunzioni arrotondati, come stroke-linecap/linejoin round."""
    r = width / 2
    draw.line(pts, fill=color, width=round(width))
    for x, y in pts:
        draw.ellipse((x - r, y - r, x + r, y + r), fill=color)


def icon(size, color, glyph, radius):
    """glyph: lato della foglia (riquadro 24) in frazione dell'icona."""
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
        # Proporzioni del mockup: foglia 84/148 dell'icona, angoli 34/148.
        icon(192, color, 0.57, 0.23).save(OUT / f"icon-{name}-192.png", optimize=True)
        icon(512, color, 0.57, 0.23).save(OUT / f"icon-{name}-512.png", optimize=True)
        # Maskable: Android ritaglia fino a un cerchio di raggio 40%; la foglia
        # (58% del suo riquadro) resta ben dentro.
        icon(512, color, 0.5, 0).save(OUT / f"maskable-{name}-512.png", optimize=True)
        icon(180, color, 0.57, 0).save(OUT / f"apple-{name}-180.png", optimize=True)
        icon(32, color, 0.7, 0.22).save(OUT / f"favicon-{name}-32.png", optimize=True)


if __name__ == "__main__":
    main()
