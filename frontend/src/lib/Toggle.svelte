<script lang="ts">
  let {
    checked = $bindable(false),
    label,
    hint = '',
    onchange,
  }: { checked: boolean; label: string; hint?: string; onchange?: (v: boolean) => void } = $props()

  function flip() {
    checked = !checked
    onchange?.(checked)
  }
</script>

<button type="button" class="row" role="switch" aria-checked={checked} onclick={flip}>
  <span class="text">
    <span class="label">{label}</span>
    {#if hint}<span class="hint">{hint}</span>{/if}
  </span>
  <span class="track" class:on={checked}><span class="knob"></span></span>
</button>

<style>
  .row {
    display: flex;
    align-items: center;
    gap: 12px;
    width: 100%;
    padding: 9px 0;
    background: none;
    border: 0;
    text-align: left;
  }
  .text {
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .label {
    color: var(--text);
    font-weight: 500;
  }
  .hint {
    color: var(--muted);
    font-size: 11.5px;
  }
  .track {
    flex: none;
    width: 34px;
    height: 20px;
    border-radius: 20px;
    background: var(--surface-3);
    border: 1px solid var(--line-strong);
    position: relative;
    transition: background 0.15s, border-color 0.15s;
  }
  .knob {
    position: absolute;
    top: 2px;
    left: 2px;
    width: 14px;
    height: 14px;
    border-radius: 50%;
    background: var(--text-2);
    transition: transform 0.15s, background 0.15s;
  }
  .track.on {
    background: rgba(201, 164, 92, 0.25);
    border-color: var(--gold-dim);
  }
  .track.on .knob {
    transform: translateX(14px);
    background: var(--gold-bright);
  }
</style>
