import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { createItem, setRarity, manualAdd, currencyReason } from '../public/craft/engine.mjs';
import { omenDefinitions, omenEffects, applyOrbOmens, orbOmenReason, relevantOmens } from '../public/craft/omens.mjs';
import { multiplier, scaleValues, corruptOutcomes, corrupt, sanctify, sanctifyReason, enchantRows } from '../public/craft/corrupt.mjs';
import { craftText } from '../public/craft/trade.mjs';
import { augmentedMod } from '../public/craft/catalyst.mjs';
import { rolledText } from '../public/craft/engine.mjs';
import { setLang } from '../public/craft/i18n.mjs';
setLang('en');
const json = async name => JSON.parse(await readFile(new URL('../public/craft/data/' + name, import.meta.url)));
const wands = await json('Wands.mods.json'), gloves = await json('Gloves_str.mods.json'), rings = await json('Rings.mods.json');
const amulets = await json('Amulets.mods.json');
const rules = await json('currency-rules.source.json'), omens = omenDefinitions(rules);
const row = (data, text) => data.mods.find(m => m.pool === 'normal' && m.text === text);
const rare = (data, texts) => texts.reduce((item, text) => manualAdd(item, row(data, text), () => 0), setRarity({ ...createItem(data.page || 'Wands'), ilvl: 82 }, 'Rare'));
const seeded = seed => () => { seed = seed + 0x6D2B79F5 | 0; let t = Math.imul(seed ^ seed >>> 15, 1 | seed); t = t + Math.imul(t ^ t >>> 7, 61 | t) ^ t; return ((t ^ t >>> 14) >>> 0) / 4294967296; };

test('the multiplier runs from 0.78 to 1.22 in 0.01 steps', () => {
  assert.equal(multiplier(() => 0), 0.78);
  assert.equal(multiplier(() => 0.99999), 1.22);
  const seen = new Set(); const r = seeded(1);
  for (let i = 0; i < 5000; i++) seen.add(multiplier(r));
  assert.equal(seen.size, 45);
});

test('scaled values round to the nearest (corruption) or up (Sanctify), decimals keep two digits', () => {
  const mod = { values: [100, 1.5], ranges: [{ min: 90, max: 100 }, { min: 1.2, max: 1.6 }] };
  assert.deepEqual(scaleValues(mod, 1.22).values, [122, 1.83]);
  assert.deepEqual(scaleValues({ values: [33], ranges: [{ min: 30, max: 40 }] }, 0.79).values, [26]);       // 26.07 → 26
  assert.deepEqual(scaleValues({ values: [33], ranges: [{ min: 30, max: 40 }] }, 0.79, true).values, [27]); // up
});

test('fixed skill levels use Sanctify and corruption rounding without scaling reference numbers', () => {
  const item = rare(amulets, ['+3 to Level of all Spell Skills']);
  const spell = item.mods[0];
  assert.equal(rolledText(scaleValues(spell, 1.22, true)), '+4 to Level of all Spell Skills');
  assert.equal(rolledText(scaleValues(spell, 0.78, true)), '+3 to Level of all Spell Skills');
  assert.equal(rolledText(scaleValues(spell, 1, true)), '+3 to Level of all Spell Skills');
  assert.equal(rolledText(scaleValues(spell, 1.01, true)), '+4 to Level of all Spell Skills');
  assert.equal(rolledText(scaleValues(spell, 0.78)), '+2 to Level of all Spell Skills');
  assert.equal(rolledText(scaleValues(spell, 1.22)), '+4 to Level of all Spell Skills');
  assert.equal(rolledText(spell), '+3 to Level of all Spell Skills');
  const reference = { text: '20% chance for Skills to retain 40% of Glory on use', ranges: [], values: [] };
  assert.equal(scaleValues(reference, 1.22, true).text, reference.text);
});

test('Sanctification and 40% caster quality can produce +5 Spell Skills in the display and price query', () => {
  const item = { ...rare(amulets, ['+3 to Level of all Spell Skills']), catalyst: { id: 'sibilant-catalyst', quality: 40 } };
  const original = structuredClone(item);
  const effects = omenEffects(omens, ['omen-of-sanctification'], 'divine', rules.divine, amulets);
  const good = applyOrbOmens(item, amulets, 'divine', rules.divine, effects, () => 0.99999);
  assert.equal(good.sanctified, true);
  assert.equal(rolledText(good.mods[0]), '+4 to Level of all Spell Skills');
  assert.equal(rolledText(augmentedMod(good.mods[0], good)), '+5 to Level of all Spell Skills');
  assert.match(craftText(good, 'Amulets'), /\+5 to Level of all Spell Skills/);
  const poor = sanctify(item, () => 0);
  assert.equal(rolledText(augmentedMod(poor.mods[0], poor)), '+4 to Level of all Spell Skills');
  assert.deepEqual(item, original);
  assert.equal(rolledText(augmentedMod(item.mods[0], item)), '+4 to Level of all Spell Skills');
});

test('Sanctification leaves fractured fixed skill levels intact', () => {
  const item = rare(amulets, ['+3 to Level of all Spell Skills', '(25—28)% increased Cast Speed']);
  item.mods[0] = { ...item.mods[0], fractured: true };
  const next = sanctify(item, () => 0.99999);
  assert.deepEqual(next.mods[0], item.mods[0]);
  assert.notDeepEqual(next.mods[1].values, item.mods[1].values);
});

test('Vaal outcomes depend on the item class', () => {
  const wand = rare(wands, ['+(17—20) to Intelligence']);
  assert.deepEqual(corruptOutcomes(wand, wands, { classId: 'wand', quality: 20 }), ['nothing', 'reroll', 'scale', 'quality', 'enchant']);
  assert.deepEqual(corruptOutcomes(wand, wands, { classId: 'wand', quality: 23 }), ['nothing', 'reroll', 'scale', 'enchant']);
  const glove = { ...rare(gloves, []), base: 'Gloves_str' };
  assert.deepEqual(corruptOutcomes(glove, gloves, { classId: 'gloves', sockets: 1, maxSockets: 2 }), ['nothing', 'socket', 'enchant']);
  // Corruption ignores the limit: a full item still gains a socket (3 → 4 on an exceptional bow).
  assert.ok(corruptOutcomes(glove, gloves, { classId: 'gloves', sockets: 2, maxSockets: 2 }).includes('socket'));
  const bow = corrupt({ ...rare(wands, []), base: 'Bows' }, wands, { classId: 'bow', sockets: 3, maxSockets: 3 }, () => 0.5);
  assert.equal(bow.outcome, 'socket'); assert.equal(bow.item.sockets, 4);
  const ring = rare(rings, []);
  assert.ok(!corruptOutcomes(ring, rings, { classId: 'ring', maxSockets: 0 }).some(o => o === 'socket' || o === 'quality'));
  assert.ok(!corruptOutcomes({ ...wand, enchant: enchantRows(wands)[0] }, wands, { classId: 'wand' }).includes('enchant'));
});

test('a Vaal Orb corrupts the item, each outcome about equally often, and locks it', () => {
  const item = rare(wands, ['+(17—20) to Intelligence', '+1 to Level of all Fire Spell Skills', '(55—64)% increased Spell Damage']);
  item.mods[0] = { ...item.mods[0], fractured: true };
  const counts = {}, r = seeded(7);
  for (let i = 0; i < 5000; i++) {
    const result = corrupt(item, wands, { classId: 'wand', quality: 20 }, r);
    counts[result.outcome] = (counts[result.outcome] || 0) + 1;
    assert.equal(result.item.corrupted, true);
    assert.equal(result.item.mods.length, item.mods.length);
    assert.deepEqual(result.item.mods.find(m => m.fractured), item.mods[0]); // fractured keeps place and values
    if (result.outcome === 'enchant') assert.equal(result.item.enchant.pool, 'corrupted');
    if (result.outcome === 'quality') assert.ok(result.item.quality > 20 && result.item.quality <= 23);
  }
  for (const o of ['nothing', 'reroll', 'scale', 'quality', 'enchant']) assert.ok(Math.abs(counts[o] - 1000) < 120, `${o}: ${counts[o]}`);
  const locked = corrupt(item, wands, { classId: 'wand' }, () => 0).item;
  assert.match(currencyReason(locked, wands, 'chaos', rules.chaos), /Corrupted/);
  assert.throws(() => corrupt(locked, wands, { classId: 'wand' }), /Corrupted/);
  assert.match(craftText(locked, 'Wands'), /\nCorrupted$/);
});

test('Omen of Sanctification turns a Divine Orb on a Rare into Sanctify', () => {
  const item = rare(wands, ['+(17—20) to Intelligence', '(55—64)% increased Spell Damage']);
  assert.ok(relevantOmens(omens, 'divine', rules.divine, wands).some(([id]) => id === 'omen-of-sanctification'));
  const e = omenEffects(omens, ['omen-of-sanctification'], 'divine', rules.divine, wands);
  assert.equal(e.sanctify, true);
  assert.equal(orbOmenReason(item, wands, 'divine', rules.divine, e), '');
  const next = applyOrbOmens(item, wands, 'divine', rules.divine, e, () => 0.99999); // ×1.22
  assert.equal(next.sanctified, true);
  next.mods.forEach((m, i) => assert.deepEqual(m.values, item.mods[i].values.map(v => Math.ceil(v * 1.22 - 1e-9))));
  assert.match(currencyReason(next, wands, 'exalted', rules.exalted), /Sanctified/);
  assert.match(sanctifyReason({ ...item, rarity: 'Magic' }), /Rare/);
  assert.match(craftText(next, 'Wands'), /\nSanctified$/);
  assert.throws(() => sanctify(next), /Sanctified/);
});
