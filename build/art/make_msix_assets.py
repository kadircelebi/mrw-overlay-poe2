"""Tile and logo images for the Microsoft Store (MSIX) package.

Run from the repository root:  python build/art/make_msix_assets.py
Writes build/windows/msix/Assets/. Small sizes use the bare crystal (it stays
readable at 16-44 px), larger ones the round emblem (build/appicon.png).
"""
from pathlib import Path

from PIL import Image

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / "build" / "windows" / "msix" / "Assets"
CRYSTAL = Image.open(ROOT / "build" / "art" / "crystal.png").convert("RGBA")
EMBLEM = Image.open(ROOT / "build" / "appicon.png").convert("RGBA")


def fit(src, w, h, pad):
    """src scaled to fit inside w x h minus pad on each side, centered."""
    box = src.crop(src.getbbox())
    scale = min((w - 2 * pad) / box.width, (h - 2 * pad) / box.height)
    img = box.resize((max(1, round(box.width * scale)), max(1, round(box.height * scale))), Image.LANCZOS)
    canvas = Image.new("RGBA", (w, h), (0, 0, 0, 0))
    canvas.paste(img, ((w - img.width) // 2, (h - img.height) // 2), img)
    return canvas


def main():
    OUT.mkdir(parents=True, exist_ok=True)
    files = {
        # name: (source, width, height, padding)
        "Square44x44Logo.png": (CRYSTAL, 44, 44, 2),
        "Square44x44Logo.targetsize-16.png": (CRYSTAL, 16, 16, 0),
        "Square44x44Logo.targetsize-24.png": (CRYSTAL, 24, 24, 1),
        "Square44x44Logo.targetsize-32.png": (CRYSTAL, 32, 32, 1),
        "Square44x44Logo.targetsize-48.png": (CRYSTAL, 48, 48, 2),
        "Square44x44Logo.targetsize-256.png": (EMBLEM, 256, 256, 4),
        "Square44x44Logo.targetsize-16_altform-unplated.png": (CRYSTAL, 16, 16, 0),
        "Square44x44Logo.targetsize-24_altform-unplated.png": (CRYSTAL, 24, 24, 1),
        "Square44x44Logo.targetsize-32_altform-unplated.png": (CRYSTAL, 32, 32, 1),
        "Square44x44Logo.targetsize-48_altform-unplated.png": (CRYSTAL, 48, 48, 2),
        "Square44x44Logo.targetsize-256_altform-unplated.png": (EMBLEM, 256, 256, 4),
        "StoreLogo.png": (EMBLEM, 50, 50, 1),
        "Square150x150Logo.png": (EMBLEM, 150, 150, 14),
        "Wide310x150Logo.png": (EMBLEM, 310, 150, 14),
        "SplashScreen.png": (EMBLEM, 620, 300, 40),
    }
    for name, (src, w, h, pad) in files.items():
        fit(src, w, h, pad).save(OUT / name, optimize=True)
        print(name, (w, h))


if __name__ == "__main__":
    main()
