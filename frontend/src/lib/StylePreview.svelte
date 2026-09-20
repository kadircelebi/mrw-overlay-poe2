<script lang="ts">
  import type { StyleGroup } from '../../bindings/poe2filter/internal/filter/models'
  import Swatch from './Swatch.svelte'
  import { effectCss, shapeName, colourName, type Look } from './look'
  import { t } from './i18n.svelte'

  let { group, look, sound = '' }: { group: StyleGroup; look: Look; sound?: string } = $props()

  // In-game font sizes run up to 45; scale them into the panel.
  const fontPx = $derived(Math.round(10 + (group.fontSize / 45) * 8))
  const beamName = $derived((look.beam || '').split(' ')[0])
</script>

<div class="stage" aria-label={t('preview.aria', group.label)}>
  {#if look.beam}<span class="beam" class:temp={look.beam.includes('Temp')} style="--beam: {effectCss(look.beam)}"></span>{/if}
  <Swatch {look} text={group.sample} large {fontPx} />
</div>
<p class="caption">
  {look.beam
    ? t(look.beam.includes('Temp') ? 'preview.beamTemp' : 'preview.beam', colourName(beamName) || beamName)
    : t('preview.noBeam')} ·
  {look.shape
    ? t('preview.minimap', colourName(look.icon) || look.icon, shapeName(look.shape) || look.shape)
    : t('preview.noMinimap')}{sound ? t('preview.soundSuffix', sound) : ''}
</p>

<style>
  .stage {
    position: relative;
    display: flex;
    align-items: center;
    justify-content: center;
    height: 92px;
    margin-top: 6px;
    padding: 0 10px;
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
  .beam.temp {
    opacity: 0.3;
  }
  .caption {
    margin: 6px 0 0;
    color: var(--muted);
    font-size: 11px;
  }
</style>
