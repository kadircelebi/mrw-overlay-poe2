import { t } from './i18n.mjs';
import { candidates, currencyReason, applyCurrency, chooseWeighted, roll } from './engine.mjs';
import { sanctify, sanctifyReason } from './corrupt.mjs';

const ids = new Set(['omen-of-greater-exaltation','omen-of-sinistral-exaltation','omen-of-dextral-exaltation',
  'omen-of-sinistral-necromancy','omen-of-dextral-necromancy','omen-of-the-liege','omen-of-the-sovereign','omen-of-the-blackblooded',
  // Omens that steer what an Annulment or a Chaos Orb removes.
  'omen-of-light','omen-of-greater-annulment','omen-of-sinistral-annulment','omen-of-dextral-annulment',
  'omen-of-sinistral-erasure','omen-of-dextral-erasure','omen-of-whittling',
  // Which side a Perfect or Corrupted essence removes from.
  'omen-of-sinistral-crystallisation','omen-of-dextral-crystallisation',
  // A Divine Orb on a Rare Sanctifies it instead (corrupt.mjs).
  'omen-of-sanctification']);
const crystallisation = ['omen-of-sinistral-crystallisation','omen-of-dextral-crystallisation'];
// The data gives the Crystallisation pair no exclusives; they exclude each other.
export const omenDefinitions = rules => Object.fromEntries(Object.entries(rules).filter(([id]) => ids.has(id))
  .map(([id,omen]) => [id, crystallisation.includes(id) ? {...omen,exclusives:crystallisation.filter(x => x !== id)} : omen]));
export function relevantOmens(definitions, currencyId, rule, data) {
  return Object.entries(definitions).filter(([,omen]) =>
    (!omen.beforeClassIds || omen.beforeClassIds.includes(data.options.ItemClassesCode)) &&
    (omen.reqids?.includes(currencyId) || (rule.operation === 'desecrate' && omen.reqpool === 'desecrated') ||
      (rule.operation === 'essence' && rule.removes && omen.reqids?.includes('perfect-essences'))));
}
// A removal omen works on the Annulment / Chaos Orb's removal; the others on
// what is added (side and tags) and how many.
const removesWith = omen => omen.reqids?.some(id => id === 'annu' || id.includes('chaos') || id === 'perfect-essences');
export function omenEffects(definitions, chosen, currencyId, rule, data) {
  const allowed = new Map(relevantOmens(definitions,currencyId,rule,data));
  const effects = {side:null,tags:[],quantity:1,removal:{},sanctify:false};
  for (const id of chosen) {
    const omen = allowed.get(id);
    if (!omen) throw new Error(t('err.omenNotUsable'));
    if (omen.exclusives?.some(ex => chosen.includes(ex))) throw new Error(t('err.omenConflict'));
    if (removesWith(omen)) {
      if (crystallisation.includes(id)) effects.removal.side = id.includes('sinistral') ? 'Prefix' : 'Suffix';
      if (omen.gentype_only) effects.removal.side = omen.gentype_only === 1 ? 'Prefix' : 'Suffix';
      if (id === 'omen-of-light') effects.removal.desecrated = true;
      if (id === 'omen-of-greater-annulment') effects.removal.count = 2;
      if (id === 'omen-of-whittling') effects.removal.lowest = true;
      continue;
    }
    if (omen.gentype_only) effects.side = omen.gentype_only === 1 ? 'Prefix' : 'Suffix';
    if (omen.harvest_only) effects.tags = omen.harvest_only;
    if (id === 'omen-of-greater-exaltation') effects.quantity = 2;
    if (id === 'omen-of-sanctification') effects.sanctify = true;
  }
  return effects;
}
export const filterOmenRows = (rows,effects) => rows.filter(m =>
  (!effects.side || m.affix === effects.side) && (!effects.tags.length || effects.tags.some(t => m.tags.includes(t))));

export function orbOmenReason(item,data,id,rule,effects) {
  if (effects.sanctify) return sanctifyReason(item);
  const baseReason = currencyReason(item,data,id,rule,effects.removal || {}); if (baseReason) return baseReason;
  if (!id.includes('exalted')) return '';
  const rows = filterOmenRows(candidates(item,data,{minimum:rule.beforeMin_mod_lv || 1}),effects);
  if (!rows.length) return t('err.omenNoRoom');
  if (effects.quantity === 2 && rows.some(row => {
    const next = {...item,mods:[...item.mods,row]};
    return !filterOmenRows(candidates(next,data,{minimum:rule.beforeMin_mod_lv || 1}),effects).length;
  })) return t('err.omenTwo');
  return '';
}
export function applyOrbOmens(item,data,id,rule,effects,random=Math.random) {
  const reason = orbOmenReason(item,data,id,rule,effects); if (reason) throw new Error(reason);
  if (effects.sanctify) return sanctify(item,random);
  if (!id.includes('exalted')) return applyCurrency(item,data,id,rule,random,effects.removal || {});
  const next = {...item,mods:[...item.mods]};
  for (let i=0;i<effects.quantity;i++) {
    const rows = filterOmenRows(candidates(next,data,{minimum:rule.beforeMin_mod_lv || 1}),effects);
    next.mods.push(roll(chooseWeighted(rows,random),random));
  }
  return next;
}
