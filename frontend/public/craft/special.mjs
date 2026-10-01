import { t } from './i18n.mjs';
import { candidates, chooseWeighted, roll, overlaps, limits, count, removeMod } from './engine.mjs';
import { filterOmenRows } from './omens.mjs';
const noOmens = {side:null,tags:[],quantity:1};

export function applicable(rule, data) {
  return !rule.beforeClassIds || rule.beforeClassIds.includes(data.options.ItemClassesCode);
}

// data.tags are the tags every base of the class carries (from the data
// build), so an essence row fits jewellery and weapons as well as armour.
export function essenceRows(data, rule) {
  const tags = new Set(data.tags || [data.options.ItemClassesCode.toLowerCase(), 'armour', data.options.tags]);
  return data.mods.filter(m => m.pool === rule.pool && m.name === rule.name &&
    (!m.spawn_tags.some(t => t !== 'default') || m.spawn_tags.some(t => tags.has(t))));
}

function availableEssences(item, data, rule) {
  return essenceRows(data, rule).filter(m => m.required_ilvl <= item.ilvl &&
    count(item, m.affix) < limits.Rare && !item.mods.some(other => overlaps(other, m)));
}

export function specialReason(item, data, rule, effects=noOmens) {
  if (item.reveal) return t('err.pendingReveal');
  if (!applicable(rule, data)) return t('err.notForClass');
  if (!rule.beforeRarity.includes(item.rarity)) return t('err.needRarity', rule.beforeRarity.join(' / '));
  if (rule.operation === 'desecrate') {
    if (item.mods.some(m => m.desecrated || m.pool === 'desecrated')) return t('err.hasDesecrated');
    if (rule.maxItemLevel && item.ilvl > rule.maxItemLevel) return t('err.maxIlvl', rule.maxItemLevel);
    const possible = item.mods.length === 6 ? item.mods.some((m,i) => (!effects.side || m.affix === effects.side) && boneRows(removeMod(item,i),data,rule,effects).length) : boneRows(item,data,rule,effects).length;
    return possible ? '' : t('err.noDesecratePool');
  }
  if (rule.removes) {
    if (!item.mods.length) return t('err.nothingToRemove');
    // Do not preselect a favourable removal: all random removals must be valid.
    return item.mods.every((_,i) => availableEssences(removeMod(item,i),data,rule).length) ? '' :
      t('err.essenceRemoveRisk');
  }
  return availableEssences(item,data,rule).length ? '' : t('err.essenceNoRoom');
}

function boneRows(item,data,rule,effects=noOmens) {
  return filterOmenRows((effects.tags.length ? ['desecrated'] : ['normal','desecrated'])
    .flatMap(pool => candidates(item,data,{pool,minimum:rule.minimum || 1})),effects);
}

export function applySpecial(item, data, rule, random = Math.random, effects=noOmens) {
  const reason = specialReason(item,data,rule,effects); if (reason) throw new Error(reason);
  let next = structuredClone(item);
  if (rule.operation === 'essence') {
    if (rule.removes) next = removeMod(next,Math.floor(random()*next.mods.length));
    const rows = availableEssences(next,data,rule);
    // Multiple guaranteed attribute variants are equiprobable in this prototype.
    next.mods.push(roll(rows[Math.floor(random()*rows.length)],random));
    next.rarity = 'Rare'; return next;
  }
  if (next.mods.length === 6) {
    const removable = next.mods.map((m,i)=>({m,i})).filter(({m})=>!effects.side || m.affix === effects.side);
    next = removeMod(next,removable[Math.floor(random()*removable.length)].i);
  }
  let rows = boneRows(next,data,rule,effects);
  const first = chooseWeighted(rows,random), choices = [roll(first,random)];
  rows = rows.filter(m => m.affix === first.affix && !overlaps(m,first));
  while (choices.length < 3 && rows.length) {
    const chosen = chooseWeighted(rows,random); choices.push(roll(chosen,random));
    rows = rows.filter(m => !overlaps(m,chosen));
  }
  const index = next.mods.length;
  next.mods.push({source_id:'local-unrevealed',pool:'desecrated',affix:first.affix,name:'Unrevealed',tier:'?',
    families:['Unrevealed'],required_ilvl:1,text:'Unrevealed Desecrated modifier',ranges:[],values:[],desecrated:true});
  next.reveal = {index,choices,effects:structuredClone(effects)}; return next;
}

export function revealChoice(item,index) {
  if (!item.reveal || !item.reveal.choices[index]) throw new Error(t('err.badReveal'));
  const next = structuredClone(item);
  next.mods[next.reveal.index] = {...next.reveal.choices[index],desecrated:true};
  delete next.reveal; return next;
}
