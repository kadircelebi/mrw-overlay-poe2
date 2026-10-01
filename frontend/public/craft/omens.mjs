import { t } from './i18n.mjs';
import { candidates, currencyReason, applyCurrency, chooseWeighted, roll } from './engine.mjs';

const ids = new Set(['omen-of-greater-exaltation','omen-of-sinistral-exaltation','omen-of-dextral-exaltation',
  'omen-of-sinistral-necromancy','omen-of-dextral-necromancy','omen-of-the-liege','omen-of-the-sovereign','omen-of-the-blackblooded']);
export const omenDefinitions = rules => Object.fromEntries(Object.entries(rules).filter(([id]) => ids.has(id)));
export function relevantOmens(definitions, currencyId, rule, data) {
  return Object.entries(definitions).filter(([,omen]) =>
    (!omen.beforeClassIds || omen.beforeClassIds.includes(data.options.ItemClassesCode)) &&
    (omen.reqids?.includes(currencyId) || (rule.operation === 'desecrate' && omen.reqpool === 'desecrated')));
}
export function omenEffects(definitions, chosen, currencyId, rule, data) {
  const allowed = new Map(relevantOmens(definitions,currencyId,rule,data));
  const effects = {side:null,tags:[],quantity:1};
  for (const id of chosen) {
    const omen = allowed.get(id);
    if (!omen) throw new Error(t('err.omenNotUsable'));
    if (omen.exclusives?.some(ex => chosen.includes(ex))) throw new Error(t('err.omenConflict'));
    if (omen.gentype_only) effects.side = omen.gentype_only === 1 ? 'Prefix' : 'Suffix';
    if (omen.harvest_only) effects.tags = omen.harvest_only;
    if (id === 'omen-of-greater-exaltation') effects.quantity = 2;
  }
  return effects;
}
export const filterOmenRows = (rows,effects) => rows.filter(m =>
  (!effects.side || m.affix === effects.side) && (!effects.tags.length || effects.tags.some(t => m.tags.includes(t))));

export function orbOmenReason(item,data,id,rule,effects) {
  const baseReason = currencyReason(item,data,id,rule); if (baseReason) return baseReason;
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
  if (!id.includes('exalted')) return applyCurrency(item,data,id,rule,random);
  const next = {...item,mods:[...item.mods]};
  for (let i=0;i<effects.quantity;i++) {
    const rows = filterOmenRows(candidates(next,data,{minimum:rule.beforeMin_mod_lv || 1}),effects);
    next.mods.push(roll(chooseWeighted(rows,random),random));
  }
  return next;
}
