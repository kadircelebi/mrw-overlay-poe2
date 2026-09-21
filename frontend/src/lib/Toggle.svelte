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
  /* The game has no sliding switches; it has a round socket that either holds
     a lit gem or sits empty. The knob grows into place instead of travelling. */
  .track {
    flex: none;
    width: 19px;
    height: 19px;
    border-radius: 50%;
    background: radial-gradient(circle at 35% 30%, var(--surface-3), var(--sunk));
    border: 1px solid var(--line-strong);
    box-shadow: inset 0 1px 2px #000;
    position: relative;
    transition: border-color 0.15s;
  }
  .knob {
    position: absolute;
    inset: 3px;
    border-radius: 50%;
    background: var(--gold-bright);
    transform: scale(0);
    opacity: 0;
    transition: transform 0.15s, opacity 0.15s;
  }
  .track.on {
    border-color: var(--gold-dim);
  }
  .track.on .knob {
    transform: scale(1);
    opacity: 1;
  }
</style>
