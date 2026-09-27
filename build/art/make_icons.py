"""Builds every icon of the app from the logo art (build/art/logo.png).

    python build/art/make_icons.py      (needs Pillow)

The full logo is too detailed below ~128 px, so the icons use its crystal
alone: on a dark badge with a gold ring (app, window, extension, site) or
bare (tray). Everything stays transparent outside the shapes.
"""
from pathlib import Path

from PIL import Image, ImageChops, ImageDraw, ImageFilter

ROOT = Path(__file__).resolve().parents[2]
ART = ROOT / "build" / "art"

# The crystal in the logo, traced by hand (logo pixels, 1254 x 1254 source).
CROP = (380, 180, 880, 700)
POLY = [(252, 20), (447, 224), (434, 350), (418, 436), (382, 482), (250, 468),
        (118, 482), (78, 426), (62, 320), (47, 222)]


def crystal() -> Image.Image:
    src = Image.open(ART / "logo.png").convert("RGBA")
    im = src.crop(CROP)
    s = 4
    m = Image.new("L", (im.width * s, im.height * s), 0)
    ImageDraw.Draw(m).polygon([(x * s, y * s) for x, y in POLY], fill=255)
    m = m.resize(im.size, Image.LANCZOS).filter(ImageFilter.GaussianBlur(0.6))
    im.putalpha(ImageChops.multiply(im.getchannel("A"), m))
    return im.crop(m.getbbox())


def gold(t: float) -> tuple:
    top, bottom = (243, 217, 139), (150, 104, 46)
    return tuple(round(a + (b - a) * t) for a, b in zip(top, bottom)) + (255,)


def badge(gem: Image.Image, size: int = 1024, ring: float = 0.045, fill: float = 0.66) -> Image.Image:
    S = 2
    n = size * S
    out = Image.new("RGBA", (n, n), (0, 0, 0, 0))
    r = n / 2 * 0.965
    c = n / 2
    # Gold ring: a vertical gradient disc, then the dark face on top of it.
    grad = Image.new("RGBA", (n, n))
    gd = ImageDraw.Draw(grad)
    for y in range(n):
        gd.line([(0, y), (n, y)], fill=gold(y / n))
    disc = Image.new("L", (n, n), 0)
    ImageDraw.Draw(disc).ellipse([c - r, c - r, c + r, c + r], fill=255)
    out.paste(grad, (0, 0), disc)
    inner = r * (1 - ring)
    face = Image.new("RGBA", (n, n), (0, 0, 0, 0))
    fd = ImageDraw.Draw(face)
    steps = 60
    for i in range(steps):  # radial gradient, lighter in the middle
        t = i / steps
        rr = inner * (1 - t)
        col = tuple(round(a + (b - a) * t) for a, b in zip((14, 13, 18), (36, 31, 44))) + (255,)
        fd.ellipse([c - rr, c - rr, c + rr, c + rr], fill=col)
    out.alpha_composite(face)
    # The crystal with a soft glow of its own colours behind it.
    h = round(n * fill)
    w = round(gem.width * h / gem.height)
    g = gem.resize((w, h), Image.LANCZOS)
    x, y = round(c - w / 2), round(c - h / 2)
    glow = Image.new("RGBA", (n, n), (0, 0, 0, 0))
    glow.alpha_composite(g, (x, y))
    glow = glow.filter(ImageFilter.GaussianBlur(n * 0.035))
    glow.putalpha(glow.getchannel("A").point(lambda v: round(v * 0.75)))
    clip = Image.new("L", (n, n), 0)
    ImageDraw.Draw(clip).ellipse([c - inner, c - inner, c + inner, c + inner], fill=255)
    glow.putalpha(ImageChops.multiply(glow.getchannel("A"), clip))
    out.alpha_composite(glow)
    out.alpha_composite(g, (x, y))
    return out.resize((size, size), Image.LANCZOS)


def bare(gem: Image.Image, size: int) -> Image.Image:
    out = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    h = round(size * 0.94)
    w = round(gem.width * h / gem.height)
    g = gem.resize((w * 4, h * 4), Image.LANCZOS).resize((w, h), Image.LANCZOS)
    out.alpha_composite(g, ((size - w) // 2, (size - h) // 2))
    return out


def main() -> None:
    gem = crystal()
    gem.save(ART / "crystal.png")
    big = badge(gem)
    big.save(ROOT / "build" / "appicon.png")
    big.resize((128, 128), Image.LANCZOS).save(ROOT / "frontend" / "public" / "emblem.png")
    bare(gem, 64).save(ROOT / "internal" / "assets" / "tray.png")
    ext = ROOT / "browser-extension" / "icons"
    ext.mkdir(exist_ok=True)
    for s in (16, 32, 48, 128):
        # Small sizes: a thinner ring and a bigger crystal read better.
        img = badge(gem, 512, ring=0.03 if s <= 32 else 0.045, fill=0.74 if s <= 32 else 0.66)
        img.resize((s, s), Image.LANCZOS).save(ext / f"icon-{s}.png")
    Image.open(ART / "logo.png").convert("RGBA").save(ROOT / "docs" / "logo.png")
    print("icons written")


if __name__ == "__main__":
    main()
