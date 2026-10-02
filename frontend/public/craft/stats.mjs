import { rolledText } from './engine.mjs';

// The numbers a base shows once its modifiers and quality are applied, the
// way the game computes them: (base + flat) × (1 + increased%) × (1 + quality%).
// Quality is its own multiplier, not one more "increased": a Sirenscale Gloves
// (54 ES) with +60 ES, 99% + 20% increased ES and 20% quality shows
// 114 × 2.19 × 1.2 = 299.6 → 300 in game (it would be 272 if added).
// Modifiers are local only on items with that defence or damage of their own,
// so an amulet's "+# to maximum Energy Shield" adds nothing here.

const flatDefence = [
  [/^\+?(-?\d+(?:\.\d+)?) to Armour$/, ['ar']],
  [/^\+?(-?\d+(?:\.\d+)?) to Evasion Rating$/, ['ev']],
  [/^\+?(-?\d+(?:\.\d+)?) to maximum Energy Shield$/, ['es']],
];
const increasedDefence = {
  'Armour': ['ar'], 'Evasion Rating': ['ev'], 'Energy Shield': ['es'],
  'Armour and Evasion': ['ar', 'ev'], 'Armour and Energy Shield': ['ar', 'es'],
  'Evasion and Energy Shield': ['ev', 'es'], 'Armour, Evasion and Energy Shield': ['ar', 'ev', 'es'],
};
const elements = ['Fire', 'Cold', 'Lightning', 'Chaos'];

// The item's own modifiers plus what its ordinary runes add
// ("20% increased Armour, Evasion and Energy Shield", kept from a copied item).
function lines(item) {
  return [...item.mods.flatMap(mod => rolledText(mod).split(/\n|<br\s*\/?\s*>/i)), ...(item.runeStats || [])]
    .map(l => l.trim()).filter(Boolean);
}

export function baseStats(base, item) {
  if (!base) return null;
  const quality = 1 + (item.quality || 0) / 100;
  const flat = { ar: 0, ev: 0, es: 0 }, inc = { ar: 0, ev: 0, es: 0 };
  let blockInc = 0, physInc = 0, speedInc = 0, critFlat = 0;
  const physAdd = [0, 0], added = {};
  for (const line of lines(item)) {
    let m;
    for (const [re, keys] of flatDefence) if ((m = re.exec(line))) for (const k of keys) flat[k] += Number(m[1]);
    if ((m = /^(-?\d+(?:\.\d+)?)% (increased|reduced) (.+)$/.exec(line))) {
      const value = Number(m[1]) * (m[2] === 'reduced' ? -1 : 1);
      for (const k of increasedDefence[m[3]] || []) inc[k] += value;
      if (m[3] === 'Block chance') blockInc += value;
      if (m[3] === 'Physical Damage') physInc += value;
      if (m[3] === 'Attack Speed') speedInc += value;
    }
    if ((m = /^\+(\d+(?:\.\d+)?)% to Critical Hit Chance$/.exec(line))) critFlat += Number(m[1]);
    if ((m = /^Adds (\d+) to (\d+) (\w+) Damage$/.exec(line))) {
      if (m[3] === 'Physical') { physAdd[0] += Number(m[1]); physAdd[1] += Number(m[2]); }
      else if (elements.includes(m[3])) {
        const sum = added[m[3]] ||= [0, 0];
        sum[0] += Number(m[1]); sum[1] += Number(m[2]);
      }
    }
  }
  const out = { quality: item.quality || 0, req: base.req || {} };
  for (const k of ['ar', 'ev', 'es']) {
    if (base[k]) out[k] = Math.round((base[k] + flat[k]) * (1 + inc[k] / 100) * quality);
  }
  if (base.block) out.block = Math.round(base.block * (1 + blockInc / 100));
  if (base.phys) {
    // Weapon quality is assumed to work like armour quality (a multiplier on
    // physical damage); not yet checked against an item copied in game.
    const scale = (1 + physInc / 100) * quality;
    out.phys = [Math.round((base.phys[0] + physAdd[0]) * scale), Math.round((base.phys[1] + physAdd[1]) * scale)];
    out.elemental = Object.entries(added).map(([kind, [min, max]]) => ({ kind, min, max }));
    out.aps = Math.round(base.aps * (1 + speedInc / 100) * 100) / 100;
    out.crit = Math.round((base.crit + critFlat) * 100) / 100;
    const avg = ([min, max]) => (min + max) / 2;
    out.pdps = Math.round(avg(out.phys) * out.aps * 10) / 10;
    out.edps = Math.round(out.elemental.filter(e => e.kind !== 'Chaos').reduce((s, e) => s + avg([e.min, e.max]), 0) * out.aps * 10) / 10;
    out.cdps = Math.round(out.elemental.filter(e => e.kind === 'Chaos').reduce((s, e) => s + avg([e.min, e.max]), 0) * out.aps * 10) / 10;
    out.dps = Math.round((out.pdps + out.edps + out.cdps) * 10) / 10;
  }
  return out;
}

/** The property lines a copied item prints for these numbers. */
export function propertyLines(stats) {
  if (!stats) return [];
  const lines = [];
  if (stats.quality) lines.push(`Quality: +${stats.quality}% (augmented)`);
  if (stats.block) lines.push(`Block chance: ${stats.block}%`);
  if (stats.ar) lines.push(`Armour: ${stats.ar}`);
  if (stats.ev) lines.push(`Evasion Rating: ${stats.ev}`);
  if (stats.es) lines.push(`Energy Shield: ${stats.es}`);
  if (stats.phys) {
    lines.push(`Physical Damage: ${stats.phys[0]}-${stats.phys[1]}`);
    for (const e of stats.elemental) lines.push(`${e.kind} Damage: ${e.min}-${e.max}`);
    lines.push(`Critical Hit Chance: ${stats.crit.toFixed(2)}%`, `Attacks per Second: ${stats.aps.toFixed(2)}`);
  }
  return lines;
}

export function requirementLine(req) {
  const parts = [];
  if (req.level) parts.push(`Level ${req.level}`);
  if (req.strength) parts.push(`${req.strength} Str`);
  if (req.dexterity) parts.push(`${req.dexterity} Dex`);
  if (req.intelligence) parts.push(`${req.intelligence} Int`);
  return parts.length ? `Requires: ${parts.join(', ')}` : '';
}
