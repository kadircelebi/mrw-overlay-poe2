import { applyThemePalette } from './ui-theme.mjs';
// Same-origin cache makes the iframe's first paint follow its host. The host
// always sends the authoritative disk palette again on craft-ready.
try {
  const s=JSON.parse(localStorage.getItem('mrw-ui-theme-v1')||'null');
  const t=s?.themes?.find(t=>t.id===s.selected);
  const base=s?.themes?.find(p=>p.readOnly&&p.id===t?.base);
  if(t&&base)applyThemePalette({base:t.base,colors:t.colors,reference:base.colors,grain:t.grain});
}catch{ /* default theme while the host loads */ }
