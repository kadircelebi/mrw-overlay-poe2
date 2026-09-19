<script lang="ts">
  import type { DivineThemeStyle } from '../../bindings/poe2filter/internal/filter/models'

  let { theme }: { theme: DivineThemeStyle | undefined } = $props()

  // Filter colours are "R G B A" with alpha 0..255.
  function css(c: string | undefined): string {
    const [r, g, b, a = 255] = (c ?? '').split(' ').map(Number)
    return Number.isFinite(r) ? `rgba(${r}, ${g}, ${b}, ${a / 255})` : 'transparent'
  }

  // In-game beam / minimap colour names.
  const beamColours: Record<string, string> = {
    Cyan: '#2fe6f0', Purple: '#b25cff', Red: '#ff3b3b', Yellow: '#ffd23a',
    Green: '#3be36a', White: '#f4f4f4', Blue: '#4a7dff', Orange: '#ff9a2e', Pink: '#ff6fb5',
  }
  const beam = $derived(beamColours[theme?.beam ?? ''] ?? '#ccc')
</script>

{#if theme}
  <div class="stage" aria-label="Divine Orb önizlemesi">
    <span class="beam" style="--beam: {beam}"></span>
    <span
      class="label"
      style="background: {css(theme.bg)}; color: {css(theme.text)}; border-color: {css(theme.border)}"
      >Divine Orb</span
    >
    <span class="icon" title="Minimap simgesi" style="--beam: {beam}">
      <svg viewBox="0 0 24 24"><path d="M12 2l2.9 6.6 7.1.6-5.4 4.7 1.6 7L12 17.3 5.8 20.9l1.6-7L2 9.2l7.1-.6z" /></svg>
    </span>
  </div>
  <p class="caption">Işın ve minimap simgesi: {theme.beam}</p>
{/if}

<style>
  .stage {
    position: relative;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 14px;
    height: 92px;
    margin-top: 6px;
    border-radius: var(--radius-sm);
    border: 1px solid var(--line);
    /* rough ground texture so light and dark labels both read */
    background:
      radial-gradient(60% 80% at 30% 70%, rgba(80, 70, 50, 0.35), transparent 70%),
      radial-gradient(50% 60% at 75% 30%, rgba(40, 55, 40, 0.35), transparent 70%),
      #14110e;
    overflow: hidden;
  }
  .beam {
    position: absolute;
    left: 50%;
    bottom: 50%;
    width: 6px;
    height: 120px;
    transform: translateX(-50%);
    background: linear-gradient(to top, var(--beam), transparent);
    opacity: 0.55;
    filter: blur(2px);
  }
  .label {
    position: relative;
    padding: 5px 12px;
    border: 2px solid;
    font-family: Georgia, 'Times New Roman', serif;
    font-size: 17px;
    font-weight: 700;
    letter-spacing: 0.02em;
    white-space: nowrap;
  }
  .icon svg {
    width: 20px;
    height: 20px;
    fill: var(--beam);
    filter: drop-shadow(0 0 4px var(--beam));
  }
  .caption {
    margin: 6px 0 0;
    color: var(--muted);
    font-size: 11px;
  }
</style>
