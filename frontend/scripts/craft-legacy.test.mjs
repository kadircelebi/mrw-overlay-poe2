import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { createItem, setRarity, applyCurrency, currencyReason, isRetired } from '../public/craft/engine.mjs';
import { applicable, specialReason, applySpecial, revealChoice } from '../public/craft/special.mjs';
import { corrupt } from '../public/craft/corrupt.mjs';
import { setLang } from '../public/craft/i18n.mjs';
setLang('en');

const json = async name => JSON.parse(await readFile(new URL('../public/craft/data/' + name, import.meta.url)));
const rules = await json('currency-rules.source.json');
const special = (await json('special-currencies.json')).rules;

// Seeded, so a failure repeats.
function rng(seed) {
  return () => {
    seed = (seed + 0x6D2B79F5) | 0;
    let t = Math.imul(seed ^ (seed >>> 15), 1 | seed);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

// A modifier a patch took out: a real row under an id, name and family the
// data no longer has.
function legacy(data, side, n) {
  const row = data.mods.find(m => m.pool === 'normal' && m.affix === side && m.ranges.length);
  return { ...row, source_id: `Retired${side}${n}`, name: `of Retirement ${n}`, families: [`RetiredFamily${side}${n}`],
    values: row.ranges.map(r => r.min) };
}

// The orbs the craft window applies through applyCurrency (Vaal goes through
// corrupt, catalysts and infusers have their own steps).
const orb = /^(transmute|aug|regal|exalted|chaos|annu|divine|fracturing-orb|(?:greater|perfect)-(?:orb-of-transmutation|orb-of-augmentation|regal-orb|exalted-orb|chaos-orb))$/;
const basics = Object.entries(rules).filter(([id]) => orb.test(id));
const specials = Object.entries(special);

for (const [page, classId] of [['Gloves_str', 'gloves'], ['Wands', 'wand'], ['Amulets', 'amulet'], ['Body_Armours_str_int', 'body']]) {
  test(`${page}: a legacy modifier survives every orb, never comes back once gone`, async () => {
    const data = await json(`${page}.mods.json`);
    const random = rng(page.length * 7919);
    const seen = { steps: 0, removed: 0, divined: 0, kept: 0, specials: 0, corrupted: 0 };
    for (let run = 0; run < 150; run++) {
      const old = [legacy(data, 'Prefix', run), legacy(data, 'Suffix', run)];
      let item = { ...setRarity(createItem(page), 'Rare'), ilvl: 82, mods: [...old] };
      const gone = new Set();
      for (let step = 0; step < 30 && !item.corrupted; step++) {
        const before = item;
        const roll = random();
        let label;
        if (roll < 0.04) {
          item = corrupt(item, data, { classId }, random).item;
          label = 'vaal';
          seen.corrupted++;
        } else if (roll < 0.25) {
          const usable = specials.filter(([, r]) => applicable(r, data) && !specialReason(item, data, r));
          if (!usable.length) continue;
          const [, rule] = usable[Math.floor(random() * usable.length)];
          item = applySpecial(item, data, rule, random);
          if (item.reveal) item = revealChoice(item, Math.floor(random() * item.reveal.choices.length));
          label = rule.name;
          seen.specials++;
        } else {
          const usable = basics.filter(([key, r]) => !currencyReason(item, data, key, r));
          if (!usable.length) break;
          const [key, rule] = usable[Math.floor(random() * usable.length)];
          item = applyCurrency(item, data, key, rule, random);
          label = key;
          if (rule.afterTrigger === 'divine' && before.mods.some(m => old.includes(m) || m.source_id.startsWith('Retired'))) seen.divined++;
        }
        seen.steps++;
        const ids = new Set(item.mods.map(m => m.source_id));
        for (const m of before.mods) if (m.source_id.startsWith('Retired') && !ids.has(m.source_id)) { gone.add(m.source_id); seen.removed++; }
        for (const m of item.mods) {
          if (!m.source_id.startsWith('Retired')) {
            assert.equal(isRetired(m, data), false, `${label} rolled a modifier the data does not have`);
            continue;
          }
          assert.ok(!gone.has(m.source_id), `${label} brought ${m.source_id} back`);
          assert.equal(isRetired(m, data), true);
          // Still a whole modifier: its own range and a number for each roll
          // (Vaal's scaling may take a roll past its range, as in the game).
          assert.ok(m.ranges.length && m.values.length === m.ranges.length && m.values.every(Number.isFinite), `${label} broke ${m.source_id}`);
        }
      }
      seen.kept += item.mods.filter(m => m.source_id.startsWith('Retired')).length;
    }
    // The run went through every path: removal, Divine on it, keeping it, specials, Vaal.
    assert.ok(seen.removed > 50 && seen.divined > 20 && seen.kept > 10 && seen.specials > 100 && seen.corrupted > 20, JSON.stringify(seen));
  });
}
