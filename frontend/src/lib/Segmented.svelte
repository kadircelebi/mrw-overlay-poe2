<script lang="ts" generics="T extends string | number">
  let {
    options,
    value = $bindable(),
    onchange,
    small = false,
  }: {
    options: { value: T; label: string }[]
    value: T
    onchange?: (v: T) => void
    small?: boolean
  } = $props()
</script>

<div class="seg" class:small role="radiogroup">
  {#each options as o (o.value)}
    <button
      type="button"
      role="radio"
      aria-checked={o.value === value}
      class:on={o.value === value}
      onclick={() => {
        value = o.value
        onchange?.(o.value)
      }}>{o.label}</button
    >
  {/each}
</div>

<style>
  /* One framed strip divided by hairlines, not a row of floating pills. */
  .seg {
    display: flex;
    background: var(--sunk);
    border: 1px solid var(--line-strong);
    border-radius: var(--radius-sm);
    overflow: hidden;
  }
  button {
    flex: 1;
    padding: 7px 6px;
    border: 0;
    border-right: 1px solid var(--line);
    background: transparent;
    color: var(--text-2);
    font-weight: 500;
    white-space: nowrap;
    transition: background 0.12s, color 0.12s;
  }
  button:last-child {
    border-right: 0;
  }
  button:hover {
    color: var(--text);
  }
  button.on {
    background: linear-gradient(180deg, var(--surface-3), var(--surface));
    color: var(--gold-bright);
  }
  .small button {
    padding: 5px 4px;
    font-size: 12px;
  }
</style>
