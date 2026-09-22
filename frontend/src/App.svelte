<script lang="ts">
  import { onMount } from 'svelte'
  import { Events } from '@wailsio/runtime'
  import { AppService, type Meta } from '../bindings/poe2filter'
  import type { Config } from '../bindings/poe2filter/internal/filter/models'
  import type { State } from '../bindings/poe2filter/internal/engine/models'
  import Toggle from './lib/Toggle.svelte'
  import Segmented from './lib/Segmented.svelte'
  import ListEditor from './lib/ListEditor.svelte'
  import StylePreview from './lib/StylePreview.svelte'
  import ThemePicker from './lib/ThemePicker.svelte'
  import TierSlider from './lib/TierSlider.svelte'
  import { lookOf, fromHex } from './lib/look'
  import type { CustomStyle, ItemGroup } from '../bindings/poe2filter/internal/filter/models'
  import type { StyleOptions } from '../bindings/poe2filter/models'
  import type { StyleGroup, Theme } from '../bindings/poe2filter/internal/filter/models'
  import { clock, relative, until, money, strictnessNames } from './lib/format'
  import { t, setLang } from './lib/i18n.svelte'
  import type { LanguageOption } from '../bindings/poe2filter/models'
  import type { ProfileInfo } from '../bindings/poe2filter/internal/engine/models'
  import type { State as AppUpdateState } from '../bindings/poe2filter/internal/appupdate/models'

  let meta = $state<Meta | null>(null)
  let cfg = $state<Config | null>(null)
  let st = $state<State | null>(null)
  let view = $state<'main' | 'settings'>('main')
  let now = $state(Date.now())
  // Settings changed since the filter was last written.
  let dirty = $state(false)
  let saveState = $state<'idle' | 'saving' | 'saved' | 'error'>('idle')
  let saveTimer: ReturnType<typeof setTimeout> | undefined
  let saveSeq = 0
  let actionError = $state('')
  let appUpdateActionError = $state('')
  let themes = $state<Theme[]>([])
  let nsThemes = $state<Theme[]>([])
  let styleOptions = $state<StyleOptions>({ colours: [], shapes: [], preset: {}, sounds: [] })
  let groups = $state<StyleGroup[]>([])
  let styleGroup = $state('divine')
  let sounds = $state<string[]>([])
  let soundError = $state('')
  let leagues = $state<string[]>([])
  let languages = $state<LanguageOption[]>([])
  let groupTemplate = $state<StyleGroup | null>(null)
  // Group waiting for a second click on Delete.
  let confirmDelete = $state('')
  // Result line under the share buttons: what the last export/import did.
  let shareMsg = $state('')
  let shareErr = $state('')
  let profiles = $state<ProfileInfo[]>([])
  let newProfile = $state('')
  let profileMsg = $state('')
  let profileErr = $state('')
  let confirmProfileDelete = $state(false)
  // Index the user is dragging a group from, and the card it hovers over.
  let dragFrom = $state<number | null>(null)
  let dragOver = $state<number | null>(null)
  let appUpdate = $state<AppUpdateState | null>(null)

  // "auto" resolves to whatever the first entry (the system language) reports.
  function languageOf(setting: string | undefined): string {
    if (!setting || setting === 'auto') return languages.find((l) => l.auto)?.id ?? 'auto'
    return setting
  }

  function applyLanguage(setting: string) {
    const lang = setting === 'auto' ? (languages.find((l) => l.auto)?.resolved ?? 'en') : setting
    setLang(lang)
    // CSS uppercase follows the document language: with lang="tr" the browser
    // turns "i" into "İ", which is wrong in the other two languages.
    document.documentElement.lang = lang
  }

  async function refresh() {
    st = await AppService.GetState()
    leagues = (await AppService.Leagues()) ?? leagues
  }

  onMount(() => {
    ;(async () => {
      meta = await AppService.GetMeta()
      appUpdate = await AppService.GetAppUpdateState()
      themes = (await AppService.Themes()) ?? []
      nsThemes = (await AppService.NeverSinkThemes()) ?? []
      styleOptions = (await AppService.StyleOptions()) ?? styleOptions
      groups = (await AppService.StyleGroups()) ?? []
      groupTemplate = await AppService.UserGroupTemplate()
      sounds = (await AppService.ListSounds()) ?? []
      leagues = (await AppService.Leagues()) ?? []
      languages = (await AppService.Languages()) ?? []
      profiles = (await AppService.Profiles()) ?? []
      cfg = await AppService.GetConfig()
      applyLanguage(cfg.language)
      await refresh()
    })()
    const off = Events.On('state', (ev) => {
      const prevRunning = st?.running
      st = ev.data
      if (prevRunning && !st.running && !st.lastError) {
        dirty = false
        AppService.NeverSinkThemes().then((t) => (nsThemes = t ?? []))
      }
    })
    const offAppUpdate = Events.On('app-update', (ev) => (appUpdate = ev.data))
    const tick = setInterval(() => (now = Date.now()), 1000)
    window.addEventListener('focus', refresh)
    return () => {
      off()
      offAppUpdate()
      clearInterval(tick)
      window.removeEventListener('focus', refresh)
    }
  })

  // Save shortly after the last change; the filter is rebuilt only on demand.
  function queueSave(affectsFilter = true) {
    if (affectsFilter) dirty = true
    saveState = 'saving'
    saveSeq++ // invalidates any reply still in flight
    clearTimeout(saveTimer)
    saveTimer = setTimeout(async () => {
      const seq = ++saveSeq
      try {
        const saved = await AppService.SaveConfig($state.snapshot(cfg) as Config)
        // Ignore stale replies: the user may have kept editing meanwhile.
        if (seq !== saveSeq) return
        const langChanged = languageOf(saved.language) !== languageOf(cfg?.language)
        cfg = saved
        if (langChanged) {
          applyLanguage(saved.language)
          // Group and theme names come from Go, so they need fetching again.
          themes = (await AppService.Themes()) ?? themes
          groups = (await AppService.StyleGroups()) ?? groups
          languages = (await AppService.Languages()) ?? languages
          await refresh()
        }
        saveState = 'saved'
        setTimeout(() => saveState === 'saved' && (saveState = 'idle'), 1500)
      } catch (e) {
        saveState = 'error'
      }
    }, 450)
  }

  const activeProfile = $derived(profiles.find((p) => p.active)?.name ?? '')

  // Applying a profile can change the league and the filter name, so say what
  // moved instead of letting the user discover it in game.
  function profileChanges(before: Config | null, after: Config): string {
    const notes: string[] = []
    if (before && after.filter_name !== before.filter_name) {
      notes.push(t('profile.filterNameChanged', after.filter_name))
    }
    if (before && after.league_name !== before.league_name) {
      notes.push(t('profile.leagueChanged', after.league_name))
    }
    return notes.join(' ')
  }

  async function afterProfileChange(saved: Config, before: Config | null, msg: string) {
    cfg = saved
    applyLanguage(saved.language)
    profiles = (await AppService.Profiles()) ?? profiles
    themes = (await AppService.Themes()) ?? themes
    groups = (await AppService.StyleGroups()) ?? groups
    sounds = (await AppService.ListSounds()) ?? sounds
    styleGroup = 'divine'
    dirty = false
    profileMsg = [msg, profileChanges(before, saved)].filter(Boolean).join(' ')
    await refresh()
  }

  async function switchProfile(name: string) {
    if (!name || name === activeProfile) return
    profileMsg = profileErr = ''
    const before = cfg ? ({ ...$state.snapshot(cfg) } as Config) : null
    try {
      await afterProfileChange(await AppService.SwitchProfile(name), before, t('profile.switched', name))
    } catch (e) {
      profileErr = String(e)
    }
  }

  async function saveProfileAs() {
    const name = newProfile.trim()
    if (!name) return
    profileMsg = profileErr = ''
    try {
      profiles = (await AppService.SaveProfileAs(name)) ?? profiles
      newProfile = ''
      profileMsg = t('profile.switched', name)
    } catch (e) {
      profileErr = String(e)
    }
  }

  async function renameProfile() {
    const name = newProfile.trim()
    if (!name || !activeProfile) return
    profileMsg = profileErr = ''
    try {
      profiles = (await AppService.RenameProfile(activeProfile, name)) ?? profiles
      newProfile = ''
      profileMsg = t('profile.renamed', name)
    } catch (e) {
      profileErr = String(e)
    }
  }

  async function deleteProfile() {
    profileMsg = profileErr = ''
    confirmProfileDelete = false
    const before = cfg ? ({ ...$state.snapshot(cfg) } as Config) : null
    try {
      const saved = await AppService.DeleteProfile(activeProfile)
      await afterProfileChange(saved, before, '')
    } catch (e) {
      profileErr = String(e)
    }
  }

  async function exportProfile() {
    profileMsg = profileErr = ''
    try {
      const path = await AppService.ExportProfile(activeProfile)
      if (path) profileMsg = t('profile.exported', path)
    } catch (e) {
      profileErr = String(e)
    }
  }

  async function importProfile() {
    profileMsg = profileErr = ''
    const before = cfg ? ({ ...$state.snapshot(cfg) } as Config) : null
    try {
      const name = await AppService.ImportProfile()
      if (!name) return
      await afterProfileChange(await AppService.GetConfig(), before, t('profile.imported', name))
    } catch (e) {
      profileErr = String(e)
    }
  }

  async function exportFilter() {
    shareMsg = shareErr = ''
    try {
      const path = await AppService.ExportFilter()
      if (path) shareMsg = t('share.exported', path)
    } catch (e) {
      shareErr = String(e)
    }
  }

  async function exportScan() {
    shareMsg = shareErr = ''
    try {
      const path = await AppService.ExportScan()
      if (path) shareMsg = t('share.exported', path)
    } catch (e) {
      shareErr = String(e)
    }
  }

  async function importScan() {
    shareMsg = shareErr = ''
    try {
      const res = await AppService.ImportScan()
      if (res) shareMsg = t('share.imported', res.added, res.updated, res.skipped)
    } catch (e) {
      shareErr = String(e)
    }
  }

  async function updateNow() {
    actionError = ''
    try {
      await AppService.UpdateNow()
    } catch (e) {
      actionError = String(e)
    }
  }

  async function checkForAppUpdate() {
    appUpdateActionError = ''
    try {
      appUpdate = await AppService.CheckForAppUpdate()
    } catch (e) {
      appUpdateActionError = String(e)
    }
  }

  async function downloadAppUpdate() {
    appUpdateActionError = ''
    try {
      appUpdate = await AppService.DownloadAppUpdate()
    } catch (e) {
      appUpdateActionError = String(e)
    }
  }

  async function installAppUpdate() {
    appUpdateActionError = ''
    try {
      await AppService.InstallAppUpdate()
    } catch (e) {
      appUpdateActionError = String(e)
    }
  }

  async function openAppUpdatePage() {
    appUpdateActionError = ''
    try {
      await AppService.OpenAppUpdatePage()
    } catch (e) {
      appUpdateActionError = String(e)
    }
  }

  const last = $derived(st?.last ?? null)

  // Renaming the filter starts writing a different file, but the game keeps
  // loading whatever is selected in its own options. That silence cost an
  // evening once: the app looked right and the game ignored it.
  const renamedFilter = $derived.by(() => {
    const path = last?.filterPath
    if (!path || !cfg?.filter_name) return ''
    const written = path.split(/[\\/]/).pop()?.replace(/\.filter$/i, '') ?? ''
    return written && written !== cfg.filter_name ? written : ''
  })
  const divineEx = $derived(last?.divineEx ?? 0)
  const chaosEx = $derived(last?.chaosEx ?? 0)

  // Live conversion of the threshold, shown under the input.
  const thresholdEx = $derived.by(() => {
    if (!cfg) return 0
    if (cfg.min_value_unit === 'divine') return cfg.min_value * divineEx
    if (cfg.min_value_unit === 'chaos') return cfg.min_value * chaosEx
    return cfg.min_value
  })

  function groupMode(g: ItemGroup): 'show' | 'hide' | 'value' {
    if (g.mode === 'show' || g.mode === 'hide' || g.mode === 'value') return g.mode
    return g.hide ? 'hide' : 'show'
  }

  function valueInEx(value: number, unit: string): number {
    if (unit === 'divine') return divineEx > 0 ? value * divineEx : 0
    if (unit === 'chaos') return chaosEx > 0 ? value * chaosEx : 0
    return value
  }

  function groupThresholdEx(g: ItemGroup): number {
    return valueInEx(g.threshold_value ?? 0, g.threshold_unit || 'exalted')
  }

  function groupThresholdTooLow(g: ItemGroup): boolean {
    const value = groupThresholdEx(g)
    return value > 0 && thresholdEx > 0 && value <= thresholdEx
  }

  function setGroupMode(index: number, mode: 'show' | 'hide' | 'value') {
    if (!cfg) return
    const g = cfg.item_groups![index]
    g.mode = mode
    g.hide = mode === 'hide'
    if (mode !== 'show') g.always = false
    if (mode === 'value' && !((g.threshold_value ?? 0) > 0)) {
      g.threshold_value = Math.max(1, cfg.min_value * 2)
      g.threshold_unit = cfg.min_value_unit
    }
    queueSave()
  }

  const status = $derived.by(() => {
    if (!st) return { tone: 'idle', title: t('status.starting'), sub: '' }
    if (st.running) return { tone: 'busy', title: t('status.updating'), sub: st.step }
    if (st.lastError) return { tone: 'bad', title: t('status.failed'), sub: st.lastError }
    if (st.lastRunAtMs) return { tone: 'ok', title: t('status.fresh'), sub: t('status.lastUpdate', relative(st.lastRunAtMs, now)) }
    return { tone: 'idle', title: t('status.never'), sub: '' }
  })

  // The configured league is always offered, even if the live list lost it.
  const leagueOptions = $derived.by(() => {
    const list = leagues.length ? [...leagues] : []
    const current = cfg?.league_name ?? ''
    if (current && !list.some((l) => l.toLowerCase() === current.toLowerCase())) list.push(current)
    return list
  })
  const leagueUnlisted = $derived(
    !!cfg?.league_name &&
      leagues.length > 0 &&
      !leagues.some((l) => l.toLowerCase() === cfg!.league_name.toLowerCase()),
  )

  // The Appearance tabs cover the built-in groups and every user group, which
  // all share one look template.
  const allGroups = $derived.by<StyleGroup[]>(() => {
    if (!groupTemplate || !cfg?.item_groups?.length) return groups
    // A hidden group draws nothing, so it has no colours to pick.
    const own = cfg.item_groups!
      .filter((g) => groupMode(g) !== 'hide')
      .map((g) => ({ ...groupTemplate!, id: 'user:' + g.id, label: g.name }) as StyleGroup)
    return [...groups, ...own]
  })

  const selGroup = $derived(allGroups.find((g) => g.id === styleGroup))

  function addGroup() {
    if (!cfg || (cfg.item_groups ?? []).length >= (meta?.maxItemGroups ?? 12)) return
    cfg.item_groups = [
      ...(cfg.item_groups ?? []),
      {
        id: '',
        name: '',
        items: [],
        mode: 'show',
        hide: false,
        always: false,
        threshold_value: 0,
        threshold_unit: 'exalted',
      } as ItemGroup,
    ]
    queueSave()
  }

  // Jump to the group's colours: the picker lives in the Appearance section.
  function openLook(id: string) {
    styleGroup = 'user:' + id
    requestAnimationFrame(() => document.querySelector('#appearance')?.scrollIntoView({ block: 'start', behavior: 'smooth' }))
  }

  // Order decides which group's rules reach the filter first, so moving a card
  // is a real setting, not decoration.
  function moveGroup(from: number, to: number) {
    if (!cfg) return
    const list = [...(cfg.item_groups ?? [])]
    if (from < 0 || to < 0 || from >= list.length || to >= list.length || from === to) return
    const [g] = list.splice(from, 1)
    list.splice(to, 0, g)
    cfg.item_groups = list
    queueSave()
  }

  function onGripKey(e: KeyboardEvent, i: number) {
    if (e.key !== 'ArrowUp' && e.key !== 'ArrowDown') return
    e.preventDefault()
    moveGroup(i, i + (e.key === 'ArrowUp' ? -1 : 1))
  }

  // The same item in two lists is legal but confusing: the earlier rule wins
  // and the later one silently does nothing, so say where the clash is.
  function duplicatesOf(index: number): string {
    const groupsList = cfg?.item_groups ?? []
    if (groupMode(groupsList[index]) === 'value') return ''
    const key = (v: string) => v.split('|')[0].trim().toLowerCase()
    const mine = new Set(groupsList[index]?.items?.map(key) ?? [])
    const hits: string[] = []
    const scan = (items: string[] | null, where: string) => {
      for (const it of items ?? []) {
        if (mine.has(key(it))) hits.push(`${it} (${where})`)
      }
    }
    groupsList.forEach((g, i) => i !== index && groupMode(g) !== 'value' && scan(g.items, g.name || t('groups.title')))
    scan(cfg?.whitelist ?? [], t('lists.showTop'))
    scan(cfg?.chance_bases ?? [], t('lists.chance'))
    return hits.join(', ')
  }

  function removeGroup(id: string) {
    if (!cfg) return
    confirmDelete = ''
    cfg.item_groups = (cfg.item_groups ?? []).filter((g) => g.id !== id)
    if (styleGroup === 'user:' + id) styleGroup = 'divine'
    queueSave()
  }

  // Groups without a built-in look (Divine Orb) default to their first theme.
  function styleValue(g: StyleGroup): string {
    const v = cfg?.styles?.[g.id]
    if (v) return v
    return g.allowDefault ? 'default' : g.default.id
  }

  function paletteOf(g: StyleGroup): Theme {
    const v = cfg?.styles?.[g.id] ?? ''
    const cs = cfg?.custom_styles?.[g.id]
    if (v === 'custom' && cs) {
      return { id: 'custom', label: t('picker.custom'), full: true, bg: fromHex(cs.bg), text: fromHex(cs.text),
        border: fromHex(cs.border), beam: cs.beam, icon: cs.icon, shape: cs.shape } as Theme
    }
    const pool = v.startsWith('ns:') ? nsThemes : themes
    return pool.find((t) => t.id === v) ?? g.default
  }

  function setCustom(group: string, cs: CustomStyle) {
    if (!cfg) return
    cfg.custom_styles = { ...(cfg.custom_styles ?? {}), [group]: cs }
    cfg.styles = { ...(cfg.styles ?? {}), [group]: 'custom' }
    queueSave()
  }

  // Every group to the NeverSink style closest in spirit (when present).
  function applyNeverSink() {
    if (!cfg) return
    const next = { ...(cfg.styles ?? {}) }
    for (const g of allGroups) {
      const id = 'ns:' + (styleOptions.preset?.[g.id] ?? '')
      if (nsThemes.some((t) => t.id === id)) next[g.id] = id
    }
    cfg.styles = next
    queueSave()
  }

  function resetStyles() {
    if (!cfg) return
    cfg.styles = {}
    queueSave()
  }

  function customised(g: StyleGroup): boolean {
    const v = styleValue(g)
    return g.allowDefault ? v !== 'default' : v !== g.default.id
  }

  function setSound(group: string, v: string) {
    if (!cfg) return
    const next = { ...(cfg.sounds ?? {}) }
    if (v) next[group] = v
    else delete next[group]
    cfg.sounds = next
    soundError = ''
    queueSave()
  }

  function soundFile(g: StyleGroup): string {
    const v = cfg?.sounds?.[g.id] ?? ''
    return v.startsWith('file:') ? v.slice(5) : ''
  }

  // A game sound belongs to the game itself; the app can only play a copy of
  // it, and only once the user has downloaded one.
  function soundIsGame(g: StyleGroup): boolean {
    const v = cfg?.sounds?.[g.id] ?? ''
    if (v === 'none' || v.startsWith('file:')) return false
    return v !== '' || !!g.defaultSound
  }

  function soundLabel(g: StyleGroup): string {
    const v = cfg?.sounds?.[g.id] ?? ''
    if (v === 'none') return t('look.soundDefaultSilent')
    if (v.startsWith('file:')) return v.slice(5)
    if (v) return t('look.soundDefaultGame', soundName(v))
    return g.defaultSound
      ? t('look.soundDefaultGame', soundName(g.defaultSound))
      : t('look.soundDefaultSilent')
  }

  // "ShDivine" means nothing to a reader; "24 · Divine Orb" does.
  function soundName(id: string): string {
    const i = (styleOptions.sounds ?? []).indexOf(id)
    const name = soundCurrency[id]
    if (!name) return id
    return `${i >= 0 ? i + 1 : id} · ${name}`
  }

  // The game sound a group ends up with, '' when it plays a file or nothing.
  function soundGameID(g: StyleGroup): string {
    const v = cfg?.sounds?.[g.id] ?? ''
    if (v === 'none' || v.startsWith('file:')) return ''
    return v || g.defaultSound || ''
  }

  async function previewSound(g: StyleGroup) {
    soundError = ''
    try {
      const id = soundGameID(g)
      if (soundFile(g)) await AppService.PreviewSound(soundFile(g))
      else if (id) await AppService.PreviewGameSound(id)
    } catch (e) {
      soundError = String(e)
    }
  }

  // Sounds 17-26 are named after the currency whose drop they announce; the
  // filter spells them "ShAlchemy" and so on, but nobody thinks of them that
  // way, so the menu shows the number and the currency.
  const soundCurrency: Record<string, string> = {
    ShAlchemy: 'Orb of Alchemy',
    ShBlessed: 'Blessed Orb',
    ShChaos: 'Chaos Orb',
    ShFusing: 'Orb of Fusing',
    ShGeneral: 'Orb of immense power',
    ShRegal: 'Regal Orb',
    ShVaal: 'Vaal Orb',
    ShDivine: 'Divine Orb',
    ShExalted: 'Exalted Orb',
    ShMirror: 'Mirror of Kalandra',
  }

  // The list comes from Go so the menu can never offer something the filter
  // writer would refuse.
  const gameSoundIDs = $derived(styleOptions.sounds ?? [])

  function gameSoundLabel(id: string, i: number): string {
    const name = soundCurrency[id]
    return t('look.soundGame', name ? `${i + 1} · ${name}` : id)
  }

  async function addSound() {
    soundError = ''
    try {
      const name = await AppService.AddSound()
      if (!name) return // dialog cancelled
      sounds = (await AppService.ListSounds()) ?? sounds
      if (selGroup) setSound(selGroup.id, 'file:' + name)
    } catch (e) {
      soundError = String(e)
    }
  }

  function setStyle(group: string, id: string) {
    if (!cfg) return
    cfg.styles = { ...(cfg.styles ?? {}), [group]: id }
    queueSave()
  }

  const scanPct = $derived(st && st.scan.keys ? Math.min(1, st.scan.scanned / st.scan.keys) : 0)
</script>

<main>
  <header>
    <img src="/emblem.png" alt="" class="emblem" />
    {#if view === 'main'}
      <div class="brand">
        <h1 lang="en">{t('app.title')}</h1>
        {#if cfg}<span class="league">{cfg.league_name}</span>{/if}
      </div>
      <button class="icon" title={t('header.settings')} aria-label={t('header.settings')} onclick={() => (view = 'settings')}>
        <svg viewBox="0 0 24 24"><path d="M12 15.5a3.5 3.5 0 1 0 0-7 3.5 3.5 0 0 0 0 7Z" /><path d="M19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.8.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.8V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1Z" /></svg>
      </button>
    {:else}
      <div class="brand">
        <h1>{t('header.settings')}</h1>
        <span class="save {saveState}">
          {#if saveState === 'saving'}{t('save.saving')}{:else if saveState === 'saved'}{t('save.saved')}{:else if saveState === 'error'}{t('save.error')}{/if}
        </span>
      </div>
      <button class="icon" title={t('header.back')} aria-label={t('header.back')} onclick={() => (view = 'main')}>
        <svg viewBox="0 0 24 24"><path d="M15 18l-6-6 6-6" /></svg>
      </button>
    {/if}
    <button class="icon" title={t('header.hide')} aria-label={t('header.hide')} onclick={() => AppService.HidePanel()}>
      <svg viewBox="0 0 24 24"><path d="M6 6l12 12M18 6L6 18" /></svg>
    </button>
  </header>

  {#if cfg && view === 'main'}
    <div class="scroll">
      <!-- Status -->
      <section class="card status {status.tone}">
        <div class="status-head">
          <span class="dot"></span>
          <div class="status-text">
            <strong>{status.title}</strong>
            {#if status.sub}<span class="sub" title={status.sub}>{status.sub}</span>{/if}
          </div>
        </div>
        {#if st?.running}
          <div class="bar"><span style="width: {Math.round((st.progress || 0.05) * 100)}%"></span></div>
        {/if}
        {#if dirty && !st?.running}
          <p class="notice">{t('status.dirty')}</p>
        {/if}
        {#if st?.lastError && !st.running}
          <p class="notice bad">
            {t(
              'status.writeFailed',
              st.lastOkAtMs ? t('status.fileFrom', relative(st.lastOkAtMs, now)) : t('status.fileNever'),
            )}
          </p>
        {/if}
        <button class="primary" class:pulse={dirty} disabled={st?.running} onclick={updateNow}>
          {st?.running ? t('button.updating') : st?.lastError ? t('button.retry') : t('button.updateNow')}
        </button>
        <div class="meta-row">
          {#if st?.running}
            <span>&nbsp;</span>
          {:else if st?.nextRetryAtMs}
            <span>{t('meta.autoRetry', clock(st.nextRetryAtMs))} <em>({until(st.nextRetryAtMs, now)})</em></span>
          {:else if st?.nextRunAtMs}
            <span>{t('meta.next', clock(st.nextRunAtMs))} <em>({until(st.nextRunAtMs, now)})</em></span>
          {:else if !cfg.auto_update_enabled}
            <span>{t('meta.autoOff')}</span>
          {/if}
          <span class="reload">{t('meta.reload')}</span>
        </div>
        {#if actionError}<p class="error">{actionError}</p>{/if}
      </section>

      <!-- Threshold -->
      <section class="card">
        <div class="card-title">
          <h2>{t('threshold.title')}</h2>
          {#if divineEx && cfg.min_value_unit !== 'exalted'}<span class="aside num">≈ {money(thresholdEx, 0)}</span>{/if}
        </div>
        <p class="desc">
          {t(cfg.filter_mode === 'dim' ? 'threshold.dim' : cfg.filter_mode === 'show_only' ? 'threshold.show' : 'threshold.hide')}
        </p>
        <div class="threshold">
          <input
            type="number"
            class="num"
            min="0"
            step={cfg.min_value_unit === 'divine' ? 0.1 : 1}
            bind:value={cfg.min_value}
            oninput={() => queueSave()}
          />
          <Segmented
            small
            bind:value={cfg.min_value_unit}
            onchange={() => queueSave()}
            options={[
              { value: 'exalted', label: 'Exalted' },
              { value: 'chaos', label: 'Chaos' },
              { value: 'divine', label: 'Divine' },
            ]}
          />
        </div>
        <div class="chips">
          {#each cfg.min_value_unit === 'divine' ? [0.1, 0.5, 1, 5] : [5, 10, 50, 100] as v}
            <button class:on={cfg.min_value === v} onclick={() => { cfg!.min_value = v; queueSave() }}>{v}</button>
          {/each}
        </div>
      </section>

      <!-- Strictness + mode -->
      <section class="card">
        <div class="card-title">
          <h2>{t('base.title')}</h2>
          <span class="aside gold">{strictnessNames[cfg.strictness]}</span>
        </div>
        <input
          type="range"
          class="strict"
          min="0"
          max="6"
          step="1"
          bind:value={cfg.strictness}
          onchange={() => queueSave()}
          style="--p: {(cfg.strictness / 6) * 100}%"
        />
        <div class="scale"><span>{t('base.soft')}</span><span>{t('base.strict')}</span><span>{t('base.uber')}</span></div>

        <h2 class="sub-title">{t('mode.title')}</h2>
        <Segmented
          bind:value={cfg.filter_mode}
          onchange={() => queueSave()}
          options={[
            { value: 'hide', label: t('mode.hide') },
            { value: 'dim', label: t('mode.dim') },
            { value: 'show_only', label: t('mode.show') },
          ]}
        />
      </section>

      <!-- Exceptional scan -->
      <section class="card">
        <Toggle
          bind:checked={cfg.exceptional_scan}
          label={t('scan.toggle')}
          hint={t('scan.hint')}
          onchange={() => queueSave(false)}
        />
        {#if st && cfg.exceptional_scan}
          <div class="bar thin"><span class="cyan" style="width: {scanPct * 100}%"></span></div>
          <div class="scan-line num">
            <span>{t('scan.scanned', st.scan.scanned, st.scan.keys || '—')}</span>
            <span class="cyan-text">{t('scan.valuable', st.scan.valuable)}</span>
          </div>
          {#if st.scan.current}
            <div class="scan-line muted">
              <span class="ellipsis">{t('scan.next', st.scan.current)}</span>
              <span class="num">{st.scan.nextAtMs > now + 1000 ? until(st.scan.nextAtMs, now) : t('scan.searching')}</span>
            </div>
          {/if}
          {#if st.scan.last}
            <div class="scan-line muted"><span class="ellipsis">{t('scan.last', st.scan.last)}</span></div>
          {/if}
          {#if st.scan.etaSec > 0}
            <p class="scan-note">
              {t(
                'scan.note',
                Math.round(st.scan.etaSec / Math.max(1, st.scan.keys - st.scan.scanned)),
                until(now + st.scan.etaSec * 1000, now),
              )}
            </p>
          {/if}
        {/if}
      </section>

      <!-- Summary -->
      {#if last}
        <section class="stats">
          <div><strong class="num">{last.valuableCurrency}</strong><span>{t('stats.currency')}</span></div>
          <div><strong class="num">{last.valuableUniques}</strong><span>{t('stats.uniqueBases')}</span></div>
          <div><strong class="num cyan-text">{last.valuableExcept}</strong><span>{t('stats.exceptional')}</span></div>
        </section>
        <p class="stats-caption">{t('stats.caption', Math.round(divineEx))}</p>
      {/if}
    </div>
  {:else if cfg && view === 'settings'}
    <div class="scroll settings">
      <section class="card">
        <h2>{t('profile.title')}</h2>
        <p class="desc">{t('profile.desc')}</p>
        <label class="field">
          <span>{t('profile.title')}</span>
          <select value={activeProfile} onchange={(e) => switchProfile(e.currentTarget.value)}>
            {#each profiles as p (p.name)}
              <option value={p.name}>{p.name}</option>
            {/each}
          </select>
        </label>
        <div class="group-head">
          <input
            class="group-name"
            bind:value={newProfile}
            placeholder={t('profile.namePlaceholder')}
            onkeydown={(e) => e.key === 'Enter' && saveProfileAs()}
            spellcheck="false"
          />
          <button type="button" class="group-del" onclick={saveProfileAs} disabled={!newProfile.trim()}>
            {t('profile.saveAs')}
          </button>
          <button type="button" class="group-del" onclick={renameProfile} disabled={!newProfile.trim()}>
            {t('profile.rename')}
          </button>
        </div>
        <div class="presets">
          <button type="button" onclick={exportProfile}>{t('profile.export')}</button>
          <button type="button" onclick={importProfile}>{t('profile.import')}</button>
          <button
            type="button"
            class:confirm={confirmProfileDelete}
            disabled={profiles.length < 2}
            onclick={() => (confirmProfileDelete ? deleteProfile() : (confirmProfileDelete = true))}
            onblur={() => (confirmProfileDelete = false)}
          >
            {confirmProfileDelete ? t('profile.deleteConfirm') : t('profile.delete')}
          </button>
        </div>
        <p class="desc hint">{t('profile.everything')}</p>
        {#if profileMsg}<p class="desc warn">{profileMsg}</p>{/if}
        {#if profileErr}<p class="error">{profileErr}</p>{/if}
      </section>

      <section class="card">
        <h2>{t('gear.title')}</h2>
        <Toggle bind:checked={cfg.include_gear} label={t('gear.strict')} hint={t('gear.strictHint')} onchange={() => queueSave()} />
        <TierSlider
          bind:value={cfg.t5_rare_tier}
          min={0}
          max={5}
          label={t('tier.t5rare')}
          hint={t('tier.t5rareHint')}
          onchange={() => queueSave()}
        />
        <TierSlider
          bind:value={cfg.rare_jewel_tier}
          min={0}
          max={5}
          label={t('tier.jewels')}
          hint={t('tier.jewelsHint')}
          onchange={() => queueSave()}
        />
        <label class="field">
          <span>{t('gear.quality')}</span>
          <select bind:value={cfg.quality_threshold} onchange={() => queueSave()}>
            <option value={0}>{t('gear.qualityOff')}</option>
            <option value={15}>%15+</option>
            <option value={20}>%20+</option>
          </select>
        </label>
      </section>

      <section class="card">
        <h2>{t('rules.title')}</h2>
        <TierSlider
          bind:value={cfg.waystone_tier}
          min={1}
          max={15}
          prefix="T"
          label={t('tier.waystones')}
          hint={t('tier.waystonesHint')}
          onchange={() => queueSave()}
        />
        <TierSlider
          bind:value={cfg.uncut_gem_level}
          min={1}
          max={20}
          label={t('tier.uncut')}
          hint={t('tier.uncutHint')}
          onchange={() => queueSave()}
        />
        <TierSlider
          bind:value={cfg.uncut_support_level}
          min={1}
          max={5}
          label={t('tier.support')}
          hint={t('tier.supportHint')}
          onchange={() => queueSave()}
        />
        <Toggle bind:checked={cfg.boss_keys_and_tablets} label={t('rules.pinnacle')} onchange={() => queueSave()} />
        <Toggle bind:checked={cfg.hide_exalt} label={t('rules.hideExalt')} onchange={() => queueSave()} />
        <Toggle bind:checked={cfg.hide_gold} label={t('rules.hideGold')} onchange={() => queueSave()} />
      </section>

      <section class="card">
        <h2>{t('lists.title')}</h2>
        <h3>{t('lists.showTop')} <span class="h3-note">{t('lists.showTopNote')}</span></h3>
        <p class="desc">{t('lists.showTopDesc')}</p>
        <ListEditor bind:items={cfg.whitelist} uniqueVariants placeholder={t('lists.searchItem')} onchange={() => queueSave()} />
        <h3>{t('lists.chance')} <span class="h3-note">{t('lists.chanceNote')}</span></h3>
        <ListEditor bind:items={cfg.chance_bases} placeholder={t('lists.searchBase')} onchange={() => queueSave()} />
      </section>

      <section class="card">
        <h2>{t('groups.title')}</h2>
        <p class="desc">{t('groups.desc')}</p>
        <p class="desc hint">{t('groups.order')}</p>
        {#each cfg.item_groups ?? [] as g, i (g.id || i)}
          <div
            class="group"
            class:drag-over={dragOver === i && dragFrom !== i}
            ondragover={(e) => {
              if (dragFrom === null) return
              e.preventDefault()
              dragOver = i
            }}
            ondrop={(e) => {
              e.preventDefault()
              if (dragFrom !== null) moveGroup(dragFrom, i)
              dragFrom = dragOver = null
            }}
            role="listitem"
          >
            <div class="group-head">
              <button
                type="button"
                class="grip"
                draggable="true"
                aria-label={t('groups.reorder')}
                title={t('groups.reorder')}
                ondragstart={() => (dragFrom = i)}
                ondragend={() => (dragFrom = dragOver = null)}
                onkeydown={(e) => onGripKey(e, i)}
              >
                <svg viewBox="0 0 24 24"><path d="M9 6h.01M9 12h.01M9 18h.01M15 6h.01M15 12h.01M15 18h.01" /></svg>
              </button>
              <input
                class="group-name"
                bind:value={cfg.item_groups![i].name}
                placeholder={t('groups.namePlaceholder')}
                onchange={() => queueSave()}
                spellcheck="false"
              />
              <button
                type="button"
                class="group-del"
                class:confirm={confirmDelete === g.id}
                onclick={() => (confirmDelete === g.id ? removeGroup(g.id) : (confirmDelete = g.id))}
                onblur={() => (confirmDelete = '')}
              >
                {confirmDelete === g.id ? t('groups.deleteConfirm') : t('groups.delete')}
              </button>
            </div>
            <Segmented
              small
              value={groupMode(g)}
              onchange={(v) => setGroupMode(i, v as 'show' | 'hide' | 'value')}
              options={[
                { value: 'show', label: t('groups.modeShow') },
                { value: 'hide', label: t('groups.modeHide') },
                { value: 'value', label: t('groups.modeValue') },
              ]}
            />
            {#if groupMode(g) === 'value'}
              <p class="desc">{t('groups.valueDesc')}</p>
              <div class="threshold group-threshold">
                <input
                  type="number"
                  class="num"
                  min="0"
                  step={g.threshold_unit === 'divine' ? 0.1 : 1}
                  bind:value={cfg.item_groups![i].threshold_value}
                  oninput={() => queueSave()}
                  aria-label={t('groups.valueAmount')}
                />
                <Segmented
                  small
                  value={g.threshold_unit || 'exalted'}
                  onchange={(v) => {
                    cfg!.item_groups![i].threshold_unit = v
                    queueSave()
                  }}
                  options={[
                    { value: 'exalted', label: 'Exalted' },
                    { value: 'chaos', label: 'Chaos' },
                    { value: 'divine', label: 'Divine' },
                  ]}
                />
              </div>
              {#if groupThresholdEx(g) > 0}
                <p class="desc hint" class:warn={groupThresholdTooLow(g)}>
                  {groupThresholdTooLow(g)
                    ? t('groups.valueTooLow', money(groupThresholdEx(g), 0), money(thresholdEx, 0))
                    : t('groups.valueEquivalent', money(groupThresholdEx(g), 0))}
                </p>
              {/if}
            {:else}
              {#if groupMode(g) === 'show'}
                <Toggle
                  bind:checked={cfg.item_groups![i].always}
                  label={t('groups.always')}
                  hint={t('groups.alwaysHint')}
                  onchange={() => queueSave()}
                />
              {/if}
              <ListEditor
                bind:items={cfg.item_groups![i].items}
                uniqueVariants={groupMode(g) === 'show'}
                placeholder={groupMode(g) === 'hide' ? t('lists.searchHide') : t('lists.searchItem')}
                onchange={() => queueSave()}
              />
              {#if duplicatesOf(i)}
                <p class="desc warn">{t('groups.duplicates', duplicatesOf(i))}</p>
              {/if}
            {/if}
            {#if groupMode(g) !== 'hide' && g.id}
              <button type="button" class="look-link" onclick={() => openLook(g.id)}>
                {t('groups.lookLink', g.name)}
              </button>
            {/if}
          </div>
        {:else}
          <p class="desc hint">{t('groups.empty')}</p>
        {/each}
        <div class="presets">
          <button type="button" onclick={addGroup} disabled={(cfg.item_groups ?? []).length >= (meta?.maxItemGroups ?? 12)}>
            {t('groups.add')}
          </button>
        </div>
        {#if (cfg.item_groups ?? []).length >= (meta?.maxItemGroups ?? 12)}
          <p class="desc hint">{t('groups.limit', meta?.maxItemGroups ?? 12)}</p>
        {/if}
      </section>

      <section class="card" id="appearance">
        <h2>{t('look.title')}</h2>
        <p class="desc">{t('look.desc')}</p>
        <div class="presets">
          <button type="button" onclick={applyNeverSink} disabled={!nsThemes.length}>{t('look.applyNeverSink')}</button>
          <button type="button" onclick={resetStyles}>{t('look.reset')}</button>
        </div>
        <div class="groups" role="tablist">
          {#each allGroups as g (g.id)}
            <button type="button" role="tab" aria-selected={g.id === styleGroup} class:on={g.id === styleGroup} onclick={() => (styleGroup = g.id)}>
              <span lang={g.id === 'divine' ? 'en' : undefined}>{g.label}</span>
              {#if customised(g)}<i class="custom-dot" title={t('look.customised')}></i>{/if}
            </button>
          {/each}
        </div>
        {#if selGroup}
          <StylePreview group={selGroup} look={lookOf(selGroup, paletteOf(selGroup))} sound={soundLabel(selGroup)} />
          <div class="picker">
            <ThemePicker
              group={selGroup}
              value={styleValue(selGroup)}
              {themes}
              {nsThemes}
              custom={cfg.custom_styles?.[selGroup.id]}
              colours={styleOptions.colours ?? []}
              shapes={styleOptions.shapes ?? []}
              current={lookOf(selGroup, paletteOf(selGroup))}
              onselect={(id) => setStyle(selGroup.id, id)}
              oncustom={(cs) => setCustom(selGroup.id, cs)}
            />
          </div>
          <label class="field">
            <span>{t('look.sound')}</span>
            <span class="sound-row">
              <select value={cfg.sounds?.[selGroup.id] ?? ''} onchange={(e) => setSound(selGroup.id, e.currentTarget.value)}>
                <option value="">
                  {t(
                    'look.soundDefault',
                    selGroup.defaultSound ? t('look.soundDefaultGame', selGroup.defaultSound) : t('look.soundDefaultSilent'),
                  )}
                </option>
                <option value="none">{t('look.soundNone')}</option>
                {#each gameSoundIDs as id, i}
                  <option value={id}>{gameSoundLabel(id, i)}</option>
                {/each}
                {#each sounds as f (f)}
                  <option value={'file:' + f}>{f}</option>
                {/each}
              </select>
              {#if soundFile(selGroup) || soundGameID(selGroup)}
                <button
                  type="button"
                  class="play"
                  title={t('look.play')}
                  aria-label={t('look.playAria')}
                  onclick={() => previewSound(selGroup)}
                >
                  <svg viewBox="0 0 24 24"><path d="M8 5v14l11-7z" /></svg>
                </button>
              {/if}
            </span>
          </label>
          {#if soundIsGame(selGroup)}
            <p class="desc hint">{t('look.gameSoundNote')}</p>
          {/if}
          <button type="button" class="ghost" onclick={addSound}>{t('look.addSound')}</button>
          {#if soundError}<p class="error">{soundError}</p>{/if}
          <p class="desc hint">{t('look.soundHint')}</p>
        {/if}
      </section>

      <section class="card">
        <h2>{t('auto.title')}</h2>
        <Toggle bind:checked={cfg.auto_update_enabled} label={t('auto.enable')} onchange={() => queueSave(false)} />
        {#if cfg.auto_update_enabled}
          <Segmented
            small
            bind:value={cfg.auto_update_hours}
            onchange={() => queueSave(false)}
            options={[1, 2, 4, 6, 12].map((h) => ({ value: h, label: t('auto.hours', h) }))}
          />
        {/if}
        <Toggle bind:checked={cfg.notify_enabled} label={t('auto.notify')} onchange={() => queueSave(false)} />
      </section>

      {#if appUpdate?.status !== 'disabled'}
        <section class="card">
          <h2>{t('appUpdate.title')}</h2>
          <p class="desc">{t('appUpdate.desc')}</p>
          {#if appUpdate?.status === 'checking'}
            <p class="desc hint">{t('appUpdate.checking')}</p>
          {:else if appUpdate?.status === 'downloading'}
            <p class="desc hint">{t('appUpdate.downloading', appUpdate.progress ?? 0)}</p>
          {:else if appUpdate?.status === 'available'}
            <p class="notice">{t('appUpdate.available', appUpdate.latestVersion ?? '')}</p>
            {#if !appUpdate.canInstall}<p class="desc warn">{t('appUpdate.noPermission')}</p>{/if}
          {:else if appUpdate?.status === 'ready'}
            <p class="notice">{t('appUpdate.ready', appUpdate.latestVersion ?? '')}</p>
            <p class="desc hint">{t('appUpdate.installNote')}</p>
          {:else if appUpdate?.status === 'up_to_date'}
            <p class="desc hint">{t('appUpdate.upToDate', appUpdate.currentVersion)}</p>
          {:else if appUpdate?.status === 'error'}
            <p class="error">{t('appUpdate.error')} {appUpdate.error}</p>
          {/if}
          {#if appUpdate?.error && appUpdate.status !== 'error'}
            <p class="error">{t('appUpdate.error')} {appUpdate.error}</p>
          {/if}
          {#if appUpdateActionError}<p class="error">{appUpdateActionError}</p>{/if}
          <div class="presets">
            {#if appUpdate?.status === 'available' && appUpdate.canInstall}
              <button type="button" onclick={downloadAppUpdate}>{t('appUpdate.download')}</button>
            {/if}
            {#if appUpdate?.status === 'ready'}
              <button type="button" class="primary" onclick={installAppUpdate}>{t('appUpdate.install')}</button>
            {/if}
            {#if appUpdate?.releaseUrl}
              <button type="button" onclick={openAppUpdatePage}>{t('appUpdate.release')}</button>
            {/if}
            {#if appUpdate?.status !== 'checking' && appUpdate?.status !== 'downloading' && appUpdate?.status !== 'installing'}
              <button type="button" onclick={checkForAppUpdate}>{t('appUpdate.check')}</button>
            {/if}
          </div>
        </section>
      {/if}

      <section class="card">
        <h2>{t('trade.title')}</h2>
        <p class="desc">{t('trade.desc')}</p>
        <Segmented
          small
          bind:value={cfg.scan_budget_pct}
          onchange={() => queueSave(false)}
          options={[20, 40, 60].map((p) => ({ value: p, label: t('trade.budget', p) }))}
        />
        <h3>{t('share.title')}</h3>
        <p class="desc">{t('share.desc')}</p>
        <div class="presets">
          <button type="button" onclick={exportScan}>{t('share.export')}</button>
          <button type="button" onclick={importScan}>{t('share.import')}</button>
        </div>
        {#if shareMsg}<p class="desc hint ellipsis" title={shareMsg}>{shareMsg}</p>{/if}
        {#if shareErr}<p class="error">{shareErr}</p>{/if}
      </section>

      <section class="card">
        <h2>{t('general.title')}</h2>
        <label class="field">
          <span>{t('general.language')}</span>
          <select bind:value={cfg.language} onchange={() => queueSave(false)}>
            {#each languages as l (l.id)}
              <option value={l.id}>{l.auto ? t('general.languageAuto', l.label) : l.label}</option>
            {/each}
          </select>
        </label>
        <label class="field">
          <span>{t('general.league')}</span>
          <select bind:value={cfg.league_name} onchange={() => queueSave()}>
            {#each leagueOptions as l (l)}
              <option value={l}>{l}</option>
            {/each}
          </select>
        </label>
        {#if leagueUnlisted}
          <p class="desc hint">{t('general.leagueUnlisted')}</p>
        {/if}
        <label class="field stack">
          <span>{t('general.filterName')}</span>
          <input bind:value={cfg.filter_name} onchange={() => queueSave()} spellcheck="false" />
        </label>
        {#if renamedFilter}
          <p class="notice">{t('general.filterNameChanged', cfg.filter_name, renamedFilter)}</p>
        {/if}
        <div class="presets">
          <button type="button" onclick={exportFilter}>{t('filter.export')}</button>
        </div>
        <p class="desc hint">{t('filter.exportHint')}</p>
        <label class="field stack">
          <span>{t('general.customBase')}</span>
          <input bind:value={cfg.custom_base_filter} onchange={() => queueSave()} placeholder={t('general.customBasePlaceholder')} spellcheck="false" />
        </label>
        <label class="field stack">
          <span>{t('general.priceServer')}</span>
          <input bind:value={cfg.price_source_url} onchange={() => queueSave(false)} placeholder="https://…/prices.json" spellcheck="false" />
        </label>
      </section>

      <section class="actions">
        <button onclick={() => AppService.OpenGameFolder()}>{t('actions.filterFolder')}</button>
        <button onclick={() => AppService.OpenDataFolder()}>{t('actions.dataFolder')}</button>
        <button class="danger" onclick={() => AppService.Quit()}>{t('actions.quit')}</button>
      </section>
      {#if meta}<p class="version">v{meta.version}{meta.testMode ? t('footer.testMode') : ''}</p>{/if}
    </div>
  {/if}
</main>

<style>
  main {
    height: 100%;
    display: flex;
    flex-direction: column;
    /* Grain over stone, with the light falling from above, the way the game's
       own panels are lit. No coloured glow: that is what made this look like a
       landing page rather than something from the game. */
    background:
      linear-gradient(180deg, rgba(255, 255, 255, 0.03), transparent 260px),
      var(--grain),
      var(--bg);
    border: 1px solid var(--line-strong);
    border-radius: var(--radius);
    box-shadow:
      inset 0 0 0 1px #000,
      inset 0 0 40px rgba(0, 0, 0, 0.7);
    overflow: hidden;
  }

  header {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 12px 10px 10px 14px;
    border-bottom: 1px solid var(--gold-dim);
    --wails-draggable: drag;
  }
  header button {
    --wails-draggable: no-drag;
  }
  .emblem {
    width: 28px;
    height: 28px;
  }
  .brand {
    flex: 1;
    display: flex;
    align-items: baseline;
    gap: 8px;
    min-width: 0;
  }
  h1 {
    margin: 0;
    font-family: var(--serif);
    font-weight: 600;
    font-size: 12.5px;
    letter-spacing: 0.08em;
    /* The element carries lang="en" for this: uppercasing under lang="tr" turns
       "i" into "İ", and Cinzel's own small-capital "i" keeps its dot too, so
       lowercase was no escape either. Real capitals, cased as English. */
    text-transform: uppercase;
    white-space: nowrap;
    color: var(--gold-bright);
  }
  .league {
    color: var(--muted);
    font-size: 11.5px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .save {
    font-size: 11px;
    color: var(--muted);
  }
  .save.saved {
    color: var(--ok);
  }
  .save.error {
    color: var(--bad);
  }
  .icon {
    width: 30px;
    height: 30px;
    display: grid;
    place-items: center;
    border: 0;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-2);
  }
  .icon:hover {
    background: var(--surface-2);
    color: var(--text);
  }
  .icon svg {
    width: 17px;
    height: 17px;
    fill: none;
    stroke: currentColor;
    stroke-width: 1.8;
    stroke-linecap: round;
    stroke-linejoin: round;
  }

  .scroll {
    flex: 1;
    overflow-y: auto;
    padding: 12px;
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  /* A titled group, like the game's "Advanced Settings" block: a framed recess
     with its name on a band across the top. */
  .card {
    background: rgba(0, 0, 0, 0.22);
    border: 1px solid var(--line);
    border-radius: var(--radius-sm);
    padding: 12px 14px;
  }
  .card-title {
    display: flex;
    justify-content: space-between;
    align-items: baseline;
  }
  h2 {
    /* Pulled out to the card's edges so the title reads as a band, without
       every card needing a wrapper element around its body. */
    margin: -12px -14px 10px;
    padding: 7px 14px 6px;
    border-bottom: 1px solid var(--line);
    background: rgba(255, 255, 255, 0.025);
    font-family: var(--serif);
    font-weight: 600;
    font-size: 11.5px;
    letter-spacing: 0.12em;
    text-transform: uppercase;
    color: var(--gold);
  }
  .card-title h2 {
    flex: 1;
  }
  /* A title that shares its band with a value on the right. */
  .card-title {
    margin: -12px -14px 10px;
    padding: 0 14px 0 0;
    border-bottom: 1px solid var(--line);
    background: rgba(255, 255, 255, 0.025);
  }
  .card-title h2 {
    margin: 0;
    border: 0;
    background: none;
  }
  .settings h2 {
    margin-bottom: 10px;
  }
  h3 {
    margin: 12px 0 6px;
    font-size: 12px;
    font-weight: 600;
    color: var(--text-2);
  }
  .groups {
    display: flex;
    flex-wrap: wrap;
    gap: 5px;
    margin-bottom: 4px;
  }
  .groups button {
    position: relative;
    padding: 5px 10px;
    border: 1px solid var(--line);
    border-radius: var(--radius-sm);
    background: var(--sunk);
    color: var(--text-2);
    font-size: 12px;
  }
  .groups button:hover {
    color: var(--text);
    border-color: var(--line-strong);
  }
  .groups button.on {
    border-color: var(--gold-dim);
    background: var(--surface-3);
    color: var(--gold-bright);
  }
  .custom-dot {
    display: inline-block;
    width: 6px;
    height: 6px;
    margin-left: 5px;
    vertical-align: middle;
    border-radius: 50%;
    background: var(--gold-bright);
  }
  .presets {
    display: flex;
    gap: 6px;
    margin-bottom: 10px;
  }
  .presets button {
    flex: 1;
    padding: 6px 8px;
    border: 1px solid var(--line-strong);
    border-radius: var(--radius-sm);
    background: var(--surface-2);
    color: var(--text-2);
    font-size: 11.5px;
  }
  .presets button:hover:not(:disabled) {
    color: var(--gold-bright);
    border-color: var(--gold-dim);
  }
  .presets button:disabled {
    opacity: 0.5;
    cursor: default;
  }
  /* "Download the game sounds": an offer, not a main action. */
  button.ghost {
    padding: 6px 10px;
    border: 1px solid var(--line-strong);
    border-radius: var(--radius-sm);
    background: var(--surface-2);
    color: var(--text-2);
    font-size: 11.5px;
  }
  button.ghost:hover:not(:disabled) {
    color: var(--gold-bright);
    border-color: var(--gold-dim);
  }
  button.ghost:disabled {
    opacity: 0.5;
    cursor: default;
  }
  .picker {
    margin: 10px 0 4px;
  }
  .group {
    border: 1px solid var(--line);
    border-radius: var(--radius-sm);
    padding: 10px;
    margin: 10px 0;
    display: grid;
    gap: 8px;
  }
  .group-head {
    display: flex;
    gap: 6px;
  }
  .group.drag-over {
    border-color: var(--gold-bright);
  }
  .grip {
    width: 24px;
    padding: 0;
    border: 0;
    background: none;
    color: var(--muted);
    cursor: grab;
  }
  .grip svg {
    width: 16px;
    height: 16px;
    fill: none;
    stroke: currentColor;
    stroke-width: 3;
    stroke-linecap: round;
  }
  .desc.warn {
    color: var(--gold);
  }
  .group-name {
    flex: 1;
    min-width: 0;
  }
  .group-del {
    padding: 0 10px;
    border: 1px solid var(--line);
    border-radius: var(--radius-sm);
    background: var(--bg);
    color: var(--muted);
    white-space: nowrap;
  }
  .group-del.confirm,
  .presets button.confirm {
    color: var(--bad);
    border-color: var(--bad);
  }
  .look-link {
    justify-self: start;
    padding: 0;
    border: 0;
    background: none;
    color: var(--muted);
    font-size: 11px;
    text-decoration: underline;
  }
  .sound-row {
    display: flex;
    gap: 6px;
  }
  .play {
    width: 34px;
    display: grid;
    place-items: center;
    border: 1px solid var(--line);
    border-radius: var(--radius-sm);
    background: var(--bg);
    color: var(--gold-bright);
  }
  .play svg {
    width: 14px;
    height: 14px;
    fill: currentColor;
  }
  .hint {
    margin-top: 8px;
  }
  .h3-note {
    margin-left: 4px;
    color: var(--muted);
    font-weight: 400;
    font-size: 11px;
  }
  .sub-title {
    margin-top: 16px;
    margin-bottom: 8px;
  }
  .aside {
    color: var(--muted);
    font-size: 12px;
  }
  .gold {
    color: var(--gold-bright);
    font-weight: 500;
  }
  .desc {
    margin: 4px 0 10px;
    color: var(--muted);
    font-size: 12px;
  }

  /* Status card */
  .status-head {
    display: flex;
    align-items: flex-start;
    gap: 10px;
  }
  .dot {
    flex: none;
    width: 10px;
    height: 10px;
    margin-top: 4px;
    border-radius: 50%;
    background: var(--muted);
  }
  .status.ok .dot {
    background: var(--ok);
  }
  .status.busy .dot {
    background: var(--gold-bright);
    animation: pulse 1.2s ease-in-out infinite;
  }
  .status.bad .dot {
    background: var(--bad);
  }
  .status-text {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }
  .status-text strong {
    font-size: 15px;
    font-weight: 600;
  }
  .sub {
    color: var(--text-2);
    font-size: 12px;
    overflow: hidden;
    text-overflow: ellipsis;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
  }
  .status.bad .sub {
    color: #e39a95;
  }
  .notice {
    margin: 10px 0 0;
    padding: 7px 9px;
    border-radius: var(--radius-sm);
    background: rgba(224, 166, 74, 0.1);
    border: 1px solid rgba(224, 166, 74, 0.25);
    color: var(--warn);
    font-size: 12px;
  }
  .notice.bad {
    background: rgba(226, 92, 92, 0.1);
    border-color: rgba(226, 92, 92, 0.28);
    color: var(--bad);
  }
  /* A metal plate, like the game's own buttons: dark face, thin lit edge,
     inscribed label. No gradient sweep, no glow. */
  .primary {
    width: 100%;
    margin-top: 12px;
    padding: 9px;
    border: 1px solid #8d7f5c;
    border-radius: var(--radius-sm);
    background: linear-gradient(180deg, #2c333b, #191d22);
    box-shadow: inset 0 0 0 1px #000;
    color: var(--gold-bright);
    font-family: var(--serif);
    font-weight: 600;
    font-size: 12.5px;
    letter-spacing: 0.12em;
    text-transform: uppercase;
    transition: filter 0.12s;
  }
  .primary:hover:not(:disabled) {
    filter: brightness(1.25);
  }
  .primary:disabled {
    opacity: 0.55;
    cursor: default;
  }
  .primary.pulse:not(:disabled) {
    animation: glow 1.8s ease-in-out infinite;
  }
  .meta-row {
    display: flex;
    justify-content: space-between;
    gap: 8px;
    margin-top: 8px;
    color: var(--muted);
    font-size: 11.5px;
  }
  .meta-row em {
    font-style: normal;
    color: var(--text-2);
  }
  .reload {
    margin-left: auto;
  }
  .error {
    margin: 8px 0 0;
    color: var(--bad);
    font-size: 12px;
  }

  .bar {
    height: 5px;
    margin-top: 12px;
    background: var(--sunk);
    border: 1px solid var(--line);
    overflow: hidden;
  }
  .bar.thin {
    height: 4px;
    margin-top: 4px;
  }
  .bar span {
    display: block;
    height: 100%;
    background: linear-gradient(90deg, var(--gold-dim), var(--gold-bright));
    transition: width 0.3s ease;
  }
  .bar span.cyan {
    background: linear-gradient(90deg, #2f6f80, var(--exceptional));
  }

  /* Threshold */
  .threshold {
    display: flex;
    gap: 8px;
    align-items: stretch;
  }
  .threshold input {
    width: 96px;
    padding: 0 10px;
    background: var(--bg);
    border: 1px solid var(--line);
    border-radius: var(--radius-sm);
    font-size: 18px;
    font-weight: 600;
    color: var(--gold-bright);
    text-align: right;
    user-select: text;
  }
  .threshold input:focus {
    outline: none;
    border-color: var(--gold-dim);
  }
  .threshold :global(.seg) {
    flex: 1;
  }
  .chips {
    display: flex;
    gap: 6px;
    margin-top: 8px;
  }
  .chips button {
    flex: 1;
    padding: 4px 0;
    border: 1px solid var(--line);
    border-radius: var(--radius-sm);
    background: var(--sunk);
    color: var(--text-2);
    font-size: 12px;
  }
  .chips button:hover {
    border-color: var(--line-strong);
    color: var(--text);
  }
  .chips button.on {
    border-color: var(--gold-dim);
    color: var(--gold-bright);
  }

  /* Strictness slider */
  .strict {
    width: 100%;
    margin: 14px 0 4px;
    -webkit-appearance: none;
    appearance: none;
    height: 4px;
    background: linear-gradient(90deg, var(--gold-dim) var(--p), var(--sunk) var(--p));
    border: 1px solid var(--line);
  }
  .strict::-webkit-slider-thumb {
    -webkit-appearance: none;
    width: 11px;
    height: 11px;
    background: linear-gradient(135deg, #ddd0aa, #7c7256);
    border: 1px solid #14161a;
    transform: rotate(45deg);
    cursor: pointer;
  }
  .scale {
    display: flex;
    justify-content: space-between;
    color: var(--muted);
    font-size: 11px;
  }

  /* Scan */
  .scan-line {
    display: flex;
    justify-content: space-between;
    gap: 8px;
    margin-top: 6px;
    font-size: 12px;
    color: var(--text-2);
  }
  .scan-line.muted {
    margin-top: 2px;
    color: var(--muted);
    font-size: 11.5px;
  }
  .ellipsis {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .scan-note {
    margin: 8px 0 0;
    color: var(--muted);
    font-size: 11px;
    line-height: 1.45;
  }
  .cyan-text {
    color: var(--exceptional);
  }

  /* Summary */
  .stats {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 8px;
  }
  .stats div {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 2px;
    padding: 10px 4px;
    background: var(--surface);
    border: 1px solid var(--line);
    border-radius: var(--radius);
  }
  .stats strong {
    font-size: 18px;
    font-weight: 600;
    color: var(--gold-bright);
  }
  .stats span {
    color: var(--muted);
    font-size: 11px;
  }
  .stats-caption {
    margin: -2px 0 0;
    text-align: center;
    color: var(--muted);
    font-size: 11px;
  }

  /* Settings */
  .field {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
    padding: 8px 0;
  }
  .field.stack {
    flex-direction: column;
    align-items: stretch;
    gap: 6px;
  }
  .field > span {
    font-weight: 500;
  }
  .field select,
  .field input {
    padding: 7px 9px;
    background: var(--bg);
    border: 1px solid var(--line);
    border-radius: var(--radius-sm);
    user-select: text;
  }
  .field select:focus,
  .field input:focus {
    outline: none;
    border-color: var(--gold-dim);
  }
  .actions {
    display: flex;
    gap: 8px;
  }
  .actions button {
    flex: 1;
    padding: 8px;
    border: 1px solid var(--line-strong);
    border-radius: var(--radius-sm);
    background: var(--surface-2);
    color: var(--text-2);
  }
  .actions button:hover {
    color: var(--text);
    border-color: var(--gold-dim);
  }
  .actions .danger:hover {
    color: var(--bad);
    border-color: var(--bad);
  }
  .version {
    margin: 0;
    text-align: center;
    color: var(--muted);
    font-size: 11px;
  }

  @keyframes pulse {
    50% {
      opacity: 0.35;
    }
  }
  /* The waiting button breathes along its edge instead of throwing a halo. */
  @keyframes glow {
    50% {
      border-color: var(--gold-bright);
    }
  }
</style>
