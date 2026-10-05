// Shared by the Wails documents and the craft iframe. Only palette roles may
// be written; rarity, loot-filter swatches and special affixes are not roles.
export const paletteKeys=['bg','surface','surface-2','surface-3','sunk','line','line-strong','gold','gold-bright','gold-dim','text','text-2','muted','ok','warn','bad','exceptional','hover','on-accent','item-bg','item-line','item-divider','item-text','item-muted','item-property','item-outline'];
const hex=/^#[\da-f]{6}$/i;
/** @param {string} color */
const isLight=color=>[.2126,.7152,.0722].reduce((n,w,i)=>n+w*Number.parseInt(color.slice(1+i*2,3+i*2),16),0)>150;
/** @param {{base:string,colors:Record<string,string>,reference:Record<string,string>,grain:boolean}} payload */
export function applyThemePalette(payload) {
  if(!payload || !['default','dark','light'].includes(payload.base) || !paletteKeys.every(k=>hex.test(payload.colors?.[k])&&hex.test(payload.reference?.[k])))return false;
  const root=document.documentElement;
  root.dataset.uiTheme=payload.base;
  root.style.colorScheme=isLight(payload.colors.bg)?'light':'dark';
  root.dataset.uiItemLight=String(isLight(payload.colors['item-bg']));
  for(const k of paletteKeys){
    const override=payload.base!=='default'||payload.colors[k]!==payload.reference[k];
    for(const prop of [k,`ui-${k}`]){if(override)root.style.setProperty(`--${prop}`,payload.colors[k]);else root.style.removeProperty(`--${prop}`);}
  }
  const aliases={raised:'surface-2',steel:'line-strong',bright:'gold-bright',secondary:'text-2'};
  for(const [alias,k] of Object.entries(aliases)){const v=root.style.getPropertyValue(`--${k}`);if(v)root.style.setProperty(`--${alias}`,v);else root.style.removeProperty(`--${alias}`);}
  if(payload.base!=='default'){
    root.style.setProperty('--ui-shadow',`color-mix(in srgb, ${payload.colors.bg} 28%, transparent)`);
    root.style.setProperty('--ui-tint',`color-mix(in srgb, ${payload.colors.gold} 9%, transparent)`);
  }else{root.style.removeProperty('--ui-shadow');root.style.removeProperty('--ui-tint');}
  if(!payload.grain)root.style.setProperty('--grain','none');else root.style.removeProperty('--grain');
  return true;
}
