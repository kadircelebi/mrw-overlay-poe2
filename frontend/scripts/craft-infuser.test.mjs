import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { createItem, setRarity, applyCurrency, currencyReason } from '../public/craft/engine.mjs';
import { catalystRules } from '../public/craft/catalyst.mjs';
import { infuserRules, maxQuality, corruptChance } from '../public/craft/infuser.mjs';
import { setLang } from '../public/craft/i18n.mjs';
setLang('tr');
const json = async name => JSON.parse(await readFile(new URL('../public/craft/data/' + name, import.meta.url)));
const ring = await json('Rings.mods.json'), gloves = await json('Gloves_str.mods.json'), wands = await json('Wands.mods.json');
const rules = { ...catalystRules(), ...infuserRules() };
const breachEssence = { text: '+20% to Maximum Quality', affix: 'Prefix', families: ['x'], tags: [], values: [] };

test('maximum quality: 20, Breach Ring 40, Refined 45, Essence of the Breach +20', () => {
  assert.equal(maxQuality({ mods: [] }), 20);
  assert.equal(maxQuality({ baseName: 'Breach Ring', mods: [] }), 40);
  assert.equal(maxQuality({ baseName: 'Breach Ring', mods: [breachEssence] }), 60);
  assert.equal(maxQuality({ baseName: 'Refined Breach Ring', mods: [breachEssence] }), 65);
});

test('catalysts stop at the ring maximum; Infusers go 10 past it (75 on a Refined Breach Ring with the essence)', () => {
  let item = { ...setRarity(createItem('Rings'), 'Rare'), ilvl: 82, baseName: 'Refined Breach Ring', mods: [breachEssence] };
  for (let i = 0; i < 70; i++) if (!currencyReason(item, ring, 'flesh-catalyst', rules['flesh-catalyst'])) item = applyCurrency(item, ring, 'flesh-catalyst', rules['flesh-catalyst']);
  assert.equal(item.catalyst.quality, 65);
  const infuser = rules['vaal-catalysing-infuser'];
  for (let i = 0; i < 20; i++) if (!currencyReason(item, ring, 'vaal-catalysing-infuser', infuser)) item = applyCurrency(item, ring, 'vaal-catalysing-infuser', infuser, () => 0.99);
  assert.equal(item.catalyst.quality, 75);
  assert.equal(item.corrupted, undefined);
  assert.match(currencyReason(item, ring, 'vaal-catalysing-infuser', infuser), /75/);
});

test('an Infuser needs the maximum, fits only its classes, and corrupts more often the higher it goes', () => {
  const armour = rules['vaal-armourers-infuser'];
  const below = { ...setRarity(createItem(), 'Rare'), quality: 19 };
  assert.match(currencyReason(below, gloves, 'vaal-armourers-infuser', armour), /%20/);
  assert.match(currencyReason({ ...below, quality: 20 }, wands, 'vaal-armourers-infuser', armour), /Body Armour/);
  assert.equal(currencyReason({ ...below, quality: 20 }, gloves, 'vaal-armourers-infuser', armour), '');
  assert.equal(corruptChance(20, 20), 0);
  assert.equal(corruptChance(21, 20), 0.05);
  assert.ok(Math.abs(corruptChance(29, 20) - 0.45) < 1e-9);
  // At the maximum it never corrupts; above it, an unlucky roll does.
  const safe = applyCurrency({ ...below, quality: 20, ilvl: 70 }, gloves, 'vaal-armourers-infuser', armour, () => 0);
  assert.equal(safe.quality, 21);
  const risky = applyCurrency({ ...below, quality: 25 }, gloves, 'vaal-armourers-infuser', armour, () => 0.1);
  assert.equal(risky.corrupted, true);
  assert.equal(risky.quality, 25);
  assert.match(currencyReason(risky, gloves, 'vaal-armourers-infuser', armour), /[Cc]orrupt/);
});
