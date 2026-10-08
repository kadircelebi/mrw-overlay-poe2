"""Refresh the game data embedded in the app, in one go (Python stdlib only).

    python build/refresh_data.py              fetch, build into a work folder, report
    python build/refresh_data.py --apply      the same, then put it in the app and test
    python build/refresh_data.py --work DIR --skip-fetch [--apply]
                                              rebuild from an earlier fetch

Sources, all saved with their SHA-256 in <work>/fetch.json:
  PoE2DB      modifier pages of every craft class and jewel, the currency rules
              and the Runeshape Combinations page (scrape_poe2db.py, 3 s apart)
  RePoE       base_items.min.json (item tags and base numbers)
  Craft of Exile  the current beta data file: rune pool weights, and the
              reference the modifier weights are checked against

Weight rule (PoE2DB is the base source; GGG publishes no weights and the game
files hold 1 for every modifier):
  * PoE2DB weight 1 on a normal modifier means "unknown": Craft of Exile's
    weight is used when it has a real one (weight-overrides.json). Without
    one, a weight from the previous overrides file is kept and reported;
    with neither the build stops.
  * Both real and different: PoE2DB's stays, the row is marked "disputed"
    with Craft of Exile's as "alt" (weight-notes.json, shown in the craft
    window).
  * Desecrated modifiers with weight 1 are marked "unknown": every source
    gives 1, so they are rolled as equally likely.
  * Jewel modifiers really are equally likely and are not checked.

The report (<work>/report.md) lists what changed against the data in the app:
pages, modifiers added or removed, weight changes, new currencies, essences,
bones and runes, bases, Runeshape rewards, and every weight decision. Read it
before --apply; nothing goes into the app without it.
"""
import argparse
import datetime
import hashlib
import importlib.util
import json
import pathlib
import re
import shutil
import subprocess
import sys
import urllib.request

ROOT = pathlib.Path(__file__).resolve().parents[1]
CRAFT = ROOT / 'build' / 'craft'
APP_CRAFT = ROOT / 'frontend' / 'public' / 'craft' / 'data'
APP_RUNESHAPE = ROOT / 'internal' / 'overlay' / 'data' / 'runeshape.json'
GENERATED = ['weight-overrides.json', 'weight-notes.json', 'special-weights.json']
UA = 'MrW-Overlay-CraftData/0.3 (+https://github.com/kadircelebi/mrw-overlay-poe2)'
REPOE_BASES = 'https://repoe-fork.github.io/poe2/base_items.min.json'
COE_SITE = 'https://beta.craftofexile.com/?game=poe2'
COE_BASE = 'https://beta.craftofexile.com/'
RUNESHAPE = 'https://poe2db.tw/us/Runeshape_Combinations'


def load_module(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


build_data = load_module('build_data', CRAFT / 'build_data.py')


def read(path):
    return json.loads(pathlib.Path(path).read_text(encoding='utf-8-sig'))


def write(path, value):
    pathlib.Path(path).write_text(json.dumps(value, ensure_ascii=False, indent=1) + '\n', encoding='utf-8')


def now():
    return datetime.datetime.now(datetime.timezone.utc)


def get(url):
    request = urllib.request.Request(url, headers={'User-Agent': UA})
    with urllib.request.urlopen(request, timeout=90) as response:
        return response.read()


def run(*args, cwd=ROOT):
    print('>', ' '.join(str(a) for a in args), flush=True)
    subprocess.run([str(a) for a in args], check=True, cwd=cwd)


def craft_pages():
    pages = []
    for _, _, _, _, (stem, attrs) in build_data.CLASSES:
        pages += [f'{stem}_{a}' if a else stem for a in (attrs or [None])]
    return pages


# --- fetch ------------------------------------------------------------------

def fetch(work):
    sources = []

    def save(url, path):
        raw = get(url)
        path.write_bytes(raw)
        sources.append({'url': url, 'sha256': hashlib.sha256(raw).hexdigest(), 'bytes': len(raw),
                        'fetched_at_utc': now().isoformat()})
        return raw

    pages = craft_pages() + build_data.JEWEL_PAGES
    run(sys.executable, CRAFT / 'scrape_poe2db.py', '--pages', *pages, '--output', work / 'poe2db')
    save(RUNESHAPE, work / 'runeshape.html')
    save(REPOE_BASES, work / 'base_items.min.json')
    site = get(COE_SITE).decode('utf-8', 'replace')
    found = re.search(r'json/poe2/([0-9.]+)/data\.json\?v=(\d+)', site)
    if not found:
        raise SystemExit("Craft of Exile: data file not found on the beta page; did the site change?")
    version, stamp = found[1], found[2]
    (work / 'coe').mkdir(exist_ok=True)
    save(f'{COE_BASE}json/poe2/{version}/data.json?v={stamp}', work / 'coe' / 'data.json')
    save(f'{COE_BASE}json/poe2/{version}/localization/english.json?v={stamp}', work / 'coe' / 'english.json')
    write(work / 'fetch.json', {'fetched_at_utc': now().isoformat(), 'coe_version': version, 'sources': sources})


# --- weights ------------------------------------------------------------------

def loose_json(path):
    """Craft of Exile's files are JSON wrapped in a little script."""
    raw = pathlib.Path(path).read_text(encoding='utf-8')
    start = min(i for i in (raw.find('{'), raw.find('[')) if i >= 0)
    end = max(raw.rfind('}'), raw.rfind(']'))
    return json.loads(raw[start:end + 1])


def norm(value):
    return re.sub(r'[^a-z0-9]', '', str(value).lower())


def signature(side, families, name, level, ranges):
    return (side, tuple(sorted(families)), name, int(level),
            tuple(sorted((float(a), float(b)) for a, b in ranges)))


def coe_reference(coe, language):
    """Craft of Exile's normal prefix/suffix weights per page, by signature
    (side, families, affix name, level, variable ranges): the match used by
    the 2026-10-05 audit, which found every PoE2DB row."""
    groups = {g['id']: g for g in coe['modgroups']['entries']}
    families = {f['id']: f['key'] for f in coe['families']['entries']}
    mods = {m['id']: m for m in coe['mods']['entries']}
    pages = {}
    for cls in coe['classes']['entries']:
        ref = {}
        for mod_id, weight in (coe['classmods'].get(str(cls['id'])) or {}).items():
            mod = mods.get(int(mod_id))
            group = mod and groups.get(mod['group'])
            if not mod or not group or group['type'] not in (1, 2) or not weight or mod.get('label') is None:
                continue
            key = signature('Prefix' if group['type'] == 1 else 'Suffix',
                            [families.get(f) for f in group['families']], language[mod['label']],
                            mod['minlvl'], [s['range'] for s in mod['stats']
                                            if s.get('values') and s['range'][0] != s['range'][1]])
            ref.setdefault(key, set()).add(weight)
        if cls.get('label') is not None:
            pages[norm(language[cls['label']])] = ref
    return pages


def weigh(work, previous):
    coe, language = loose_json(work / 'coe' / 'data.json'), loose_json(work / 'coe' / 'english.json')
    reference = coe_reference(coe, language)
    fetched = read(work / 'fetch.json')
    data_file = next(s['url'] for s in fetched['sources'] if '/data.json' in s['url'])
    overrides, notes, kept, unresolved, disputes = {}, {}, [], [], []
    counts = {'compared': 0, 'equal': 0, 'from_coe': 0, 'disputed': 0, 'no_reference': 0}
    for page in craft_pages():
        rows = read(work / 'poe2db' / f'{page}.mods.json')['mods']
        ref = reference.get(norm(page), {})
        for row in rows:
            if row['pool'] == 'desecrated' and row['affix'] in ('Prefix', 'Suffix') and row['weight'] <= 1:
                notes.setdefault(page, {})[row['source_id']] = {'kind': 'unknown'}
                continue
            if row['pool'] != 'normal' or row['affix'] not in ('Prefix', 'Suffix'):
                continue
            counts['compared'] += 1
            weights = ref.get(signature(row['affix'], row['families'], row['name'], row['required_ilvl'],
                                        [(r['min'], r['max']) for r in row['ranges'] if r['min'] != r['max']]), set())
            coe_weight = next(iter(weights)) if len(weights) == 1 else None
            real = coe_weight is not None and coe_weight > 1
            if row['weight'] > 1:
                if not real:
                    counts['no_reference'] += 1
                elif coe_weight == row['weight']:
                    counts['equal'] += 1
                else:
                    counts['disputed'] += 1
                    notes.setdefault(page, {})[row['source_id']] = {'kind': 'disputed', 'alt': coe_weight}
                    disputes.append((page, row, coe_weight))
                continue
            if real:
                counts['from_coe'] += 1
                overrides.setdefault(page, {})[row['source_id']] = coe_weight
            elif row['source_id'] in previous.get(page, {}):
                overrides.setdefault(page, {})[row['source_id']] = previous[page][row['source_id']]
                kept.append((page, row, previous[page][row['source_id']]))
            else:
                unresolved.append((page, row))
    stamp = now().date().isoformat()
    write(work / 'staging' / 'weight-overrides.json', {
        'source': 'Craft of Exile (https://www.craftofexile.com)',
        'data_file': f'{data_file} (fetched {stamp})',
        'note': 'Normal modifiers PoE2DB lists with DropChance 1 (unknown weight), keyed by PoE2DB source_id. '
                'Generated by build/refresh_data.py.',
        'pages': overrides})
    write(work / 'staging' / 'weight-notes.json', {
        'note': 'disputed: PoE2DB and Craft of Exile give different real weights (PoE2DB kept, alt = Craft of '
                'Exile); unknown: no source knows the weight. Generated by build/refresh_data.py.',
        'pages': notes})
    run(sys.executable, CRAFT / 'extract_special_weights.py', work / 'coe' / 'data.json',
        work / 'coe' / 'english.json', f'{data_file} (fetched {stamp})', work / 'staging' / 'special-weights.json')
    return {'counts': counts, 'kept': kept, 'unresolved': unresolved, 'disputes': disputes,
            'overrides': overrides, 'notes': notes}


# --- build --------------------------------------------------------------------

def build(work):
    staging = work / 'staging'
    staging.mkdir(exist_ok=True)
    weights = weigh(work, read(CRAFT / 'weight-overrides.json')['pages'])
    if weights['unresolved']:
        return weights, False
    run(sys.executable, CRAFT / 'build_data.py', '--scraped', work / 'poe2db', '--jewels', work / 'poe2db',
        '--base-items', work / 'base_items.min.json', '--out', staging / 'craft',
        '--overrides', staging / 'weight-overrides.json', '--special-weights', staging / 'special-weights.json',
        '--weight-notes', staging / 'weight-notes.json')
    run(sys.executable, ROOT / 'build' / 'expedition' / 'build_recipes.py', '--page', work / 'runeshape.html',
        '--out', staging / 'runeshape.json')
    return weights, True


# --- report ---------------------------------------------------------------------

def row_key(row):
    return (row['pool'], row['affix'], tuple(row['families']), row['required_ilvl'], row['name'])


def short(row):
    return f"{row['pool']} {row['affix']} ilvl {row['required_ilvl']}: {row['text'].splitlines()[0][:70]}"


def compare_pages(old_dir, new_dir):
    lines, totals = [], {'added': 0, 'removed': 0, 'reweighted': 0}
    old_pages = {p.name for p in old_dir.glob('*.mods.json')}
    new_pages = {p.name for p in new_dir.glob('*.mods.json')}
    for name in sorted(new_pages - old_pages):
        lines.append(f'- new page {name}')
    for name in sorted(old_pages - new_pages):
        lines.append(f'- **page gone** {name}')
    for name in sorted(old_pages & new_pages):
        old = {row_key(r): r for r in read(old_dir / name)['mods']}
        new = {row_key(r): r for r in read(new_dir / name)['mods']}
        added = [new[k] for k in new.keys() - old.keys()]
        removed = [old[k] for k in old.keys() - new.keys()]
        changed = [(old[k], new[k]) for k in old.keys() & new.keys() if old[k]['weight'] != new[k]['weight']]
        totals['added'] += len(added)
        totals['removed'] += len(removed)
        totals['reweighted'] += len(changed)
        if not (added or removed or changed):
            continue
        lines.append(f'- {name[:-10]}: +{len(added)} / -{len(removed)} modifiers, {len(changed)} weight changes')
        for r in sorted(added, key=short)[:8]:
            lines.append(f'  - added {short(r)}')
        for r in sorted(removed, key=short)[:8]:
            lines.append(f'  - removed {short(r)}')
        for o, n in sorted(changed, key=lambda p: short(p[1]))[:8]:
            lines.append(f"  - weight {o['weight']} → {n['weight']}: {short(n)}")
    return lines, totals


def keyed_diff(label, old, new):
    added, removed = sorted(set(new) - set(old)), sorted(set(old) - set(new))
    out = []
    if added:
        out.append(f'- {label} added: ' + ', '.join(added))
    if removed:
        out.append(f'- {label} **removed**: ' + ', '.join(removed))
    return out


def report(work, weights, built):
    staging = work / 'staging'
    fetched = read(work / 'fetch.json')
    scraped = read(work / 'poe2db' / 'summary.json')
    out = [f'# Data refresh {now():%Y-%m-%d %H:%M} UTC', '',
           f"Fetched {fetched['fetched_at_utc']}; Craft of Exile beta {fetched['coe_version']}.", '']
    out += ['## Checks', ''] + [f'- {v}' for v in scraped.get('validation', [])] + ['']
    c = weights['counts']
    out += ['## Weights', '',
            f"{c['compared']} normal modifiers checked: {c['equal']} agree with Craft of Exile, "
            f"{c['from_coe']} unknown in PoE2DB taken from Craft of Exile, {c['disputed']} disputed, "
            f"{c['no_reference']} without a Craft of Exile reference (PoE2DB kept).", '']
    old_overrides = read(CRAFT / 'weight-overrides.json')['pages']
    old_count = sum(len(v) for v in old_overrides.values())
    new_count = sum(len(v) for v in weights['overrides'].values())
    out.append(f'Overrides: {old_count} before, {new_count} now.')
    if weights['unresolved']:
        out += ['', '**Unresolved (the build stopped):** PoE2DB gives 1 and Craft of Exile has no real weight.']
        out += [f'- {p}: {short(r)}' for p, r in weights['unresolved']]
    if weights['kept']:
        out += ['', 'Kept from the previous overrides (Craft of Exile no longer matches them):']
        out += [f'- {p}: {short(r)} = {w}' for p, r, w in weights['kept']]
    if weights['disputes']:
        out += ['', 'Disputed (PoE2DB kept, marked in the craft window):']
        seen = set()
        for p, r, w in weights['disputes']:
            key = (r['families'][0], r['required_ilvl'], r['weight'], w)
            if key not in seen:
                seen.add(key)
                out.append(f"- {r['text'].splitlines()[0][:60]} (ilvl {r['required_ilvl']}): PoE2DB {r['weight']}, "
                           f"Craft of Exile {w} — e.g. {p}")
    unknown = sum(1 for page in weights['notes'].values() for n in page.values() if n['kind'] == 'unknown')
    out += ['', f'{unknown} desecrated modifiers marked "unknown" (weight 1 everywhere).']
    old_runes = read(CRAFT / 'special-weights.json')['pages']
    new_runes = read(staging / 'special-weights.json')['pages']
    changed = sorted((page, key, old_runes.get(page, {}).get(key), value)
                     for page in new_runes for key, value in new_runes[page].items()
                     if old_runes.get(page, {}).get(key) != value)
    gone = sorted((page, key) for page in old_runes for key in old_runes[page] if key not in new_runes.get(page, {}))
    out.append(f'Rune pool weights (Craft of Exile): {len(changed)} new or changed, {len(gone)} gone.')
    out += [f'- {page} {key}: {old} → {new}' for page, key, old, new in changed[:20]]
    out += [f'- {page} {key}: **gone**' for page, key in gone[:20]]
    out.append('')
    if built:
        lines, totals = compare_pages(APP_CRAFT, staging / 'craft')
        out += ['## Craft data against the app', '',
                f"{totals['added']} modifiers added, {totals['removed']} removed, {totals['reweighted']} weights changed.", '']
        out += lines or ['- no modifier changes']
        old_special, new_special = read(APP_CRAFT / 'special-currencies.json'), read(staging / 'craft' / 'special-currencies.json')
        out += keyed_diff('essences, alloys, bones, liquids', old_special['rules'], new_special['rules'])
        out += keyed_diff('runes', old_special['runes'], new_special['runes'])
        out += keyed_diff('currency rules', read(APP_CRAFT / 'currency-rules.source.json'),
                          read(staging / 'craft' / 'currency-rules.source.json'))
        out += keyed_diff('bases', read(APP_CRAFT / 'classes.json')['bases'],
                          read(staging / 'craft' / 'classes.json')['bases'])
        old_r, new_r = read(APP_RUNESHAPE), read(staging / 'runeshape.json')
        out += ['', '## Runeshape Combinations', '',
                f"{len(old_r['recipes'])} recipes before, {len(new_r['recipes'])} now."]
        out += keyed_diff('rewards', {r['reward'] for r in old_r['recipes']}, {r['reward'] for r in new_r['recipes']})
    text = '\n'.join(out) + '\n'
    (work / 'report.md').write_text(text, encoding='utf-8')
    return text


# --- apply ---------------------------------------------------------------------

def apply(work):
    staging = work / 'staging'
    for old in APP_CRAFT.glob('*'):
        if old.is_file():
            old.unlink()
    for new in (staging / 'craft').glob('*'):
        shutil.copy2(new, APP_CRAFT / new.name)
    shutil.copy2(staging / 'runeshape.json', APP_RUNESHAPE)
    for name in GENERATED:
        shutil.copy2(staging / name, CRAFT / name)
    tests = sorted(str(p.relative_to(ROOT / 'frontend')) for p in (ROOT / 'frontend' / 'scripts').glob('craft-*.test.mjs'))
    run(shutil.which('node') or 'node', '--test', *tests, cwd=ROOT / 'frontend')
    run('go', 'test', './internal/overlay/')


def main():
    parser = argparse.ArgumentParser(description=__doc__.split('\n\n')[0])
    parser.add_argument('--work', type=pathlib.Path,
                        default=ROOT / 'dist' / 'scratch' / f'data-refresh-{now():%Y%m%d-%H%M}')
    parser.add_argument('--skip-fetch', action='store_true', help='rebuild from an earlier --work folder')
    parser.add_argument('--apply', action='store_true', help='put the result in the app and run the tests')
    args = parser.parse_args()
    sys.stdout.reconfigure(encoding='utf-8')  # the report has arrows and game names
    work = args.work.resolve()
    work.mkdir(parents=True, exist_ok=True)
    if not args.skip_fetch:
        fetch(work)
    weights, built = build(work)
    print(report(work, weights, built))
    print(f'Report: {work / "report.md"}')
    if not built:
        raise SystemExit('Build stopped: unresolved weights (see the report).')
    if args.apply:
        apply(work)
        print('Applied. Review the diff (git status) before committing.')


if __name__ == '__main__':
    main()
