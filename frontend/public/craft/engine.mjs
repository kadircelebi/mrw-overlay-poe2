import { t } from './i18n.mjs';
// The craft state is independent of the UI. Rarity is never inferred on removal.
export const limits = { Normal: 0, Magic: 1, Rare: 3 };
export const supported = id => /^(transmute|aug|regal|exalted|chaos|annu|divine|(?:greater|perfect)-(?:orb-of-transmutation|orb-of-augmentation|regal-orb|exalted-orb|chaos-orb))$/.test(id);
export const createItem = (base = 'Gloves_str') => ({ base, rarity: 'Normal', ilvl: 81, mods: [] });
export const count = (item, side) => item.mods.filter(m => m.affix === side).length;
export const overlaps = (a, b) => a.affix === b.affix && a.families.some(f => b.families.includes(f));

export function rarityReason(item, rarity) {
  if (!(rarity in limits)) return t('err.badRarity');
  if (['Prefix', 'Suffix'].some(side => count(item, side) > limits[rarity])) {
    return rarity === 'Normal' ? t('err.normalNeedsEmpty') : t('err.magicLimit');
  }
  return '';
}

export function setRarity(item, rarity) {
  const reason = rarityReason(item, rarity);
  if (reason) throw new Error(reason);
  return { ...item, rarity };
}

export function candidates(item, data, { pool = 'normal', minimum = 1, rarity = item.rarity } = {}) {
  const rows = data.mods.filter(m => m.pool === pool && ['Prefix', 'Suffix'].includes(m.affix) && m.weight > 0);
  const highest = new Map();
  for (const row of rows) {
    const key = row.affix + ':' + row.families[0];
    highest.set(key, Math.max(highest.get(key) || 0, row.required_ilvl));
  }
  return rows.filter(m => m.required_ilvl <= item.ilvl &&
    (m.required_ilvl >= minimum || m.required_ilvl === highest.get(m.affix + ':' + m.families[0])) &&
    count(item, m.affix) < limits[rarity] && !item.mods.some(existing => overlaps(existing, m)));
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
  if (count(item, row.affix) >= 3) return t('err.noFreeSlot', row.affix.toLowerCase());
  return '';
}

export function manualAdd(item, row, random = Math.random) {
  const reason = manualReason(item, row);
  if (reason) throw new Error(reason);
  const next = { ...item, mods: [...item.mods, roll(row, random)] };
  // Designer actions may promote rarity. They never downgrade it.
  if (next.rarity === 'Normal') next.rarity = 'Magic';
  if (next.rarity === 'Magic' && ['Prefix', 'Suffix'].some(side => count(next, side) > 1)) next.rarity = 'Rare';
  return next;
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
    { ...roll(row, random), ...((previous.desecrated || previous.pool === 'desecrated') ? {desecrated:true} : {}) } : mod) };
}

export function currencyReason(item, data, id, rule) {
  if (item.reveal) return t('err.pendingReveal');
  if (!supported(id)) return t('err.unsupported');
  if (!rule.beforeRarity.includes(item.rarity)) return t('err.needRarity', rule.beforeRarity.join(' / '));
  if (['del', 'del_add', 'divine'].includes(rule.afterTrigger) && !item.mods.length) return t('err.noMods');
  if (rule.afterTrigger === 'add' && !candidates(item, data, {
    minimum: rule.beforeMin_mod_lv || 1, rarity: rule.afterRarity || item.rarity,
  }).length) return t('err.noPool');
  if (rule.afterTrigger === 'del_add') {
    const viable = item.mods.some((_, index) => candidates(removeMod(item, index), data, {
      minimum: rule.beforeMin_mod_lv || 1,
    }).length);
    if (!viable) return t('err.noPoolAfterRemove');
  }
  return '';
}

export function applyCurrency(item, data, id, rule, random = Math.random) {
  const reason = currencyReason(item, data, id, rule);
  if (reason) throw new Error(reason);
  let next = { ...item, mods: [...item.mods] };
  if (rule.afterTrigger === 'divine') return { ...next, mods: next.mods.map(m => roll(m, random)) };
  if (['del', 'del_add'].includes(rule.afterTrigger)) {
    next = removeMod(next, Math.floor(random() * next.mods.length));
    if (rule.afterTrigger === 'del') return next;
  }
  const rows = candidates(next, data, { minimum: rule.beforeMin_mod_lv || 1, rarity: rule.afterRarity || next.rarity });
  next.mods.push(roll(chooseWeighted(rows, random), random));
  next.rarity = rule.afterRarity || next.rarity;
  return next;
}
