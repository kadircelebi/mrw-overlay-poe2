import { t } from './i18n.mjs';
import { rolledText, sortedMods } from './engine.mjs';

// The craft picks an item class and defence type, not a real base, so the
// text names the class in place of a base and the market searches the class.
export function craftText(item, itemClass = 'Gloves') {
  if (item.reveal) throw new Error(t('err.pendingRevealPrice'));
  if (!['Normal','Magic','Rare'].includes(item.rarity)) throw new Error(t('err.badRarity'));
  const lines = [`Item Class: ${itemClass}`,`Rarity: ${item.rarity}`];
  if (item.rarity === 'Rare') lines.push('Theoretical Craft');
  lines.push(itemClass,'--------',`Item Level: ${item.ilvl}`,'--------');
  for (const {mod} of sortedMods(item)) {
    lines.push(`{ ${mod.desecrated || mod.pool === 'desecrated' ? 'Desecrated ' : ''}${mod.affix} Modifier (Tier: ${mod.tier}) }`);
    lines.push(...rolledText(mod).split(/\n|<br\s*\/?\s*>/i));
  }
  return lines.join('\n');
}
