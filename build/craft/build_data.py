"""Build the theoretical craft data package from scraped PoE2DB pages.

Input is the scraper's output folder (see scrape_poe2db.py in the craft lab:
<page>.source.json, <page>.mods.json, currency-rules.source.json, manifest.json)
and RePoE's base_items.min.json for the item tags essences spawn on.

Output goes to frontend/public/craft/data and is embedded in the app:
  classes.json             item classes, their defence variants and pages,
                           and which page each released base belongs to
  <page>.mods.json         only the fields and pools the craft engine reads
  special-currencies.json  essences and abyssal bones for every class
  currency-rules.source.json, manifest.json, LICENSE.txt

Pages whose weights PoE2DB does not know (every modifier weighs 1) are left
out: rolling them would show made-up odds.

    python build/craft/build_data.py --scraped D:/projects/poe2_craft_lab/data_all \
        --base-items D:/projects/poe2_craft_lab/repoe/base_items.min.json
"""
import argparse
import datetime
import json
import pathlib
import re

ROOT = pathlib.Path(__file__).resolve().parents[2]
OUT = ROOT / 'frontend' / 'public' / 'craft' / 'data'

ATTRS = ['str', 'dex', 'int', 'str_dex', 'str_int', 'dex_int']

# id, Item Class line of a copied item, trade category, RePoE item_class,
# defence variants (page suffixes; None = a single page with the plain name).
CLASSES = [
    ('gloves', 'Gloves', 'armour.gloves', 'Gloves', ('Gloves', ATTRS)),
    ('boots', 'Boots', 'armour.boots', 'Boots', ('Boots', ATTRS)),
    ('helmet', 'Helmets', 'armour.helmet', 'Helmet', ('Helmets', ATTRS)),
    ('body', 'Body Armours', 'armour.chest', 'Body Armour', ('Body_Armours', ATTRS)),
    ('shield', 'Shields', 'armour.shield', 'Shield', ('Shields', ['str', 'str_dex', 'str_int'])),
    ('buckler', 'Bucklers', 'armour.buckler', 'Buckler', ('Bucklers', None)),
    ('focus', 'Foci', 'armour.focus', 'Focus', ('Foci', None)),
    ('quiver', 'Quivers', 'armour.quiver', 'Quiver', ('Quivers', None)),
    ('amulet', 'Amulets', 'accessory.amulet', 'Amulet', ('Amulets', None)),
    ('ring', 'Rings', 'accessory.ring', 'Ring', ('Rings', None)),
    ('belt', 'Belts', 'accessory.belt', 'Belt', ('Belts', None)),
    ('bow', 'Bows', 'weapon.bow', 'Bow', ('Bows', None)),
    ('crossbow', 'Crossbows', 'weapon.crossbow', 'Crossbow', ('Crossbows', None)),
    ('onemace', 'One Hand Maces', 'weapon.onemace', 'One Hand Mace', ('One_Hand_Maces', None)),
    ('twomace', 'Two Hand Maces', 'weapon.twomace', 'Two Hand Mace', ('Two_Hand_Maces', None)),
    ('warstaff', 'Quarterstaves', 'weapon.warstaff', 'Warstaff', ('Quarterstaves', None)),
    ('spear', 'Spears', 'weapon.spear', 'Spear', ('Spears', None)),
    ('talisman', 'Talismans', 'weapon.talisman', 'Talisman', ('Talismans', None)),
    ('sceptre', 'Sceptres', 'weapon.sceptre', 'Sceptre', ('Sceptres', None)),
    ('staff', 'Staves', 'weapon.staff', 'Staff', ('Staves', None)),
    ('wand', 'Wands', 'weapon.wand', 'Wand', ('Wands', None)),
]

POOLS = {'normal', 'desecrated', 'essence', 'perfect_essence', 'marksman', 'decay'}
FIELDS = ['source_id', 'pool', 'affix', 'name', 'families', 'tier', 'required_ilvl',
          'weight', 'text', 'ranges', 'tags', 'spawn_tags']


def read(path):
    return json.loads(pathlib.Path(path).read_text(encoding='utf-8-sig'))


def write(path, value):
    text = json.dumps(value, ensure_ascii=False, separators=(',', ':'))
    pathlib.Path(path).write_text(text + '\n', encoding='utf-8')


def class_tags(base_items, item_class):
    """Tags every released base of the class carries."""
    sets = [set(b['tags']) for b in base_items.values()
            if b.get('item_class') == item_class and b.get('release_state') == 'released'
            and b.get('domain') == 'item']
    if not sets:
        raise SystemExit(f'no released bases for {item_class}')
    return set.intersection(*sets)


def spawns_on(mod, tags):
    """The craft engine's rule: untagged rows fit everything."""
    spawn = [t for t in mod['spawn_tags'] if t != 'default']
    return not spawn or any(t in tags for t in spawn)


def placeholder_weights(mods):
    normal = [m for m in mods if m['pool'] == 'normal' and m['affix'] in ('Prefix', 'Suffix')]
    return bool(normal) and all(m['weight'] <= 1 for m in normal)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--scraped', type=pathlib.Path, required=True)
    parser.add_argument('--base-items', type=pathlib.Path, required=True)
    parser.add_argument('--out', type=pathlib.Path, default=OUT)
    args = parser.parse_args()
    base_items = read(args.base_items)
    scraped_manifest = read(args.scraped / 'manifest.json')
    args.out.mkdir(parents=True, exist_ok=True)
    for old in args.out.glob('*.mods.json'):
        old.unlink()

    classes, pages_used, essences, bases = [], [], {}, {}
    for cid, item_class, category, repoe_class, (stem, attrs) in CLASSES:
        common = class_tags(base_items, repoe_class)
        variants = []
        for attr in (attrs or [None]):
            page = f'{stem}_{attr}' if attr else stem
            source = read(args.scraped / f'{page}.source.json')
            mods = read(args.scraped / f'{page}.mods.json')
            if placeholder_weights(mods['mods']):
                raise SystemExit(f'{page}: PoE2DB has no real weights for this page')
            page_tags = [t for t in (mods['options'].get('tags') or '').split(',') if t]
            # 'default' is on every row, so as an item tag it would let every
            # essence variant through.
            tags = sorted((common | set(page_tags)) - {'default'})
            compact = {
                'page': page,
                'base': mods['base'],
                'options': mods['options'],
                'tags': tags,
                'mods': [{k: m[k] for k in FIELDS} for m in mods['mods'] if m['pool'] in POOLS],
            }
            write(args.out / f'{page}.mods.json', compact)
            variants.append({'page': page, 'attr': attr or ''})
            # Which released bases this page covers: all of a single-page
            # class, or those carrying the page's defence tags.
            for b in base_items.values():
                if (b.get('item_class') == repoe_class and b.get('release_state') == 'released'
                        and b.get('domain') == 'item' and set(page_tags) <= set(b['tags'])):
                    bases.setdefault(b['name'], page)
            pages_used.append(page)
            code = mods['options']['ItemClassesCode']
            # An essence belongs to this class only if one of its rows can
            # spawn on the class's tags; a page also lists other classes' rows.
            fits = {(m['pool'], m['name']) for m in compact['mods']
                    if m['pool'] in ('essence', 'perfect_essence') and spawns_on(m, tags)}
            for pool in ('essence', 'perfect_essence'):
                for row in source.get(pool, []):
                    if row.get('IsAlloy') or 'Abyss' in row.get('Code', ''):
                        continue
                    href = re.search(r'href="([^"]+)"', row['Name'])
                    if not href or (pool, re.sub('<[^>]+>', '', row['Name'])) not in fits:
                        continue
                    key = href[1].lower().replace('_', '-')
                    image = re.search(r'src="([^"]+)"', row['Name'])
                    rule = essences.setdefault(key, {
                        'name': re.sub('<[^>]+>', '', row['Name']),
                        'operation': 'essence',
                        'beforeRarity': ['Rare'] if row.get('Removes') else ['Magic'],
                        'removes': bool(row.get('Removes')),
                        'pool': pool,
                        'icon': image[1].split('/image/')[-1] if image else '',
                        'beforeClassIds': [],
                    })
                    if code not in rule['beforeClassIds']:
                        rule['beforeClassIds'].append(code)
        classes.append({'id': cid, 'itemClass': item_class, 'category': category,
                        'classCode': read(args.out / f'{variants[0]["page"]}.mods.json')['options']['ItemClassesCode'],
                        'variants': variants})

    rules = read(args.scraped / 'currency-rules.source.json')
    special = {}
    for key, raw in rules.items():
        if not re.fullmatch(r'(gnawed|preserved|ancient)-(rib|jawbone|collarbone|cranium)', key):
            continue
        rule = dict(raw)
        rule.pop('beforeMin_mod_lv', None)
        rule.pop('beforeMax_mod_lv', None)
        rule.update(operation='desecrate', beforeRarity=['Rare'])
        # The items' own descriptions correct PoE2DB's simulator, which swaps
        # these two: Gnawed caps the item level, Ancient raises the modifier level.
        if key.startswith('gnawed-'):
            rule['maxItemLevel'] = 64
        if key.startswith('ancient-'):
            rule['minimum'] = 40
        special[key] = rule
    special.update(essences)

    now = datetime.datetime.now(datetime.timezone.utc).isoformat()
    write(args.out / 'classes.json', {'generated_at': now, 'classes': classes, 'bases': dict(sorted(bases.items()))})
    write(args.out / 'special-currencies.json', {
        'source': 'https://poe2db.tw/us/Essence',
        'bone_source': 'https://poe2db.tw/us/Rise_of_the_Abyssal_items',
        'generated_at': now, 'rules': special})
    write(args.out / 'currency-rules.source.json', rules)
    wanted = {f'https://poe2db.tw/us/{p}' for p in pages_used}
    write(args.out / 'manifest.json', {
        'fetched_at_utc': scraped_manifest['fetched_at_utc'],
        'built_at_utc': now,
        'scope': pages_used,
        'sources': [s for s in scraped_manifest['sources']
                    if s['url'] in wanted or 'ModsView' in s['url']],
        'license': 'CC BY-NC-SA 3.0, data from https://poe2db.tw (see LICENSE.txt)',
    })
    (args.out / 'LICENSE.txt').write_text(LICENSE, encoding='utf-8')
    print(f'{len(classes)} classes, {len(pages_used)} pages, {len(bases)} bases, {len(essences)} essences, '
          f'{len(special) - len(essences)} bones')


LICENSE = """The files in this folder are modifier, essence and currency data taken from
PoE2DB (https://poe2db.tw), reduced to the fields the craft calculator uses
and reformatted. PoE2DB publishes this data under the Creative Commons
Attribution-NonCommercial-ShareAlike 3.0 licence
(https://creativecommons.org/licenses/by-nc-sa/3.0/), and these files are
shared under the same licence.

The game data itself originates from Path of Exile 2 by Grinding Gear Games.
This project is not affiliated with or endorsed by Grinding Gear Games or
PoE2DB.

The application's source code is licensed separately under the MIT licence
(see LICENSE in the repository root); that licence does not cover this folder.
"""

if __name__ == '__main__':
    main()
