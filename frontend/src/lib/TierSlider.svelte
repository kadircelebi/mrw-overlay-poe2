<script lang="ts">
  import { t } from './i18n.svelte'

  // A threshold picker whose leftmost stop is "off": one drag covers both the
  // on/off choice and how strict the rule is, which a checkbox plus a number
  // could not do in this narrow panel.
  let {
    value = $bindable(),
    min,
    max,
    label,
    hint = '',
    prefix = '',
    off = -1,
    onchange,
  }: {
    value: number
    min: number
    max: number
    label: string
    hint?: string
    prefix?: string // e.g. "T" for waystone tiers
    off?: number
    onchange?: () => void
  } = $props()

  // Slider positions: 0 is off, 1..steps map to min..max.
  const steps = $derived(max - min + 1)
  const index = $derived(value === off ? 0 : Math.min(steps, Math.max(1, value - min + 1)))
  const shown = $derived(value === off ? t('tier.off') : `${prefix}${value}+`)

  function pick(i: number) {
    value = i === 0 ? off : min + i - 1
    onchange?.()
  }
</script>

<div class="tier">
  <div class="head">
    <span class="label">{label}</span>
    <span class="value" class:off={value === off}>{shown}</span>
  </div>
  {#if hint}<p class="desc">{hint}</p>{/if}
  <input
    type="range"
    class="slider"
    min="0"
    max={steps}
    step="1"
    value={index}
    aria-label={label}
    aria-valuetext={shown}
    oninput={(e) => pick(Number(e.currentTarget.value))}
    style="--p: {(index / steps) * 100}%"
  />
  <div class="scale">
    <span>{t('tier.off')}</span>
    <span>{prefix}{min}+</span>
    <span>{prefix}{max}+</span>
  </div>
</div>

<style>
  .tier {
    padding: 8px 0 2px;
  }
  .head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 8px;
  }
  .label {
    font-weight: 600;
  }
  .value {
    color: var(--gold-bright);
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
  }
  .value.off {
    color: var(--muted);
  }
  .desc {
    margin: 2px 0 0;
    color: var(--muted);
    font-size: 12px;
  }
  .slider {
    width: 100%;
    margin: 10px 0 2px;
    -webkit-appearance: none;
    appearance: none;
    height: 6px;
    border-radius: 6px;
    background: linear-gradient(90deg, var(--gold-dim) var(--p), var(--bg) var(--p));
    border: 1px solid var(--line);
  }
  .slider::-webkit-slider-thumb {
    -webkit-appearance: none;
    width: 16px;
    height: 16px;
    border-radius: 50%;
    background: var(--gold-bright);
    border: 3px solid var(--surface);
    box-shadow: 0 0 0 1px var(--gold-dim);
    cursor: pointer;
  }
  .scale {
    display: flex;
    justify-content: space-between;
    color: var(--muted);
    font-size: 11px;
  }
</style>
