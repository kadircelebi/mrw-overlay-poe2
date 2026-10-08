"""Extract the spawn weights of the special rune pools from Craft of Exile.

PoE2DB lists the modifiers that runes like Kolr's Hunt ("Can roll Marksman
modifiers") open up, but every one of them with weight 1, which would make
them practically never roll. The real weights were measured by Krakenbul and
the Prohibited Library Discord and are published in Craft of Exile's data
(changelog 2026-09-03). Only those weights are kept, keyed by the modifier
code PoE2DB also uses (MarksmanInfluenceProjectileDamage1).

Weights can differ between defence types of one class, so they are kept per
craft data page ("Helmets (STR/DEX)" → Helmets_str_dex).

    curl -A "MrW-Overlay-CraftData/0.2 (+github)" \
        "https://beta.craftofexile.com/json/poe2/<version>/data.json" -o coe.json
    curl -A "MrW-Overlay-CraftData/0.2 (+github)" \
        "https://beta.craftofexile.com/json/poe2/<version>/localization/english.json" -o coe-en.json
    python build/craft/extract_special_weights.py coe.json coe-en.json "<data file URL and date>"

An optional fourth argument writes somewhere else (refresh_data.py stages it).
"""
import json
import pathlib
import re
import sys

OUT = pathlib.Path(__file__).resolve().parent / 'special-weights.json'

# Code prefix in the game data → the pool name PoE2DB (and the craft data) use.
POOLS = {
    'MarksmanInfluence': 'marksman',
    'DecayInfluence': 'decay',
    'BerserkInfluence': 'berserking',
    'TimeInfluence': 'chronomancy',
    'SoulInfluence': 'soul',
    'DestructionInfluence': 'destruction',
}


def load(path, opener):
    text = pathlib.Path(path).read_text(encoding='utf-8')
    return json.loads(text[text.index(opener):].rstrip().rstrip(';'))


def page_name(label):
    """'Gloves (STR/DEX)' → 'Gloves_str_dex', 'One Hand Maces' → 'One_Hand_Maces'."""
    m = re.fullmatch(r'(.+?)(?: \(([A-Z/]+)\))?', label)
    stem, attrs = m[1].replace(' ', '_'), (m[2] or '').lower()
    attrs = {'strdex': 'str/dex'}.get(attrs, attrs)
    return f'{stem}_{attrs.replace("/", "_")}' if attrs else stem


def main():
    data, labels = load(sys.argv[1], '{'), load(sys.argv[2], '[')
    mods = {m['id']: m['key'] for m in data['mods']['entries']}
    classes = {str(c['id']): page_name(labels[c['label']]) for c in data['classes']['entries']}
    pages, counts = {}, {pool: set() for pool in POOLS.values()}
    for class_id, class_weights in data['classmods'].items():
        for mod_id, weight in class_weights.items():
            key = mods.get(int(mod_id), '')
            pool = next((p for prefix, p in POOLS.items() if key.startswith(prefix)), None)
            if pool is None or not weight:
                continue
            pages.setdefault(classes[class_id], {})[key] = weight
            counts[pool].add(key)
    out = pathlib.Path(sys.argv[4]) if len(sys.argv) > 4 else OUT
    out.write_text(json.dumps({
        'source': 'Craft of Exile (https://www.craftofexile.com), weights measured by Krakenbul '
                  'and the Prohibited Library Discord',
        'data_file': sys.argv[3] if len(sys.argv) > 3 else '',
        'pages': {p: dict(sorted(w.items())) for p, w in sorted(pages.items())},
    }, indent=1, ensure_ascii=False) + '\n', encoding='utf-8')
    print({p: len(k) for p, k in counts.items()}, len(pages), 'pages')


if __name__ == '__main__':
    main()
