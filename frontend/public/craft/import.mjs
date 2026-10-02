// Turns an item copied in game (the price check's Alt+E item) into a craft
// draft: item class, defence type, rarity, item level and its own prefixes
// and suffixes. Each affix is read from the advanced copy's header
// ({ Prefix Modifier "Hale" (Tier: 3) }), so merged lines and hybrid affixes
// come apart again, and matched to the class data by side, name and tier.

const header = /^\{\s*(?:(Desecrated|Crafted|Fractured)\s+)?(Prefix|Suffix|Implicit|Unique|Rune|Corruption\s+Enhancement|Enhancement)(?:\s+Modifier)?(?:\s+"([^"]+)")?(?:\s+\(Tier:\s*(\d+)\))?/;
const range = /\((-?\d+(?:\.\d+)?)-(-?\d+(?:\.\d+)?)\)/g;
const note = /\s+\((?:fractured|crafted|desecrated|augmented|implicit|rune|enchant)\)$/i;
const number = /-?\d+(?:\.\d+)?/g;
const pools = ['normal', 'desecrated', 'essence', 'perfect_essence', 'marksman', 'decay', 'berserking', 'chronomancy', 'soul', 'destruction'];

// "41(39-42)% increased Energy Shield" and "(39—42)% increased Energy
// Shield" both become "#% increased Energy Shield".
const shape = text => text.replace(range, '').replace(/\((-?\d+(?:\.\d+)?)[—–](-?\d+(?:\.\d+)?)\)/g, '#')
  .replace(number, '#').replace(/<br\s*\/?\s*>/gi, '\n').trim();

/** The prefix and suffix blocks of an advanced copy. */
export function affixBlocks(raw) {
  const blocks = [];
  let current = null;
  for (const line of String(raw).split(/\r?\n/).map(l => l.trim())) {
    const m = header.exec(line);
    if (m) {
      current = ['Prefix', 'Suffix'].includes(m[2])
        ? { side: m[2], kind: (m[1] || '').toLowerCase(), name: m[3] || '', tier: m[4] ? Number(m[4]) : 0, lines: [] }
        : null;
      if (current) blocks.push(current);
      continue;
    }
    if (!current) continue;
    if (!line || line.startsWith('---')) { current = null; continue; }
    current.lines.push(line.replace(note, ''));
  }
  return blocks;
}

function valuesOf(lines) {
  return lines.flatMap(line => (line.replace(range, '').match(number) || []).map(Number));
}

function match(block, data) {
  const rows = data.mods.filter(m => m.affix === block.side && pools.includes(m.pool));
  const text = block.lines.join('\n');
  const want = shape(text);
  const sameShape = m => shape(m.text) === want;
  const named = rows.filter(m => block.name && m.name === block.name);
  const preferred = block.kind === 'desecrated' ? ['desecrated', 'normal'] : block.kind === 'crafted' ? ['essence', 'perfect_essence'] : ['normal'];
  const byPool = list => [...list].sort((a, b) => {
    const ia = preferred.indexOf(a.pool), ib = preferred.indexOf(b.pool);
    return (ia < 0 ? 99 : ia) - (ib < 0 ? 99 : ib);
  });
  return byPool(named.filter(m => m.tier === block.tier && sameShape(m)))[0]
    || byPool(named.filter(sameShape))[0]
    || byPool(named.filter(m => m.tier === block.tier))[0]
    || byPool(rows.filter(sameShape))[0]
    || null;
}

function withValues(row, values) {
  const out = row.ranges.map((r, i) => {
    const v = values[i];
    if (!Number.isFinite(v)) return Number.isInteger(r.min) && Number.isInteger(r.max) ? Math.round((r.min + r.max) / 2) : (r.min + r.max) / 2;
    return Math.min(r.max, Math.max(r.min, v));
  });
  return { ...row, values: out };
}

/** The craft page and the reason it cannot be used, for a copied item. */
export function pageFor(item, classData) {
  if (String(item.rarity).toLowerCase() === 'unique') return { error: 'unique' };
  const cls = classData.classes.find(c => c.itemClass === item.class);
  if (!cls) return { error: 'class' };
  const page = classData.bases?.[item.baseType];
  if (page && cls.variants.some(v => v.page === page)) return { cls, page, guessed: false };
  return { cls, page: cls.variants[0].page, guessed: cls.variants.length > 1 };
}

/** The craft draft for a copied item, given its class's data. */
export function importItem(item, page, data) {
  const rarity = { normal: 'Normal', magic: 'Magic', rare: 'Rare' }[String(item.rarity).toLowerCase()] || 'Rare';
  const mods = [], unmatched = [];
  for (const block of affixBlocks(item.raw)) {
    const row = match(block, data);
    if (!row) { unmatched.push(block.name || block.lines.join(' / ')); continue; }
    const mod = withValues(row, valuesOf(block.lines));
    if (block.kind === 'desecrated') mod.desecrated = true;
    if (block.kind === 'fractured') mod.fractured = true;
    // The game marks crafted affixes; some share their wording with the base pool.
    if (block.kind === 'crafted') mod.crafted = true;
    mods.push(mod);
  }
  const ilvl = Number.isInteger(item.itemLevel) && item.itemLevel >= 1 && item.itemLevel <= 100 ? item.itemLevel : 81;
  return { item: { base: page, rarity, ilvl, mods }, unmatched };
}

/** What ordinary runes add to a copied item ("20% increased Armour… (rune)"), the craft runes left out. */
export function runeStatLines(raw, runes) {
  const craft = new Set(Object.values(runes || {}).map(r => r.text));
  return String(raw).split(/\r?\n/).map(l => l.trim()).filter(l => /\(rune\)$/i.test(l))
    .map(l => l.replace(/\s*\(rune\)$/i, '')).filter(l => !craft.has(l));
}

/** The craft runes in a copied item's sockets, by their line ("Can roll Marksman modifiers (rune)"). */
export function runesFor(raw, page, runes) {
  const lines = String(raw).split(/\r?\n/).map(l => l.trim().replace(/\s*\(rune\)$/i, ''));
  return Object.entries(runes || {}).filter(([, r]) => r.pages.includes(page) && lines.includes(r.text)).map(([id]) => id);
}
