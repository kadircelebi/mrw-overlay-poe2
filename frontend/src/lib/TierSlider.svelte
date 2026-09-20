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
    hide = -2,
    onchange,
  }: {
    value: number
    min: number
    max: number
    label: string
    hint?: string
    prefix?: string // e.g. "T" for waystone tiers
    off?: number
    hide?: number
    onchange?: () => void
  } = $props()

  // Positions: 0 hides everything, 1 writes no rule at all, 2..steps map to
  // min..max. The two words come first because they are the coarse decision.
  const steps = $derived(max - min + 2)
  const index = $derived(
    value === hide ? 0 : value === off ? 1 : Math.min(steps, Math.max(2, value - min + 2)),
  )
  const shown = $derived(value === hide ? t('tier.hide') : value === off ? t('tier.off') : `${prefix}${value}+`)

  function pick(i: number) {
    value = i === 0 ? hide : i === 1 ? off : min + i - 2
    onchange?.()
  }
</script>

<div class="tier">
  <div class="head">
    <span class="label">{label}</span>
    <span class="value" class:off={value === off} class:hide={value === hide}>{shown}</span>
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
  <!-- The two word stops sit at the left end, in this order, before the
       numbers start; naming them together avoids two labels colliding. -->
  <div class="scale">
    <span>{t('tier.hide')} · {t('tier.off')}</span>
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
  .value.hide {
    color: var(--bad);
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
