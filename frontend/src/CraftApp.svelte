<script lang="ts">
  import { onMount } from 'svelte'
  import { Events } from '@wailsio/runtime'
  import { AppService } from '../bindings/poe2filter'
  import { allOn, buildRequest, choicesFor } from './lib/overlayQuery'
  import { t } from './lib/i18n.svelte'
  import { followAppLanguage } from './lib/windowLang'

  let frame: HTMLIFrameElement
  let error = $state('')
  let searching = false

  function send(data: unknown) { frame?.contentWindow?.postMessage(data, location.origin) }
  async function refreshPrices() {
    try { send({ type: 'craft-prices', prices: await AppService.GetCraftPrices() }) }
    catch (e) { error = String(e) }
  }
  onMount(() => {
    const offLang = followAppLanguage()
    const offPrices = Events.On('state', () => void refreshPrices())
    const receive = async (event: MessageEvent) => {
      if (event.source !== frame?.contentWindow || event.origin !== location.origin) return
      if (event.data?.type === 'craft-ready') { await refreshPrices(); return }
      if (event.data?.type !== 'craft-price' || typeof event.data.raw !== 'string' || searching) return
      searching = true; error = ''
      try {
        const snap = await AppService.ParseCraftText(event.data.raw)
        if (!snap.item) throw new Error('Craft item could not be parsed')
        const item = snap.item
        const unmatched = (item.mods ?? []).filter(m => m.type !== 'pseudo' && !m.statId)
        if (unmatched.length) throw new Error(t('craft.unmatched') + unmatched.map(m => m.text).join('; '))
        const choices = choicesFor(item, false)
        const query = buildRequest(item, choices, 'securable', [], [], { ...allOn, base: false })
        await AppService.ShowCraftMarketWithQuery(event.data.raw, query)
        send({type:'craft-result'})
      } catch (e) {
        error = String(e); send({ type:'craft-result', error })
      } finally { searching = false }
    }
    window.addEventListener('message', receive)
    window.addEventListener('focus', refreshPrices)
    return () => {
      offLang(); offPrices()
      window.removeEventListener('message', receive)
      window.removeEventListener('focus', refreshPrices)
    }
  })
</script>

<main class="craft-shell">
  <header style="--wails-draggable:drag">
    <img src="/emblem.png" alt="" /><strong>MrW Overlay · {t('craft.open')}</strong>
    <button title={t('window.close')} aria-label={t('window.close')} onclick={() => AppService.HideCraft()}>×</button>
  </header>
  {#if error}<p class="error" role="alert">{error}</p>{/if}
  <iframe bind:this={frame} src="/craft/index.html" title={t('craft.open')}></iframe>
</main>

<style>
  .craft-shell { height:100%; display:flex; flex-direction:column; border:1px solid var(--line-strong); background:var(--grain),var(--bg); }
  header { display:flex; align-items:center; gap:8px; padding:7px 10px; border-bottom:1px solid var(--line-strong); flex-shrink:0; }
  header img { width:24px; height:24px; }
  header strong { color:var(--gold-bright); font:500 14px var(--serif); flex:1; }
  header button { --wails-draggable:no-drag; padding:1px 8px; font-size:22px; }
  iframe { width:100%; flex:1; min-height:0; border:0; background:var(--bg); }
  .error { color:var(--bad); padding:8px; font-size:12px; }
</style>
