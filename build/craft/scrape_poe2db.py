"""Download PoE2DB's modifier pages for the craft data (Python stdlib only).

Parse literal data only; never execute the downloaded JavaScript.
Raw responses and provenance are preserved alongside extracted data.
build_data.py turns the output into the app's craft data; refresh_data.py
runs both.

    python build/craft/scrape_poe2db.py --pages Gloves_str Rings --output <dir>
"""
import argparse
import hashlib
import html as html_module
import json
import re
import time
import urllib.request
from datetime import datetime, timezone
from pathlib import Path


class LiteralParser:
    """Restricted JS object literal reader: objects, arrays, strings, numbers.

    Deliberately rejects expressions, function calls, and executable statements.
    """
    def __init__(self, source, position=0):
        self.source, self.position = source, position

    def whitespace(self):
        while self.position < len(self.source) and self.source[self.position].isspace():
            self.position += 1

    def consume(self, token):
        self.whitespace()
        if not self.source.startswith(token, self.position):
            raise ValueError(f"Expected {token!r} at {self.position}")
        self.position += len(token)

    def value(self):
        self.whitespace()
        char = self.source[self.position]
        if char == '{':
            self.position += 1
            result = {}
            self.whitespace()
            while self.source[self.position] != '}':
                if self.source[self.position] == '"':
                    key = self.value()
                else:
                    match = re.match(r'[A-Za-z_$][\w$]*', self.source[self.position:])
                    if not match:
                        raise ValueError(f"Invalid key at {self.position}")
                    key = match[0]
                    self.position += len(key)
                self.consume(':')
                if key in result:
                    raise ValueError(f"Duplicate key: {key}")
                result[key] = self.value()
                self.whitespace()
                if self.source[self.position] == '}':
                    break
                self.consume(',')
            self.consume('}')
            return result
        if char == '[':
            self.position += 1
            result = []
            self.whitespace()
            while self.source[self.position] != ']':
                result.append(self.value())
                self.whitespace()
                if self.source[self.position] == ']':
                    break
                self.consume(',')
            self.consume(']')
            return result
        if char == '"':
            value, length = json.JSONDecoder().raw_decode(self.source[self.position:])
            self.position += length
            return value
        for token, value in [('true', True), ('false', False), ('null', None)]:
            if self.source.startswith(token, self.position):
                self.position += len(token)
                return value
        match = re.match(r'-?(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?', self.source[self.position:])
        if not match:
            raise ValueError(f"Unsupported literal at {self.position}: {self.source[self.position:self.position+60]}")
        self.position += len(match[0])
        return float(match[0]) if any(c in match[0] for c in '.eE') else int(match[0])


def download(url, path):
    request = urllib.request.Request(url, headers={'User-Agent': 'MrW-Overlay-CraftData/0.3 (+https://github.com/kadircelebi/mrw-overlay-poe2)'})
    with urllib.request.urlopen(request, timeout=45) as response:
        raw = response.read()
        metadata = {'url': url, 'status': response.status,
                    'sha256': hashlib.sha256(raw).hexdigest(), 'bytes': len(raw),
                    'last_modified': response.headers.get('Last-Modified')}
    path.write_bytes(raw)
    return raw.decode('utf-8'), metadata


def write_json(path, data):
    path.write_text(json.dumps(data, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')


def plain_text(markup):
    markup = re.sub(r'<br\s*/?>', '\n', markup, flags=re.I)
    return html_module.unescape(re.sub(r'<[^>]+>', '', markup))


def normalize(data):
    result = []
    for pool in data['config']:
        rows = data.get(pool, [])
        levels = {}
        for row in rows:
            group = (str(row['ModGenerationTypeID']), row['ModFamilyList'][0])
            levels.setdefault(group, set()).add(int(row['Level']))
        levels = {group: sorted(values, reverse=True) for group, values in levels.items()}
        for row in rows:
            group = (str(row['ModGenerationTypeID']), row['ModFamilyList'][0])
            text = plain_text(row['str'])
            source_id = row.get('Code') or row.get('hover', '').rsplit('/', 1)[-1]
            id_origin = 'source code or hover identifier'
            if not source_id:
                source_id = 'local:' + hashlib.sha256(json.dumps(row, sort_keys=True).encode('utf-8')).hexdigest()
                id_origin = 'local content hash; source has no standalone modifier id'
            ranges = [{'min': float(low), 'max': float(high)} for low, high in
                      re.findall(r'\((-?\d+(?:\.\d+)?)[\u2014\u2013](-?\d+(?:\.\d+)?)\)', text)]
            result.append({
                'source_id': source_id, 'id_origin': id_origin,
                'pool': pool, 'affix': data['gen'].get(group[0], group[0]),
                'name': plain_text(row['Name']), 'families': row['ModFamilyList'],
                'tier': levels[group].index(int(row['Level'])) + 1,
                'tier_origin': 'derived with PoE2DB family/level ordering',
                'required_ilvl': int(row['Level']), 'weight': int(row.get('DropChance', 0)),
                'text': text, 'ranges': ranges,
                'tags': row.get('fossil_no', []), 'spawn_tags': row.get('spawn_no', []),
                'adds_tags': row.get('adds_no', []),
                'source_html': row['str'],
            })
    return {'base': data['baseitem'], 'options': data['opt'], 'mods': result}


def pool_totals(mods, minimum=1, maximum=100):
    # Mirror the site's minimum-level filter: retain the highest tier of a
    # family even when that family's highest level is below the minimum.
    rows = [r for r in mods if r['pool'] == 'normal']
    highest = {}
    for row in rows:
        key = (row['affix'], row['families'][0])
        highest[key] = max(highest.get(key, 0), row['required_ilvl'])
    allowed = [r for r in rows if r['required_ilvl'] <= maximum and
               (r['required_ilvl'] >= minimum or
                r['required_ilvl'] == highest[(r['affix'], r['families'][0])])]
    return {affix: {'count': sum(1 for r in allowed if r['affix'] == affix and r['weight'] > 0),
                    'weight': sum(r['weight'] for r in allowed if r['affix'] == affix)}
            for affix in ['Prefix', 'Suffix']}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--pages', nargs='+', default=['Gloves_str'])
    parser.add_argument('--output', type=Path, default=Path(__file__).parent / 'data')
    args = parser.parse_args()
    args.output.mkdir(parents=True, exist_ok=True)
    raw_dir = args.output / 'raw'
    raw_dir.mkdir(exist_ok=True)
    manifest = {'fetched_at_utc': datetime.now(timezone.utc).isoformat(),
                'scope': args.pages, 'sources': [], 'warnings': [
                    'Source data is not proof that every craft rule is complete or correct.',
                    'Shared source config can contain PoE1-only headings; empty pools must not be treated as supported.',
                    'Weights are community-derived, not authoritative game-file weights.',
                    'Game patch version has not been independently identified.',
                ]}
    javascript = None
    summary = {'pages': {}, 'validation': []}
    for index, page in enumerate(args.pages):
        if not re.fullmatch(r'[A-Za-z0-9_-]+', page):
            raise ValueError('Invalid page name')
        if index:
            time.sleep(3)
        html, provenance = download(f'https://poe2db.tw/us/{page}', raw_dir / f'{page}.html')
        manifest['sources'].append(provenance)
        marker = 'new ModsView('
        start = html.index(marker) + len(marker)
        data, consumed = json.JSONDecoder().raw_decode(html[start:])
        if not html[start + consumed:].lstrip().startswith(')'):
            raise ValueError('Unexpected ModsView invocation')
        write_json(args.output / f'{page}.source.json', data)
        normalized = normalize(data)
        write_json(args.output / f'{page}.mods.json', normalized)
        assert all(row['source_id'] and row['families'] and row['required_ilvl'] >= 0 and row['weight'] >= 0
                   for row in normalized['mods']), 'Malformed modifier row'
        summary['pages'][page] = {
            'mod_rows': len(normalized['mods']),
            'nonempty_pools': {key: len(data[key]) for key in data['config'] if data.get(key)},
            'normal_pool_by_minimum_level': {str(level): pool_totals(normalized['mods'], level)
                                            for level in [1, 50, 70]},
        }
        print(json.dumps({'page': page, **summary['pages'][page]}, ensure_ascii=False))
        if javascript is None:
            match = re.search(r'<script\s+src="(https://cdn\.poe2db\.tw/js/ModsView\.[^"]+\.js)"', html)
            if not match:
                raise ValueError('ModsView script not found')
            javascript, provenance = download(match[1], raw_dir / 'ModsView.js')
            manifest['sources'].append(provenance)
            field = re.search(r'\bcurrencies2\s*=\s*', javascript)
            if not field:
                raise ValueError('PoE2 currency definitions not found')
            rules = LiteralParser(javascript, field.end()).value()
            write_json(args.output / 'currency-rules.source.json', rules)
            summary['currency_definitions'] = len(rules)
            summary['currency_types'] = {kind: sum(r.get('type') == kind for r in rules.values())
                                         for kind in sorted({r.get('type') for r in rules.values()})}
            summary['source_disabled_rules'] = [key for key, value in rules.items() if value.get('class') == 'disabled']
            assert rules['transmute']['beforeRarity'] == ['Normal']
            assert rules['aug']['beforeRarity'] == ['Magic']
            assert rules['regal']['afterRarity'] == 'Rare'
            assert rules['exalted']['beforeRarity'] == ['Rare']
            summary['validation'].append('Source contains Normal -> Magic -> Rare rarity conditions and Rare-only Exalted.')
        if page == 'Gloves_str':
            # PoE2DB's own totals when the parser was written (2026-09). A patch
            # may change them legitimately, so a difference is reported, not
            # fatal; the parse is wrong only if the page looks broken too.
            expected = {1: (63700, 84500), 50: (18400, 31650), 70: (6200, 17450)}
            same = all((pool_totals(normalized['mods'], minimum)['Prefix']['weight'],
                        pool_totals(normalized['mods'], minimum)['Suffix']['weight']) == totals
                       for minimum, totals in expected.items())
            summary['validation'].append('Str gloves weight totals at minimum levels 1/50/70 '
                                         + ('match the September 2026 totals.' if same else
                                            'differ from the September 2026 totals (patch?).'))
    manifest_path = args.output / 'manifest.json'
    write_json(manifest_path, manifest)
    write_json(args.output / 'summary.json', summary)
    print(json.dumps({'currency_definitions': summary.get('currency_definitions'),
                      'currency_types': summary.get('currency_types'), 'validation': summary['validation']}))


if __name__ == '__main__':
    main()
