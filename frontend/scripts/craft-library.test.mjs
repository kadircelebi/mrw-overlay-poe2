import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { createItem, setRarity, sideLimit, candidates, manualAdd, isRetired } from '../public/craft/engine.mjs';
import { parseLibrary, entryFor, sameCraft, addEntry, removeEntry, autoLimit, libraryLimit } from '../public/craft/library.mjs';
import { setLang } from '../public/craft/i18n.mjs';
setLang('en');
const json = async name => JSON.parse(await readFile(new URL('../public/craft/data/' + name, import.meta.url)));

const craft = (n = 1) => ({ item: { ...createItem('Wands'), mods: [] }, history: Array.from({ length: n }, (_, i) => ({ label: `step ${i}` })), sessionStart: '2026-10-03T00:00:00Z' });

test('a library entry keeps the whole craft and a summary', () => {
  const e = entryFor(craft(3), { name: '  My wand  ', baseName: 'Twisted Wand', costEx: 120 });
  assert.equal(e.name, 'My wand'); assert.equal(e.steps, 3); assert.equal(e.page, 'Wands');
  assert.equal(e.cost_ex, 120); assert.equal(e.auto, false);
  assert.ok(sameCraft(e, craft(3))); assert.ok(!sameCraft(e, craft(2)));
  // The entry is a copy: later changes to the craft do not leak in.
  const c = craft(1), copy = entryFor(c, { name: 'x' }); c.history.push({}); assert.equal(copy.state.history.length, 1);
});

test('automatic entries give way beyond the limit, saved-by-hand ones never do', () => {
  let list = [];
  list = addEntry(list, { ...entryFor(craft(), { name: 'kept' }), id: 'hand' });
  for (let i = 0; i < autoLimit + 5; i++) list = addEntry(list, { ...entryFor(craft(), { name: `auto ${i}`, auto: true }), id: `a${i}` });
  assert.equal(list.filter(e => e.auto).length, autoLimit);
  assert.ok(list.some(e => e.id === 'hand'));
  assert.equal(list[0].id, `a${autoLimit + 4}`);           // newest first
  assert.ok(!list.some(e => e.id === 'a0'));                 // oldest automatic dropped
  for (let i = 0; i < libraryLimit + 3; i++) list = addEntry(list, { ...entryFor(craft(), { name: `h${i}` }), id: `h${i}` });
  assert.equal(list.length, libraryLimit);
  assert.deepEqual(removeEntry([{ id: 'x' }, { id: 'y' }], 'x'), [{ id: 'y' }]);
});

test('a damaged or foreign library reads as the valid entries only', () => {
  assert.deepEqual(parseLibrary('not json'), []);
  assert.deepEqual(parseLibrary('{"a":1}'), []);
  const good = entryFor(craft(), { name: 'ok' });
  assert.equal(parseLibrary(JSON.stringify([good, { id: 'bad' }, null])).length, 1);
});

test('bases that move the affix limits change a Rare\'s room', async () => {
  const bases = await json('bases.json'), rings = await json('Rings.mods.json'), amulets = await json('Amulets.mods.json');
  const slots = (page, name) => bases.pages[page].find(b => b.name === name).slots;
  assert.deepEqual(slots('Amulets', 'Absent Amulet'), [-1, -1]);
  assert.deepEqual(slots('Amulets', 'Lament Amulet'), [-1, 0]);
  assert.deepEqual(slots('Amulets', 'Portent Amulet'), [0, -1]);
  assert.deepEqual(slots('Rings', 'Penumbra Ring'), [2, -2]);
  const absent = setRarity({ ...createItem('Amulets'), ilvl: 82, baseSlots: [-1, -1] }, 'Rare');
  assert.equal(sideLimit(absent, 'Prefix'), 2); assert.equal(sideLimit(absent, 'Suffix'), 2);
  assert.equal(sideLimit(absent, 'Prefix', 'Magic'), 1); // only the Rare limits move
  const penumbra = setRarity({ ...createItem('Rings'), ilvl: 82, baseSlots: [2, -2] }, 'Rare');
  assert.equal(sideLimit(penumbra, 'Prefix'), 5); assert.equal(sideLimit(penumbra, 'Suffix'), 1);
  // With its one suffix taken, nothing more on that side can roll.
  const suffix = rings.mods.find(m => m.pool === 'normal' && m.affix === 'Suffix' && m.required_ilvl <= 82);
  const full = manualAdd(penumbra, suffix, () => 0);
  assert.ok(!candidates(full, rings).some(m => m.affix === 'Suffix'));
  assert.ok(candidates(full, rings).some(m => m.affix === 'Prefix'));
  assert.ok(amulets.mods.length > 0);
});

test('a modifier the data no longer has stays on a saved craft, marked, and never rolls again', async () => {
  const data = await json('Wands.mods.json');
  const row = data.mods.find(m => m.pool === 'normal' && m.affix === 'Prefix');
  const kept = { ...row, values: row.ranges.map(r => r.min) };
  assert.equal(isRetired(kept, data), false);
  // The same row under a new stand-in id (PoE2DB hashes rows without an id) is still the same modifier.
  assert.equal(isRetired({ ...kept, source_id: 'local:changed' }, data), false);
  const gone = { ...kept, source_id: 'RemovedByPatch1', name: 'of the Removed', families: ['RemovedFamily'] };
  assert.equal(isRetired(gone, data), true);
  assert.equal(isRetired({ ...gone, unrevealed: true }, data), false);
  const item = { ...setRarity(createItem('Wands'), 'Rare'), mods: [gone] };
  const entry = entryFor({ item, history: [], sessionStart: '' }, { name: 'old' });
  assert.equal(parseLibrary(JSON.stringify([entry]))[0].state.item.mods[0].source_id, 'RemovedByPatch1');
  assert.ok(!candidates(item, data).some(m => m.source_id === 'RemovedByPatch1'));
});

test('a modifier is legacy only when its family and stat are gone, not when a name, level or roll changed', async () => {
  const data = await json('Wands.mods.json');
  const row = data.mods.find(m => m.pool === 'normal' && m.affix === 'Prefix' && m.ranges.length);
  const saved = { ...row, values: row.ranges.map(r => r.min) };
  // PoE2DB renamed it, a patch moved its level and changed its roll: still the same modifier.
  assert.equal(isRetired({ ...saved, source_id: 'x', name: 'of Renaming', required_ilvl: row.required_ilvl + 7,
    text: row.text.replace(/\d+/g, n => String(Number(n) + 3)) }, data), false);
  // Same family, another stat: the old stat is gone.
  assert.equal(isRetired({ ...saved, source_id: 'x', name: 'of Renaming', text: '+(1—2) to Something Removed' }, data), true);
});
