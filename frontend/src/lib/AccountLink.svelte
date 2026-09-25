<script lang="ts">
  import { onMount } from 'svelte'
  import { AppService } from '../../bindings/poe2filter'
  import type { BrowserLinkStatus, ChromiumBrowser } from '../../bindings/poe2filter/models'
  import { t } from './i18n.svelte'

  // Connecting the pathofexile.com session through the browser extension, in
  // two steps: install the extension if it does not answer, then sign in on
  // pathofexile.com if it answers without a session.
  let status = $state<BrowserLinkStatus | null>(null)
  let browsers = $state<ChromiumBrowser[]>([])
  let folder = $state('')
  let copied = $state(false)
  let error = $state('')

  const busy = $derived(!!status && ['waiting', 'needExtension', 'needLogin'].includes(status.state))
  // GGG names an "Account" rate-limit rule for requests it counts as signed in.
  const recognized = $derived(status?.rules?.toLowerCase().includes('account') ?? false)
  const connectedAt = $derived(status?.connectedAt ? new Date(status.connectedAt).toLocaleString() : '')

  onMount(() => {
    refresh()
    AppService.ChromiumBrowsers().then((list) => (browsers = list ?? [])).catch(() => {})
    const poll = setInterval(() => { if (busy) refresh() }, 1000)
    return () => clearInterval(poll)
  })

  async function refresh() {
    try { status = await AppService.BrowserLinkState() } catch (e) { error = clean(e) }
  }

  async function run(action: () => Promise<unknown>) {
    error = ''
    try { await action() } catch (e) { error = clean(e) }
    await refresh()
  }

  function clean(e: unknown) { return String(e).replace(/^RuntimeError:\s*/i, '') }

  // The browser the extension lives in, remembered after the first choice so
  // the next connection opens it straight away.
  const PREFERRED = 'mrw.linkBrowser'
  function preferred(): string {
    try { return localStorage.getItem(PREFERRED) ?? '' } catch { return '' }
  }

  const connect = () => run(async () => {
    await AppService.ConnectBrowser(false)
    const id = preferred()
    if (id && browsers.some((b) => b.id === id)) await AppService.OpenLinkIn(id)
  })

  const openIn = (id: string) => run(async () => {
    try { localStorage.setItem(PREFERRED, id) } catch { /* not remembered */ }
    await AppService.OpenLinkIn(id)
  })
  const cancel = () => run(() => AppService.CancelBrowserConnect())
  const disconnect = () => run(() => AppService.DisconnectBrowser())
  const openFolder = () => run(async () => { folder = await AppService.OpenBrowserExtensionFolder() })
  const openPage = (id: string) => run(() => AppService.OpenExtensionsPage(id))

  async function copyLink() {
    if (!status?.url) return
    await navigator.clipboard.writeText(status.url)
    copied = true
    setTimeout(() => (copied = false), 1500)
  }
</script>

<section class="card overlay-settings-card account">
  <h2>{t('account.title')}</h2>
  <p class="desc">{t('account.desc')}</p>

  {#if status?.connected && !busy}
    <p class="state ok">✓ {t('account.connected')}{#if connectedAt} <small>· {connectedAt}</small>{/if}</p>
    {#if status.rules}
      <p class="desc hint">{recognized ? t('account.recognized') : t('account.notRecognized')} <small>({status.rules})</small></p>
    {/if}
    <div class="row">
      <button onclick={connect}>{t('account.reconnect')}</button>
      <button class="ghost" onclick={disconnect}>{t('account.disconnect')}</button>
    </div>
  {:else if !busy}
    <p class="state">{t('account.notConnected')}</p>
    <button class="primary" onclick={connect}>{t('account.connect')}</button>
  {/if}

  {#if status?.state === 'waiting'}
    <p class="state wait">{t('account.waiting')}</p>
  {:else if status?.state === 'needExtension'}
    <div class="step">
      <strong>{t('account.step1')}</strong>
      <ol>
        <li>{t('account.step1Folder')} <button onclick={openFolder}>{t('account.openFolder')}</button>{#if folder}<code>{folder}</code>{/if}</li>
        <li>
          {t('account.step1Page')}
          {#each browsers as browser (browser.id)}
            <button onclick={() => openPage(browser.id)}>{browser.name}{browser.default ? ' ★' : ''}</button>
          {:else}
            <em>{t('account.noBrowser')}</em>
          {/each}
        </li>
        <li>{t('account.step1Load')}</li>
      </ol>
      <p class="desc hint">{t('account.firefoxLater')}</p>
      <button class="primary" onclick={connect}>{t('account.retry')}</button>
    </div>
  {:else if status?.state === 'needLogin'}
    <div class="step">
      <strong>{t('account.step2')}</strong>
      <p class="desc">{t('account.step2Desc')}</p>
    </div>
  {:else if status?.state === 'linked'}
    <p class="state ok">✓ {t('account.linked')}</p>
  {:else if status?.state === 'error'}
    <p class="state bad">{status.error}</p>
  {/if}

  {#if busy}
    <p class="desc hint">{t('account.pickBrowser')}</p>
    <div class="row">
      {#each browsers as browser (browser.id)}
        <button class:primary={browser.id === preferred()} onclick={() => openIn(browser.id)}>{t('account.openIn', browser.name)}</button>
      {/each}
    </div>
    <div class="row">
      {#if status?.url}<button onclick={copyLink}>{copied ? t('account.copied') : t('account.copyLink')}</button>{/if}
      <button class="ghost" onclick={cancel}>{t('account.cancel')}</button>
    </div>
    <p class="desc hint">{t('account.copyHint')}</p>
  {/if}
  {#if status?.extensionOutdated}<p class="desc hint">{t('account.outdated')}</p>{/if}
  {#if error}<p class="state bad">{error}</p>{/if}
</section>

<style>
  .account .state { margin: 8px 0; color: var(--muted); }
  .account .state.ok { color: #9fc48a; }
  .account .state.wait { color: var(--gold-bright); }
  .account .state.bad { color: #e88b84; user-select: text; }
  .account .state small { color: var(--muted); }
  .account .row { display: flex; flex-wrap: wrap; gap: 6px; margin: 6px 0; }
  .account .step { margin: 8px 0; padding: 8px 10px; border: 1px solid var(--line-strong); background: rgba(0,0,0,.18); }
  .account .step ol { margin: 6px 0 8px; padding-left: 18px; }
  .account .step li { margin: 6px 0; line-height: 1.5; }
  .account .step li button { margin: 2px 4px 2px 0; }
  .account code { display: block; margin-top: 4px; font-size: 10px; color: var(--muted); user-select: text; word-break: break-all; }
</style>
