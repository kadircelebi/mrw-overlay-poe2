<script lang="ts">
  import { css, effectCss, shapePaths, type Look } from './look'

  let { look, text = 'Örnek eşya', large = false, fontPx = 0 }: { look: Look; text?: string; large?: boolean; fontPx?: number } = $props()
</script>

<span
  class="swatch"
  class:large
  style="background: {look.bg ? css(look.bg) : 'rgba(0,0,0,0.75)'}; color: {look.text ? css(look.text) : '#c8c8c8'}; border-color: {look.border
    ? css(look.border)
    : 'transparent'}; {fontPx ? `font-size: ${fontPx}px` : ''}"
>
  {#if look.shape && look.icon}
    <svg viewBox="0 0 24 24" aria-hidden="true" style="--c: {effectCss(look.icon)}"><path d={shapePaths[look.shape] ?? shapePaths.Circle} /></svg>
  {/if}
  <span class="t">{text}</span>
</span>

<style>
  .swatch {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 6px;
    min-width: 0;
    max-width: 100%;
    padding: 2px 10px;
    border: 1px solid;
    font-family: Georgia, 'Times New Roman', serif;
    font-variant: small-caps;
    font-size: 12px;
    letter-spacing: 0.03em;
    white-space: nowrap;
  }
  .swatch.large {
    padding: 5px 12px;
    border-width: 2px;
    font-weight: 700;
    font-size: 17px;
  }
  .t {
    overflow: hidden;
    text-overflow: ellipsis;
  }
  svg {
    flex: none;
    width: 1.05em;
    height: 1.05em;
    fill: var(--c);
    stroke: rgba(0, 0, 0, 0.6);
    stroke-width: 1;
  }
</style>
