import { t } from './i18n.mjs';
import { rolledText, sortedMods, isCrafted } from './engine.mjs';
import { baseStats, propertyLines } from './stats.mjs';

// The text the price check parses for a crafted item. With a base picked it
// names that base and prints the numbers the base would show (quality,
// defences, weapon damage); without one it names the class in place of a
// base and the market searches the class.
export function craftText(item, itemClass = 'Gloves', base = null) {
  if (item.reveal) throw new Error(t('err.pendingRevealPrice'));
  if (!['Normal','Magic','Rare'].includes(item.rarity)) throw new Error(t('err.badRarity'));
  const lines = [`Item Class: ${itemClass}`,`Rarity: ${item.rarity}`];
  if (item.rarity === 'Rare') lines.push('Theoretical Craft');
  lines.push(base?.name || itemClass,'--------');
  const properties = propertyLines(baseStats(base, item));
  if (properties.length) lines.push(...properties,'--------');
  if (item.sockets) lines.push(`Sockets: ${Array(item.sockets).fill('S').join(' ')}`,'--------');
  lines.push(`Item Level: ${item.ilvl}`,'--------');
  for (const {mod} of sortedMods(item)) {
    // An unrevealed Desecrated modifier has no stat to search for yet.
    if (mod.unrevealed) continue;
    // The trade site files these apart (fractured.stat_…, crafted.stat_…,
    // desecrated.stat_…); the header makes the price check search the right one.
    const kind = mod.fractured ? 'Fractured ' : isCrafted(mod) ? 'Crafted ' : mod.desecrated || mod.pool === 'desecrated' ? 'Desecrated ' : '';
    lines.push(`{ ${kind}${mod.affix} Modifier (Tier: ${mod.tier}) }`);
    lines.push(...rolledText(mod).split(/\n|<br\s*\/?\s*>/i));
  }
  return lines.join('\n');
}
