"""Build the game-language tables the price check uses to read items copied
from a non-English game client (Python stdlib only).

For each language: the item text's fixed words (header prefixes, rarities,
flags, the advanced copy's modifier headers) and the item, unique and gem
names with their English names. The source is Exiled Exchange 2's data
(https://github.com/Kvan7/Exiled-Exchange-2, MIT licence), which is generated
from the game files; see NOTICE.

    python build/locale/build_locale.py de [fr es ...]

Output: internal/overlay/data/locale/<lang>.json, embedded in the app. The
modifier lines are not here: they are matched against the trade site's own
catalog in that language (same stat ids as the English one) at run time.
"""
import datetime
import json
import pathlib
import re
import sys
import urllib.request

ROOT = pathlib.Path(__file__).resolve().parents[2]
OUT = ROOT / 'internal' / 'overlay' / 'data' / 'locale'
REPO = 'Kvan7/Exiled-Exchange-2'
BASE = f'https://raw.githubusercontent.com/{REPO}/master/renderer/public/data'
UA = 'MrW-Overlay-Locale/0.1 (+https://github.com/kadircelebi/mrw-overlay-poe2)'
# Our language code -> Exiled Exchange 2's folder. Thai is not here: its folder
# has no item names or client strings yet.
FOLDERS = {'de': 'de', 'fr': 'fr', 'es': 'es', 'pt': 'pt', 'ru': 'ru', 'ja': 'ja', 'ko': 'ko',
           'zh-Hant': 'cmn-Hant'}

# Client strings the reader needs: plain texts, and the patterns (JavaScript
# regexes, turned into Go syntax) for superior/unidentified names and the
# advanced copy's modifier headers.
TEXTS = ['RARITY_NORMAL', 'RARITY_MAGIC', 'RARITY_RARE', 'RARITY_UNIQUE', 'RARITY_GEM', 'RARITY_CURRENCY',
         'RARITY_DIVCARD', 'RARITY_QUEST', 'RARITY', 'ITEM_CLASS', 'ITEM_LEVEL', 'GEM_LEVEL', 'STACK_SIZE', 'SOCKETS',
         'QUALITY', 'PHYSICAL_DAMAGE', 'ELEMENTAL_DAMAGE', 'LIGHTNING_DAMAGE', 'COLD_DAMAGE', 'FIRE_DAMAGE',
         'CRIT_CHANCE', 'ATTACK_SPEED', 'ARMOUR', 'EVASION', 'ENERGY_SHIELD', 'RUNIC_WARD', 'BLOCK_CHANCE',
         'CORRUPTED', 'DOUBLE_CORRUPTED', 'MIRRORED', 'SANCTIFIED', 'FRACTURED_ITEM', 'UNMODIFIABLE',
         'PREFIX_MODIFIER', 'SUFFIX_MODIFIER', 'IMPLICIT_MODIFIER', 'ENCHANT_MODIFIER', 'CORRUPTED_MODIFIER',
         'CRAFTED_MODIFIER', 'FRACTURED_MODIFIER', 'DESECRATED_MODIFIER', 'UNIQUE_MODIFIER', 'VAAL_UNIQUE_MODIFIER',
         'UNSCALABLE_VALUE', 'REQUIRES', 'CHARM_SLOTS', 'BASE_SPIRIT', 'WAYSTONE_TIER', 'GRANTS_SKILL',
         'RELOAD_SPEED', 'AREA_LEVEL', 'MAP_TIER', 'WAYSTONE_REVIVES', 'WAYSTONE_PACK_SIZE', 'WAYSTONE_MAGIC_MONSTERS',
         'WAYSTONE_RARE_MONSTERS', 'WAYSTONE_DROP_CHANCE', 'WAYSTONE_RARITY', 'WAYSTONE_MONSTER_RARITY',
         'WAYSTONE_EFFECTIVENESS', 'TALISMAN_TIER', 'TIMELESS_RADIUS', 'TIMELESS_SMALL_PASSIVES',
         'TIMELESS_NOTABLE_PASSIVES', 'VEILED_PREFIX', 'VEILED_SUFFIX']
PATTERNS = ['ITEM_SUPERIOR', 'ITEM_EXCEPTIONAL', 'UNIDENTIFIED', 'MODIFIER_LINE', 'MODIFIER_INCREASED',
            'REQUIRES_LINE', 'FLASK_CHARGES']


def get(url):
    req = urllib.request.Request(url, headers={'User-Agent': UA})
    with urllib.request.urlopen(req, timeout=90) as resp:
        return resp.read().decode('utf-8')


def js_strings(source):
    """KEY: 'text' and KEY: /pattern/flags entries of a client_strings.js."""
    out = {}
    for m in re.finditer(r"^\s+([A-Z_0-9]+):\s*'((?:[^'\\]|\\.)*)',?\s*$", source, re.M):
        out[m[1]] = re.sub(r"\\(.)", r"\1", m[2])
    for m in re.finditer(r"^\s+([A-Z_0-9]+):\s*/(.*)/[a-z]*,?\s*$", source, re.M):
        # Go's regexp names groups (?P<name>…).
        out[m[1]] = re.sub(r'\(\?<([a-zA-Z_]+)>', r'(?P<\1>', m[2])
    return out


def build(lang):
    folder = FOLDERS[lang]
    en, local = js_strings(get(f'{BASE}/en/client_strings.js')), js_strings(get(f'{BASE}/{folder}/client_strings.js'))
    texts = {k: [en[k], local[k]] for k in TEXTS if k in en and k in local}
    patterns = {k: [en[k], local[k]] for k in PATTERNS if k in en and k in local}
    missing = [k for k in TEXTS + PATTERNS if k not in texts and k not in patterns]
    names = {'item': [], 'unique': [], 'gem': []}
    for line in get(f'{BASE}/{folder}/items.ndjson').splitlines():
        if not line.strip():
            continue
        r = json.loads(line)
        if not r.get('name') or not r.get('refName'):
            continue
        cat = (r.get('craftable') or {}).get('category', '')
        if r['namespace'] == 'ITEM':
            names['item'].append([r['name'], r['refName'], cat])
        elif r['namespace'] == 'UNIQUE':
            names['unique'].append([r['name'], r['refName'], (r.get('unique') or {}).get('base', '')])
        elif r['namespace'] == 'GEM':
            names['gem'].append([r['name'], r['refName'], cat])
    for key in names:
        seen, kept = set(), []
        for row in names[key]:
            if tuple(row) not in seen:
                seen.add(tuple(row))
                kept.append(row)
        names[key] = sorted(kept)
    OUT.mkdir(parents=True, exist_ok=True)
    data = {
        'lang': lang,
        'source': f'https://github.com/{REPO} (MIT), renderer/public/data/{folder}',
        'built': datetime.date.today().isoformat(),
        'texts': texts, 'patterns': patterns, 'names': names,
    }
    path = OUT / f'{lang}.json'
    path.write_text(json.dumps(data, ensure_ascii=False, separators=(',', ':')) + '\n', encoding='utf-8')
    print(f'{lang}: {len(texts)} texts, {len(patterns)} patterns, '
          f'{len(names["item"])} items, {len(names["unique"])} uniques, {len(names["gem"])} gems, '
          f'{path.stat().st_size // 1024} KB' + (f'; missing {missing}' if missing else ''))


if __name__ == '__main__':
    args = sys.argv[1:]
    if '--out' in args:
        i = args.index('--out')
        OUT = pathlib.Path(args[i + 1])
        del args[i:i + 2]
    for code in args or ['de']:
        build(code)
