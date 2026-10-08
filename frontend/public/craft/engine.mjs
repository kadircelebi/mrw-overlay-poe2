import { t } from './i18n.mjs';
import { isCatalyst, catalystReason, applyCatalyst } from './catalyst.mjs';
import { isInfuser, infuseReason, applyInfuser } from './infuser.mjs';
// The craft state is independent of the UI. Rarity is never inferred on removal.
export const limits = { Normal: 0, Magic: 1, Rare: 3 };
export const supported = id => isCatalyst(id) || isInfuser(id) || /^(transmute|aug|regal|exalted|chaos|annu|divine|fracturing-orb|vaal-orb|(?:greater|perfect)-(?:orb-of-transmutation|orb-of-augmentation|regal-orb|exalted-orb|chaos-orb))$/.test(id);
export const createItem = (base = 'Gloves_str') => ({ base, rarity: 'Normal', ilvl: 81, mods: [] });
export const count = (item, side) => item.mods.filter(m => m.affix === side).length;
export const overlaps = (a, b) => a.affix === b.affix && a.families.some(f => b.families.includes(f));
// A socketed rune ("Can roll Marksman modifiers") adds its pool to every
// random roll; its modifiers then compete with the base pool by weight.
// item.runePools is set from the item's sockets; item.runePool is the single
// rune of drafts saved before sockets.
export const rollPools = item => ['normal', ...(item.runePools || (item.runePool ? [item.runePool] : []))];
// Serle's Triumph allows one more suffix (item.suffixBonus); Astrid's
// Creativity one more crafted (essence) modifier (item.craftedLimit). Some
// ring and amulet bases move the Rare limits (item.baseSlots, [prefix,
// suffix]: Dusk Ring +1/-1, Absent Amulet -1/-1).
export const sideLimit = (item, side, rarity = item.rarity) => Math.max(0,
  limits[rarity] + (side === 'Suffix' && rarity !== 'Normal' ? item.suffixBonus || 0 : 0) +
  (rarity === 'Rare' ? (item.baseSlots?.[side === 'Prefix' ? 0 : 1] || 0) + item.mods.filter(m => m.allows === side).length : 0));
export const isFull = (item, rarity = 'Rare') => ['Prefix', 'Suffix'].every(side => count(item, side) >= sideLimit(item, side, rarity));
export const isCrafted = mod => mod.crafted || mod.pool === 'essence' || mod.pool === 'perfect_essence' || mod.pool === 'liquid';

// Jewels: Potent Liquid Contempt's crafted modifier ("+1 Suffix Modifier
// allowed", itself a prefix) lets one side hold a third affix. Remove the
// crafted modifier once that side is full and the side is over its limit: a
// Chaos Orb never removes from such a side, since nothing could go back there.
export const contemptSide = item => item.mods.find(m => m.allows)?.allows || null;
export const overfull = (item, side) => count(item, side) > sideLimit(item, side);
export const craftedLimit = item => item.craftedLimit || 1;
export const isDesecrated = mod => Boolean(mod.desecrated || mod.pool === 'desecrated');

// removable lists the indexes a random removal may hit: never a fractured
// modifier, and only what the omens allow (one side, only Desecrated, only
// the lowest modifier level).
export function removable(item, { side = null, desecrated = false, lowest = false, skipOverfull = false } = {}) {
  let list = item.mods.map((mod, index) => ({ mod, index })).filter(({ mod }) => !mod.fractured &&
    (!side || mod.affix === side) && (!desecrated || isDesecrated(mod)) && (!skipOverfull || !overfull(item, mod.affix)));
  // An unrevealed Desecrated modifier has no level yet, so it is never "the
  // lowest".
  if (lowest) list = list.filter(({ mod }) => !mod.unrevealed);
  if (lowest && list.length) {
    const level = Math.min(...list.map(({ mod }) => mod.required_ilvl));
    list = list.filter(({ mod }) => mod.required_ilvl === level);
  }
  return list.map(({ index }) => index);
}
function removalReason(item, removal) {
  if (!item.mods.length) return t('err.noMods');
  const found = removable(item, removal).length;
  if (found >= (removal.count || 1)) return '';
  if (removal.desecrated) return t('err.noDesecratedMod');
  if (removal.side) return t('err.noSideMod', removal.side);
  return item.mods.some(m => m.fractured) ? t('err.onlyFractured') : t('err.noMods');
}
function removeRandom(item, removal, random) {
  let next = item;
  for (let i = 0; i < (removal.count || 1); i++) {
    const options = removable(next, removal);
    next = removeMod(next, options[Math.floor(random() * options.length)]);
  }
  return next;
}

export function rarityReason(item, rarity) {
  if (!(rarity in limits)) return t('err.badRarity');
  if (['Prefix', 'Suffix'].some(side => count(item, side) > sideLimit(item, side, rarity))) {
    return rarity === 'Normal' ? t('err.normalNeedsEmpty') : t('err.magicLimit');
  }
  return '';
}

export function setRarity(item, rarity) {
  const reason = rarityReason(item, rarity);
  if (reason) throw new Error(reason);
  return { ...item, rarity };
}

export function candidates(item, data, { pool = rollPools(item), minimum = 1, rarity = item.rarity } = {}) {
  const pools = Array.isArray(pool) ? pool : [pool];
  const rows = data.mods.filter(m => pools.includes(m.pool) && ['Prefix', 'Suffix'].includes(m.affix) && m.weight > 0);
  const highest = new Map();
  for (const row of rows) {
    const key = row.affix + ':' + row.families[0];
    highest.set(key, Math.max(highest.get(key) || 0, row.required_ilvl));
  }
  return rows.filter(m => m.required_ilvl <= item.ilvl &&
    (m.required_ilvl >= minimum || m.required_ilvl === highest.get(m.affix + ':' + m.families[0])) &&
    count(item, m.affix) < sideLimit(item, m.affix, rarity) && !item.mods.some(existing => overlaps(existing, m)));
}

export function chooseWeighted(rows, random = Math.random) {
  const total = rows.reduce((sum, row) => sum + row.weight, 0);
  if (!total) throw new Error(t('err.noAffix'));
  let needle = random() * total;
  for (const row of rows) { needle -= row.weight; if (needle < 0) return row; }
  return rows.at(-1);
}

export function roll(row, random = Math.random) {
  const values = row.ranges.map(({ min, max }) => Number.isInteger(min) && Number.isInteger(max)
    ? min + Math.floor(random() * (max - min + 1))
    : Number((min + random() * (max - min)).toFixed(2)));
  return { ...row, values };
}

export function rolledText(mod) {
  let index = 0;
  return mod.text.replace(/\((-?\d+(?:\.\d+)?)[—–](-?\d+(?:\.\d+)?)\)/g,
    (range) => String(mod.values?.[index++] ?? range));
}

export function manualReason(item, row) {
  if (item.reveal) return t('err.pendingReveal');
  if (row.required_ilvl > item.ilvl) return t('err.needIlvl', row.required_ilvl);
  if (item.mods.some(m => overlaps(m, row))) return t('err.familyExists');
  if (count(item, row.affix) >= sideLimit(item, row.affix, 'Rare')) return t('err.noFreeSlot', row.affix.toLowerCase());
  if (isCrafted(row) && item.mods.filter(isCrafted).length >= craftedLimit(item)) return t('err.craftedLimit', craftedLimit(item));
  return '';
}

export function manualAdd(item, row, random = Math.random) {
  const reason = manualReason(item, row);
  if (reason) throw new Error(reason);
  const next = { ...item, mods: [...item.mods, roll(row, random)] };
  // Designer actions may promote rarity. They never downgrade it.
  if (next.rarity === 'Normal') next.rarity = 'Magic';
  if (next.rarity === 'Magic' && ['Prefix', 'Suffix'].some(side => count(next, side) > sideLimit(next, side, 'Magic'))) next.rarity = 'Rare';
  return next;
}

// A modifier the current data no longer has: a patch took its family or its
// stat out. Saved crafts keep it, like an old item in the game keeps a legacy
// modifier, but nothing rolls or adds it again, because every candidate comes
// from the data. Matching is lenient on purpose, so a slip in our data does
// not mark a live modifier: the id, or the same side and family with either
// the same stat (any numbers, level or affix name) or the same affix name and
// level (PoE2DB's stand-in id is a hash of the whole row and changes with any
// field). build/refresh_data.py uses the same rule.
export const statShape = text => String(text || '').replace(/\((-?\d+(?:\.\d+)?)[—–](-?\d+(?:\.\d+)?)\)/g, '#')
  .replace(/-?\d+(?:\.\d+)?/g, '#');
export function isRetired(mod, data) {
  if (!mod || mod.unrevealed) return false;
  const shape = statShape(mod.text);
  return !data.mods.some(r => r.source_id === mod.source_id || (r.pool === mod.pool && r.affix === mod.affix &&
    r.families?.[0] === mod.families?.[0] && (statShape(r.text) === shape ||
      (r.name === mod.name && r.required_ilvl === mod.required_ilvl))));
}

export const removeMod = (item, index) => ({ ...item, mods: item.mods.filter((_, i) => i !== index) });
export const clearMods = item => ({ ...item, mods: [] });

export const sortedMods = item => item.mods.map((mod, index) => ({ mod, index }))
  .sort((a, b) => Number(a.mod.affix === 'Suffix') - Number(b.mod.affix === 'Suffix'));

export function replaceTier(item, index, row, random = Math.random) {
  const previous = item.mods[index];
  if (!previous || previous.affix !== row.affix ||
      previous.families[0] !== row.families[0]) throw new Error(t('err.sameFamily'));
  const reason = manualReason(removeMod(item, index), row);
  if (reason) throw new Error(reason);
  return { ...item, mods: item.mods.map((mod, i) => i === index ?
    { ...roll(row, random), ...((previous.desecrated || previous.pool === 'desecrated') ? {desecrated:true} : {}),
      ...(previous.fractured ? {fractured:true} : {}) } : mod) };
}

// Fracturing needs a Rare with at least four affixes and none fractured yet;
// each affix is equally likely to be the one locked. An unrevealed Desecrated
// modifier counts toward the four but cannot be locked, so the others share
// its chance (three affixes + an unrevealed one: 1/3 each).
export const fractureMinimum = 4;
export const fracturable = item => item.mods.map((m, i) => i).filter(i => !item.mods[i].unrevealed);

// removal carries the omens that steer a removal (see removable).
export function currencyReason(item, data, id, rule, removal = {}) {
  if (item.reveal) return t('err.pendingReveal');
  // A Corrupted or Sanctified item takes no more currency.
  if (item.sanctified) return t('err.sanctified');
  if (item.corrupted) return t('err.corrupted');
  if (!supported(id)) return t('err.unsupported');
  if (!rule.beforeRarity.includes(item.rarity)) return t('err.needRarity', rule.beforeRarity.join(' / '));
  if (rule.afterTrigger === 'catalyst') return catalystReason(item, data, id);
  if (rule.afterTrigger === 'infuse') return infuseReason(item, data, id);
  if (rule.afterTrigger === 'fracture') {
    if (item.mods.some(m => m.fractured)) return t('err.alreadyFractured');
    if (item.mods.length < fractureMinimum) return t('err.fractureNeedsMods', fractureMinimum);
    if (!fracturable(item).length) return t('err.noMods');
    return '';
  }
  if (rule.afterTrigger === 'divine' && !item.mods.some(m => !m.fractured)) return item.mods.length ? t('err.onlyFractured') : t('err.noMods');
  const lock = { removal: rule.afterTrigger === 'del_add' ? { ...removal, skipOverfull: true } : removal };
  if (['del', 'del_add'].includes(rule.afterTrigger)) {
    const reason = removalReason(item, lock.removal);
    if (reason) return reason;
  }
  if (rule.afterTrigger === 'add' && !candidates(item, data, {
    minimum: rule.beforeMin_mod_lv || 1, rarity: rule.afterRarity || item.rarity,
  }).length) return t('err.noPool');
  if (rule.afterTrigger === 'del_add') {
    const viable = removable(item, lock.removal).some(index => candidates(removeMod(item, index), data, {
      minimum: rule.beforeMin_mod_lv || 1,
    }).length);
    if (!viable) return t('err.noPoolAfterRemove');
  }
  return '';
}

export function applyCurrency(item, data, id, rule, random = Math.random, removal = {}) {
  const reason = currencyReason(item, data, id, rule, removal);
  if (reason) throw new Error(reason);
  // A Vaal Orb needs the item class and sockets (corrupt.mjs, called by the page).
  if (rule.afterTrigger === 'add_enchant') throw new Error(t('err.unsupported'));
  let next = { ...item, mods: [...item.mods] };
  if (rule.afterTrigger === 'catalyst') return applyCatalyst(next, id);
  if (rule.afterTrigger === 'infuse') return applyInfuser(next, data, id, random);
  if (rule.afterTrigger === 'fracture') {
    const options = fracturable(next), index = options[Math.floor(random() * options.length)];
    next.mods[index] = { ...next.mods[index], fractured: true };
    return next;
  }
  // A fractured affix keeps its values as well as its place.
  if (rule.afterTrigger === 'divine') return { ...next, mods: next.mods.map(m => m.fractured ? m : roll(m, random)) };
  const lock = { removal: rule.afterTrigger === 'del_add' ? { ...removal, skipOverfull: true } : removal };
  if (['del', 'del_add'].includes(rule.afterTrigger)) {
    next = removeRandom(next, lock.removal, random);
    if (rule.afterTrigger === 'del') return next;
  }
  const rows = candidates(next, data, { minimum: rule.beforeMin_mod_lv || 1, rarity: rule.afterRarity || next.rarity });
  next.mods.push(roll(chooseWeighted(rows, random), random));
  next.rarity = rule.afterRarity || next.rarity;
  return next;
}
