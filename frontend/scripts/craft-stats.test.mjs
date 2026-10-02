import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { baseStats } from '../public/craft/stats.mjs';
import { craftText } from '../public/craft/trade.mjs';

const bases = JSON.parse(readFileSync(new URL('../public/craft/data/bases.json', import.meta.url), 'utf8')).pages;
const sirenscale = bases.Gloves_int.find(b => b.name === 'Sirenscale Gloves');
const mod = (affix, text, values) => ({ affix, tier: 1, pool: 'normal', text, values });

// A Hate Palm copied in game: 54 base ES, +60 ES, 99% + 20% (rune) increased,
// 20% quality → "Energy Shield: 300". Quality is a separate multiplier.
test('defences match an item copied in game', () => {
  assert.equal(sirenscale.es, 54);
  const item = { quality: 20, mods: [
    mod('Prefix', '(92—100)% increased Energy Shield', [99]),
    mod('Prefix', '+(48—60) to maximum Energy Shield', [60]),
    mod('Prefix', '(20—20)% increased Armour, Evasion and Energy Shield', [20]),
  ] };
  assert.equal(baseStats(sirenscale, item).es, 300);
  assert.equal(baseStats(sirenscale, { quality: 20, mods: [] }).es, 65);
  assert.equal(baseStats(sirenscale, { quality: 0, mods: [] }).es, 54);
});

test('a hybrid line and global stats on jewellery', () => {
  const hybrid = { quality: 0, mods: [mod('Prefix', '(39—42)% increased Energy Shield\n+(42—49) to maximum Life', [40, 45])] };
  assert.equal(baseStats(sirenscale, hybrid).es, Math.round(54 * 1.4));
  const ring = bases.Rings.at(-1);
  const stats = baseStats(ring, { quality: 0, mods: [mod('Prefix', '+(48—60) to maximum Energy Shield', [60])] });
  assert.equal(stats.es, undefined);
});

test('weapon damage, speed and crit', () => {
  const bow = { name: 'Test Bow', phys: [10, 20], aps: 1.2, crit: 5, req: {} };
  const stats = baseStats(bow, { quality: 0, mods: [
    mod('Prefix', 'Adds (5—6) to (10—12) Physical Damage', [5, 10]),
    mod('Prefix', '(100—110)% increased Physical Damage', [100]),
    mod('Prefix', 'Adds (1—2) to (20—30) Fire Damage', [2, 20]),
    mod('Suffix', '(10—12)% increased Attack Speed', [10]),
    mod('Suffix', '+(1—2)% to Critical Hit Chance', [1.5]),
  ] });
  assert.deepEqual(stats.phys, [30, 60]);
  assert.equal(stats.aps, 1.32);
  assert.equal(stats.crit, 6.5);
  assert.equal(stats.pdps, 59.4);
  assert.equal(stats.edps, 14.5);
});

test('market text names the base and prints its numbers', () => {
  const text = craftText({ rarity: 'Rare', ilvl: 82, quality: 20, mods: [mod('Prefix', '+(48—60) to maximum Energy Shield', [60])] }, 'Gloves', sirenscale);
  assert.match(text, /Theoretical Craft\nSirenscale Gloves\n/);
  assert.match(text, /Quality: \+20% \(augmented\)\nEnergy Shield: 137\n/);
  assert.match(craftText({ rarity: 'Rare', ilvl: 82, mods: [] }, 'Gloves'), /Theoretical Craft\nGloves\n--------\nItem Level: 82/);
});

test('market text lists the sockets', () => {
  const text = craftText({ rarity: 'Rare', ilvl: 82, quality: 0, sockets: 2, mods: [] }, 'Gloves', sirenscale);
  assert.match(text, /\nSockets: S S\n--------\nItem Level: 82/);
});
