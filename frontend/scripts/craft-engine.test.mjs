import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { createItem, setRarity, manualAdd, removeMod, clearMods, candidates, applyCurrency, currencyReason, chooseWeighted, count, rolledText, replaceTier, sortedMods } from '../public/craft/engine.mjs';
const data = JSON.parse(await readFile(new URL('../public/craft/data/Gloves_str.mods.json', import.meta.url)));
const rules = JSON.parse(await readFile(new URL('../public/craft/data/currency-rules.source.json', import.meta.url)));
const zero = () => 0;

test('Tier replacement works at full capacity and preserves other affixes and rarity', () => {
  let item = setRarity(createItem(), 'Rare');
  for (let i = 0; i < 6; i++) item = applyCurrency(item, data, 'exalted', rules.exalted, zero);
  const old = item.mods[0];
  const target = data.mods.find(m => m.pool === old.pool && m.affix === old.affix &&
    m.families[0] === old.families[0] && m.source_id !== old.source_id);
  const next = replaceTier(item, 0, target, zero);
  assert.equal(next.rarity, 'Rare'); assert.equal(next.mods.length, 6);
  assert.equal(next.mods[0].source_id, target.source_id);
  assert.deepEqual(next.mods[0].values, target.ranges.map(r => r.min));
  assert.deepEqual(next.mods.slice(1), item.mods.slice(1));
  assert.equal(item.mods[0].source_id, old.source_id);
  assert.throws(() => replaceTier({ ...item, ilvl: 1 }, 0, target), /Item Level/);
  assert.throws(() => replaceTier(item, 1, target), /ailesinden/);
});

test('Prefix-first display preserves underlying indices for roll edits and removal', () => {
  const p = data.mods.find(m => m.pool === 'normal' && m.affix === 'Prefix');
  const s = data.mods.find(m => m.pool === 'normal' && m.affix === 'Suffix');
  const item = manualAdd(manualAdd(createItem(), s, zero), p, zero);
  const sorted = sortedMods(item);
  assert.deepEqual(sorted.map(r => r.mod.affix), ['Prefix', 'Suffix']);
  assert.deepEqual(sorted.map(r => r.index), [1, 0]);
  assert.equal(removeMod(item, sorted[0].index).mods[0].affix, 'Suffix');
  assert.deepEqual(item.mods.map(m => m.affix), ['Suffix', 'Prefix']);
});

test('Rare survives removal of its last affix and accepts Exalted again', () => {
  let item = setRarity(createItem(), 'Rare');
  item = applyCurrency(item, data, 'exalted', rules.exalted, zero);
  assert.equal(item.mods.length, 1);
  item = removeMod(item, 0);
  assert.equal(item.rarity, 'Rare');
  assert.equal(currencyReason(item, data, 'exalted', rules.exalted), '');
  item = applyCurrency(item, data, 'exalted', rules.exalted, zero);
  assert.equal(clearMods(item).rarity, 'Rare');
});
test('Normal rejects Exalted; Transmutation, Augmentation, Regal follows rarity and slots', () => {
  let item = createItem();
  assert.throws(() => applyCurrency(item, data, 'exalted', rules.exalted), /Rare/);
  item = applyCurrency(item, data, 'transmute', rules.transmute, zero);
  assert.equal(item.rarity, 'Magic');
  item = applyCurrency(item, data, 'aug', rules.aug, zero);
  assert.equal(count(item, 'Prefix'), 1); assert.equal(count(item, 'Suffix'), 1);
  assert.throws(() => applyCurrency(item, data, 'aug', rules.aug), /boş/);
  item = applyCurrency(item, data, 'regal', rules.regal, zero);
  assert.equal(item.rarity, 'Rare'); assert.equal(item.mods.length, 3);
});
test('Repeated Exalted caps at 3 prefixes / 3 suffixes without family collisions', () => {
  let item = setRarity(createItem(), 'Rare');
  for (let i = 0; i < 6; i++) item = applyCurrency(item, data, 'exalted', rules.exalted, zero);
  assert.equal(count(item, 'Prefix'), 3); assert.equal(count(item, 'Suffix'), 3);
  assert.equal(new Set(item.mods.map(m => m.affix + m.families[0])).size, 6);
  assert.throws(() => applyCurrency(item, data, 'exalted', rules.exalted), /boş/);
  assert.throws(() => setRarity(item, 'Magic'), /Magic/);
});
test('Manual add promotes rarity and rejects same family and above-ilvl tiers', () => {
  const prefixes = data.mods.filter(m => m.pool === 'normal' && m.affix === 'Prefix');
  const first = prefixes[0];
  let item = manualAdd(createItem(), first, zero);
  assert.equal(item.rarity, 'Magic');
  assert.throws(() => manualAdd(item, first), /ailesi/);
  const second = prefixes.find(m => !m.families.includes(first.families[0]));
  item = manualAdd(item, second, zero);
  assert.equal(item.rarity, 'Rare');
  const high = prefixes.find(m => m.required_ilvl > 1);
  assert.throws(() => manualAdd({ ...createItem(), ilvl: 1 }, high), /Item Level/);
});
test('Minimum-level fallback matches all three supplied screenshot totals', () => {
  const item = { ...setRarity(createItem(), 'Rare'), ilvl: 100 };
  for (const [minimum, p, s] of [[1,63700,84500], [50,18400,31650], [70,6200,17450]]) {
    const rows = candidates(item, data, { minimum });
    assert.equal(rows.filter(m => m.affix === 'Prefix').reduce((sum,m) => sum+m.weight,0), p);
    assert.equal(rows.filter(m => m.affix === 'Suffix').reduce((sum,m) => sum+m.weight,0), s);
  }
});
test('Weighted selection follows weight boundaries; displayed rolls stay inside ranges', () => {
  const rows = [{ weight: 1, name: 'a' }, { weight: 3, name: 'b' }];
  assert.equal(chooseWeighted(rows, () => .249).name, 'a');
  assert.equal(chooseWeighted(rows, () => .25).name, 'b');
  assert.equal(chooseWeighted(rows, () => .999).name, 'b');
  const row = data.mods.find(m => m.pool === 'normal' && m.ranges.length);
  const item = manualAdd(createItem(), row, () => .999);
  assert.deepEqual(item.mods[0].values, row.ranges.map(r => r.max));
  assert.equal(rolledText(item.mods[0]).includes('—'), false);
});
test('Annulment retains rarity; Chaos replaces one; Divine retains identities', () => {
  let item = setRarity(createItem(), 'Rare');
  for (let i = 0; i < 4; i++) item = applyCurrency(item, data, 'exalted', rules.exalted, zero);
  const chaos = applyCurrency(item, data, 'chaos', rules.chaos, zero);
  assert.equal(chaos.mods.length, 4); assert.equal(chaos.rarity, 'Rare');
  const divine = applyCurrency(item, data, 'divine', rules.divine, () => .9);
  assert.deepEqual(divine.mods.map(m => m.source_id), item.mods.map(m => m.source_id));
  const annul = applyCurrency(item, data, 'annu', rules.annu, zero);
  assert.equal(annul.mods.length, 3); assert.equal(annul.rarity, 'Rare');
});
