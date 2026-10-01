import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { affixBlocks, pageFor, importItem } from '../public/craft/import.mjs';

const json = async name => JSON.parse(await readFile(new URL('../public/craft/data/' + name, import.meta.url)));
const classData = await json('classes.json');

const helmet = {
  class: 'Helmets', rarity: 'rare', baseType: 'Kamasan Tiara', itemLevel: 81,
  raw: `Item Class: Helmets
Rarity: Rare
Rapture Salvation
Kamasan Tiara
-------------
Energy Shield: 503 (augmented)
## Item Level: 81
{ Prefix Modifier "Pope's" (Tier: 1) — Life, Energy Shield }
41(39-42)% increased Energy Shield
+44(42-49) to maximum Life
{ Suffix Modifier "of the Ice" (Tier: 2) — Elemental, Cold, Resistance }
+39(36-40)% to Cold Resistance`,
};

test('advanced copy headers split into affix blocks', () => {
  const blocks = affixBlocks(helmet.raw);
  assert.deepEqual(blocks.map(b => [b.side, b.name, b.tier, b.lines.length]),
    [['Prefix', "Pope's", 1, 2], ['Suffix', 'of the Ice', 2, 1]]);
});

test('a copied helmet becomes an Energy Shield helmet draft with its rolls', async () => {
  const where = pageFor(helmet, classData);
  assert.equal(where.page, 'Helmets_int');
  assert.equal(where.guessed, false);
  const { item, unmatched } = importItem(helmet, where.page, await json('Helmets_int.mods.json'));
  assert.deepEqual(unmatched, []);
  assert.equal(item.rarity, 'Rare'); assert.equal(item.ilvl, 81); assert.equal(item.mods.length, 2);
  const hybrid = item.mods.find(m => m.name === "Pope's");
  assert.equal(hybrid.affix, 'Prefix'); assert.equal(hybrid.tier, 1); assert.deepEqual(hybrid.values, [41, 44]);
  const cold = item.mods.find(m => m.name === 'of the Ice');
  assert.equal(cold.tier, 2); assert.deepEqual(cold.values, [39]);
});

test('desecrated affixes keep their mark; unique and unknown classes are refused', async () => {
  const amulet = {
    class: 'Amulets', rarity: 'rare', baseType: 'Absent Amulet', itemLevel: 82,
    raw: `{ Fractured Prefix Modifier "Countess'" (Tier: 1) }
+50(47-50) to Spirit (fractured)
{ Desecrated Suffix Modifier "of Kurgal" (Tier: 1) }
+6(3-5)% to Quality of all Skills`,
  };
  const where = pageFor(amulet, classData);
  assert.equal(where.page, 'Amulets');
  const { item } = importItem(amulet, where.page, await json('Amulets.mods.json'));
  const spirit = item.mods.find(m => m.name === "Countess'");
  assert.ok(spirit, 'fractured affix is imported'); assert.deepEqual(spirit.values, [50]);
  const kurgal = item.mods.find(m => m.name === 'of Kurgal');
  // "of Kurgal" names many desecrated suffixes; the line's wording picks the one.
  assert.equal(kurgal.text, '+(3—5)% to Quality of all Skills'); assert.equal(kurgal.desecrated, true);
  assert.deepEqual(kurgal.values, [5], 'a roll above the range is clamped');
  assert.equal(pageFor({ ...amulet, rarity: 'unique' }, classData).error, 'unique');
  assert.equal(pageFor({ ...amulet, class: 'Jewels' }, classData).error, 'class');
});

test('an unknown base still opens the class, flagged as a guess', () => {
  const where = pageFor({ class: 'Gloves', rarity: 'magic', baseType: 'Nonexistent Mitts' }, classData);
  assert.equal(where.page, 'Gloves_str'); assert.equal(where.guessed, true);
});
