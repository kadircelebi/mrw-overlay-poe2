"""Build the Runeshape Combinations recipe table from PoE2DB's page.

Expedition's Runeshape Combinations panel lists rewards, each made by a fixed,
ordered set of runes. The table lets the app check a reward read off the
screen and recover its count when the recognizer garbles it: of the 48
rewards that come in more than one count, all but seven early ones (two runes,
Lv15-16) are told apart by the number of runes in the row.

Input is the page saved from https://poe2db.tw/us/Runeshape_Combinations
(fetch it once by hand or pass --fetch). Output goes to
internal/overlay/data/runeshape.json and is embedded in the app.

    python build/expedition/build_recipes.py --page path/to/Runeshape_Combinations.html

PoE2DB publishes this data under CC BY-NC-SA 3.0; see NOTICE.
"""
import argparse
import datetime
import html
import json
import pathlib
import re
import urllib.request

ROOT = pathlib.Path(__file__).resolve().parents[2]
OUT = ROOT / 'internal' / 'overlay' / 'data' / 'runeshape.json'
URL = 'https://poe2db.tw/us/Runeshape_Combinations'

# One recipe: the reward in a <span> (a link and the text after it), then
# the area level band, then the runes as links in order.
RECIPE_RE = re.compile(r'<div><span>(.*?)</span>\s*<span class="default small">([^<]*)</span>(.*?)(?=class="d-flex border-top rounded"|$)', re.S)
RUNE_RE = re.compile(r'<a href="([A-Za-z]+)_Rune">')
LEVEL_RE = re.compile(r'\s*\(Level (\d+)\)')
COUNT_RE = re.compile(r'\s+x(\d+)$')
LEAD_COUNT_RE = re.compile(r'^(\d+)x\s+')


def text(fragment):
    return re.sub(r'\s+', ' ', html.unescape(re.sub(r'<[^>]+>', '', fragment))).strip()


def parse(page):
    recipes, seen = [], set()
    for block in page.split('class="d-flex border-top rounded"')[1:]:
        m = RECIPE_RE.search(block)
        if not m:
            continue
        reward, tier, rest = text(m.group(1)), m.group(2).strip(), m.group(3)
        runes = [r.lower() for r in RUNE_RE.findall(rest)]
        count, level = 1, 0
        if c := COUNT_RE.search(reward):
            count, reward = int(c.group(1)), reward[:c.start()]
        elif c := LEAD_COUNT_RE.match(reward):
            count, reward = int(c.group(1)), reward[c.end():]
        if lv := LEVEL_RE.search(reward):
            level, reward = int(lv.group(1)), (reward[:lv.start()] + reward[lv.end():]).strip()
        if not reward or not runes:
            continue
        key = (reward, count, level, tier, tuple(runes))
        if key in seen:  # the page lists every recipe twice
            continue
        seen.add(key)
        recipes.append({'reward': reward, 'count': count, 'level': level, 'tier': tier, 'runes': runes})
    recipes.sort(key=lambda r: (r['reward'], r['count'], r['level'], len(r['runes'])))
    return recipes


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument('--page', help='saved Runeshape_Combinations page')
    ap.add_argument('--fetch', action='store_true', help='download the page instead')
    ap.add_argument('--out', type=pathlib.Path, default=OUT)
    args = ap.parse_args()
    if args.fetch:
        req = urllib.request.Request(URL, headers={'User-Agent': 'MrW-Overlay-POE2 (https://github.com/kadircelebi/mrw-overlay-poe2)'})
        page = urllib.request.urlopen(req, timeout=60).read().decode('utf-8')
    elif args.page:
        page = pathlib.Path(args.page).read_text(encoding='utf-8')
    else:
        ap.error('--page or --fetch')
    recipes = parse(page)
    if len(recipes) < 200:
        raise SystemExit(f'only {len(recipes)} recipes parsed; did the page change?')
    out = {
        'source': URL,
        'license': 'CC BY-NC-SA 3.0 (poe2db.tw); game data (c) Grinding Gear Games',
        'built': datetime.date.today().isoformat(),
        'recipes': recipes,
    }
    args.out.parent.mkdir(parents=True, exist_ok=True)
    args.out.write_text(json.dumps(out, ensure_ascii=False, separators=(',', ':')) + '\n', encoding='utf-8')
    print(f'{len(recipes)} recipes, {len({r["reward"] for r in recipes})} rewards -> {args.out}')


if __name__ == '__main__':
    main()
