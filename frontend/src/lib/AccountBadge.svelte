<script lang="ts">
  import { onMount } from 'svelte'
  import { Events } from '@wailsio/runtime'
  import { AppService } from '../../bindings/poe2filter/internal/app'
  import type { BrowserLinkStatus } from '../../bindings/poe2filter/internal/app/models'
  import { t } from './i18n.svelte'

  let status = $state<BrowserLinkStatus | null>(null)
  let copied = $state(false)
  let failed = $state(false)
  let timer: ReturnType<typeof setTimeout> | undefined
  const name = $derived(status?.connected ? status.accountName ?? '' : '')

  async function refresh() {
    try { status = await AppService.BrowserLinkState() } catch { /* keep the last known state */ }
  }

  onMount(() => {
    void refresh()
    const off = Events.On('browser-account', (ev) => {
      status = ev.data as BrowserLinkStatus
      copied = failed = false
    })
    window.addEventListener('focus', refresh)
    const visible = () => { if (!document.hidden) void refresh() }
    document.addEventListener('visibilitychange', visible)
    return () => {
      off()
      window.removeEventListener('focus', refresh)
      document.removeEventListener('visibilitychange', visible)
      clearTimeout(timer)
    }
  })

  async function copy() {
    try {
      await navigator.clipboard.writeText(name)
      copied = true
      failed = false
    } catch { failed = true }
    clearTimeout(timer)
    timer = setTimeout(() => { copied = failed = false }, 1500)
  }
</script>

{#if name}
  <div class="account-badge" title={name}>
    <span class="name">{name}</span>
    <button class:failed title={t(failed ? 'account.copyNameFailed' : copied ? 'account.copied' : 'account.copyName')}
      aria-label={t('account.copyName')} onclick={copy}>
      <svg viewBox="0 0 24 24" aria-hidden="true">
        {#if copied}<path d="m5 12 4 4L19 6" />
        {:else}<rect x="8" y="8" width="12" height="12" rx="2" /><path d="M16 8V4H4v12h4" />{/if}
      </svg>
    </button>
    <span class="sr-only" role="status">{failed ? t('account.copyNameFailed') : copied ? t('account.copied') : ''}</span>
  </div>
{/if}

<style>
  .account-badge { display: flex; align-items: center; gap: 5px; min-width: 0; max-width: 35%; color: var(--text-2); --wails-draggable: no-drag; }
  .name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 11.5px; user-select: text; }
  button { display: flex; align-items: center; justify-content: center; flex-shrink: 0; width: 26px; height: 26px; padding: 5px; border: 0; border-radius: var(--radius-sm); background: transparent; color: var(--muted); cursor: pointer; }
  button:hover { color: var(--gold-bright); background: var(--surface-2); }
  button:focus-visible { outline: 1px solid var(--gold); }
  button.failed { color: var(--ui-bad, #e88b84); }
  svg { width: 16px; height: 16px; fill: none; stroke: currentColor; stroke-width: 1.5; }
  .sr-only { position: absolute; width: 1px; height: 1px; padding: 0; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
</style>
