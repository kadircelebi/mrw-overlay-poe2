<script lang="ts">
  import { t } from './i18n.svelte'

  // The panel header's league: the name, a small "auto" tag while it follows
  // the current league, a warning when a hand-picked league has left the list,
  // and a menu to switch. AUTO is the value that stands for Automatic.
  let {
    league,
    auto,
    unlisted,
    options,
    AUTO,
    onpick,
  }: { league: string; auto: boolean; unlisted: boolean; options: string[]; AUTO: string; onpick: (value: string) => void } = $props()
  let open = $state(false)
  let host: HTMLDivElement

  function closeOutside(event: PointerEvent) {
    if (host && !host.contains(event.target as Node)) open = false
  }

  function pick(value: string) {
    open = false
    if (value === AUTO ? !auto : auto || value.toLowerCase() !== league.toLowerCase()) onpick(value)
  }
</script>

<svelte:window onpointerdown={closeOutside} onkeydown={(event) => { if (event.key === 'Escape') open = false }} />
<div class="league-menu" bind:this={host}>
  <button
    class="trigger"
    class:unlisted
    title={unlisted ? t('general.leagueUnlisted') : t('header.leaguePick')}
    aria-haspopup="menu"
    aria-expanded={open}
    onclick={() => (open = !open)}
  >
    <span class="name">{league}</span>
    {#if auto}<span class="tag">{t('header.leagueAutoTag')}</span>{/if}
    {#if unlisted}<span aria-hidden="true">⚠</span>{/if}
    <span class="caret" aria-hidden="true">▾</span>
  </button>
  {#if open}
    <div class="menu" role="menu">
      <button role="menuitemradio" aria-checked={auto} class:on={auto} onclick={() => pick(AUTO)}>
        {auto ? t('general.leagueAutoNow', league) : t('general.leagueAuto')}
      </button>
      <hr />
      {#each options as l (l)}
        {@const on = !auto && l.toLowerCase() === league.toLowerCase()}
        <button role="menuitemradio" aria-checked={on} class:on onclick={() => pick(l)}>{l}</button>
      {/each}
    </div>
  {/if}
</div>

<style>
  .league-menu {
    position: relative;
    min-width: 0;
    --wails-draggable: no-drag;
  }
  .trigger {
    display: flex;
    align-items: baseline;
    gap: 5px;
    max-width: 100%;
    padding: 1px 4px;
    border: 0;
    border-radius: 3px;
    background: none;
    color: var(--muted);
    font-size: 11.5px;
    white-space: nowrap;
    cursor: pointer;
  }
  .trigger:hover,
  .trigger[aria-expanded='true'] {
    background: var(--ui-hover, #2b2d23);
    color: var(--ui-text, #ddd);
  }
  .trigger:focus-visible,
  .menu button:focus-visible {
    outline: 2px solid var(--gold-bright);
    outline-offset: 1px;
  }
  .trigger.unlisted {
    color: var(--warn);
  }
  .name {
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .tag {
    padding: 0 4px;
    border: 1px solid var(--ui-line, #3a3a30);
    border-radius: 3px;
    font-size: 9.5px;
    letter-spacing: 0.04em;
    opacity: 0.8;
  }
  .caret {
    font-size: 9px;
    opacity: 0.7;
  }
  .menu {
    position: absolute;
    z-index: 30;
    top: calc(100% + 6px);
    left: 0;
    min-width: 220px;
    max-height: 320px;
    overflow-y: auto;
    padding: 4px;
    border: 1px solid var(--ui-line-strong, #8c7b50);
    background: var(--ui-surface, #171917);
    box-shadow: 0 8px 20px var(--ui-shadow, #0009);
  }
  .menu button {
    display: block;
    width: 100%;
    padding: 8px 10px 8px 22px;
    border: 0;
    background: none;
    color: var(--ui-text, #ddd);
    text-align: left;
    font-size: 12px;
    position: relative;
    white-space: nowrap;
  }
  .menu button:hover {
    background: var(--ui-hover, #2b2d23);
  }
  .menu button.on {
    color: var(--gold-bright);
  }
  .menu button.on::before {
    content: '✓';
    position: absolute;
    left: 7px;
  }
  hr {
    margin: 4px 6px;
    border: 0;
    border-top: 1px solid var(--ui-line, #3a3a30);
  }
</style>
