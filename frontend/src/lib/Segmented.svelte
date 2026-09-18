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
  .seg {
    display: flex;
    padding: 3px;
    gap: 3px;
    background: var(--bg);
    border: 1px solid var(--line);
    border-radius: var(--radius-sm);
  }
  button {
    flex: 1;
    padding: 7px 6px;
    border: 0;
    border-radius: 5px;
    background: transparent;
    color: var(--text-2);
    font-weight: 500;
    white-space: nowrap;
    transition: background 0.12s, color 0.12s;
  }
  button:hover {
    color: var(--text);
  }
  button.on {
    background: var(--surface-3);
    color: var(--gold-bright);
    box-shadow: inset 0 0 0 1px var(--line-strong);
  }
  .small button {
    padding: 5px 4px;
    font-size: 12px;
  }
</style>
