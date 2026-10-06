<script lang="ts">
  import { onMount, untrack } from 'svelte'
  import { AppService } from '../../bindings/poe2filter/internal/app'
  import type { PublishInfo } from '../../bindings/poe2filter/internal/app/models'
  import type { ProfileInfo } from '../../bindings/poe2filter/internal/engine/models'
  import type { Config, StyleGroup } from '../../bindings/poe2filter/internal/filter/models'
  import type { Listing } from '../../bindings/poe2filter/internal/publicprofile/models'
  import { t } from './i18n.svelte'

  let {
    profiles,
    groups,
    onChanged,
    onProfiles,
  }: {
    profiles: ProfileInfo[]
    groups: StyleGroup[]
    onChanged: (cfg: Config, msg: string) => Promise<void>
    onProfiles: (list: ProfileInfo[]) => void
  } = $props()

  const MAX_TAGS = 5
  const PAGE = 50

  const active = $derived(profiles.find((p) => p.active))
  const followed = $derived(active?.follow ?? null)

  let tags = $state<string[]>([])
  let reasons = $state<string[]>([])

  // Share card
  let info = $state<PublishInfo | null>(null)
  let pubName = $state('')
  let pubDesc = $state('')
  let pubTags = $state<string[]>([])
  let busy = $state(false)
  let shareMsg = $state('')
  let shareErr = $state('')
  let confirmUnpublish = $state(false)
  let confirmUnfollow = $state(false)
  let reporting = $state(false)
  let reportReason = $state('spam')
  let reportNote = $state('')

  // Discover card
  let query = $state('')
  let tagFilter = $state('')
  let list = $state<Listing[]>([])
  let page = $state(0)
  let more = $state(false)
  let loading = $state(false)
  let listErr = $state('')
  let searchTimer: ReturnType<typeof setTimeout> | undefined

  onMount(() => {
    void AppService.PublicProfileTags().then((v) => (tags = v ?? []))
    void AppService.PublicProfileReasons().then((v) => (reasons = v ?? []))
    void load(true)
  })

  // Reload the share card whenever another profile becomes active.
  let loadedFor = ''
  $effect(() => {
    const name = active?.name ?? ''
    if (name && name !== loadedFor) {
      loadedFor = name
      untrack(() => void loadInfo(name))
    }
  })

  async function loadInfo(name: string) {
    shareMsg = shareErr = ''
    confirmUnpublish = confirmUnfollow = reporting = false
    info = null
    try {
      info = await AppService.ProfilePublishInfo(name)
    } catch (e) {
      shareErr = String(e)
    }
    const l = info?.listing
    pubName = l?.name ?? name
    pubDesc = l?.description ?? ''
    pubTags = [...(l?.tags ?? [])]
  }

  function groupLabel(key: string): string {
    return groups.find((g) => g.id === key)?.label ?? key
  }

  function toggleTag(tag: string) {
    if (pubTags.includes(tag)) pubTags = pubTags.filter((x) => x !== tag)
    else if (pubTags.length < MAX_TAGS) pubTags = [...pubTags, tag]
  }

  async function publish() {
    if (!active) return
    busy = true
    shareMsg = shareErr = ''
    const wasLive = !!info?.listing
    try {
      const l = await AppService.PublishProfile(active.name, { name: pubName.trim(), description: pubDesc.trim(), tags: pubTags })
      onProfiles((await AppService.Profiles()) ?? profiles)
      await loadInfo(active.name)
      shareMsg = wasLive ? t('pub.updated', l.version) : t('pub.published')
      void load(true)
    } catch (e) {
      shareErr = String(e)
    }
    busy = false
  }

  async function unpublish() {
    if (!active) return
    busy = true
    shareMsg = shareErr = ''
    try {
      await AppService.UnpublishProfile(active.name)
      onProfiles((await AppService.Profiles()) ?? profiles)
      await loadInfo(active.name)
      shareMsg = t('pub.unpublished')
      void load(true)
    } catch (e) {
      shareErr = String(e)
    }
    busy = false
  }

  async function unfollow() {
    if (!active) return
    busy = true
    try {
      const name = active.name
      await onChanged(await AppService.DeleteProfile(name), t('pub.unfollowed', name))
    } catch (e) {
      shareErr = String(e)
    }
    busy = false
  }

  async function sendReport() {
    if (!followed) return
    busy = true
    shareErr = ''
    try {
      await AppService.ReportProfile(followed.id, reportReason, reportNote)
      reporting = false
      reportNote = ''
      shareMsg = t('pub.reported')
    } catch (e) {
      shareErr = String(e)
    }
    busy = false
  }

  async function load(reset: boolean) {
    loading = true
    listErr = ''
    const next = reset ? 0 : page + 1
    try {
      const got = (await AppService.PublicProfiles(query, tagFilter, next)) ?? []
      list = reset ? got : [...list, ...got]
      page = next
      more = got.length === PAGE
    } catch (e) {
      listErr = String(e)
    }
    loading = false
  }

  function search() {
    clearTimeout(searchTimer)
    searchTimer = setTimeout(() => void load(true), 350)
  }

  function pickTag(tag: string) {
    tagFilter = tagFilter === tag ? '' : tag
    void load(true)
  }

  const followedIds = $derived(new Set(profiles.map((p) => p.follow?.id).filter(Boolean)))
  const ownIds = $derived(new Set(profiles.map((p) => p.publicId).filter(Boolean)))

  async function follow(l: Listing) {
    busy = true
    listErr = ''
    try {
      await onChanged(await AppService.FollowProfile(l.id), t('pub.following', l.name))
      void load(true)
    } catch (e) {
      listErr = String(e)
    }
    busy = false
  }

  function authorName(author: string): string {
    return author.split('#')[0]
  }
</script>

<section class="card pub">
  <h2>{t('pub.shareTitle')}</h2>
  {#if followed}
    <p class="who">
      <strong>{followed.name}</strong> · {authorName(followed.author)}
      <span class="muted">· {t('pub.version', followed.version)}</span>
    </p>
    <p class="desc">{t('pub.followedDesc')}</p>
    {#if followed.gone}<p class="desc warn">{t('pub.gone')}</p>{/if}
    <div class="row">
      <button
        type="button"
        class:confirm={confirmUnfollow}
        disabled={busy}
        onclick={() => (confirmUnfollow ? unfollow() : (confirmUnfollow = true))}
        onblur={() => (confirmUnfollow = false)}
      >
        {confirmUnfollow ? t('pub.unfollowConfirm') : t('pub.unfollow')}
      </button>
      {#if !followed.gone}
        <button type="button" class="ghost" disabled={busy} onclick={() => (reporting = !reporting)}>{t('pub.report')}</button>
      {/if}
    </div>
    {#if reporting}
      <div class="report">
        <select bind:value={reportReason}>
          {#each reasons as r (r)}<option value={r}>{t('pub.reason.' + r)}</option>{/each}
        </select>
        <input bind:value={reportNote} maxlength="300" placeholder={t('pub.reportNote')} spellcheck="false" />
        <button type="button" disabled={busy} onclick={sendReport}>{t('pub.reportSend')}</button>
      </div>
    {/if}
  {:else if info && !info.account}
    <p class="desc">{t('pub.needAccount')}</p>
  {:else if info}
    {#if info.listing}
      <p class="who">
        <span class="live">{t('pub.live')}</span>
        {t('pub.followers', info.listing.followers)} · {t('pub.version', info.listing.version)}
      </p>
      {#if info.listing.hidden}<p class="desc warn">{t('pub.hidden')}</p>{/if}
    {:else}
      <p class="desc">{t('pub.shareDesc')}</p>
    {/if}
    <label class="field">
      <span>{t('pub.name')}</span>
      <input bind:value={pubName} maxlength="40" spellcheck="false" />
    </label>
    <label class="field">
      <span>{t('pub.description')}</span>
      <input bind:value={pubDesc} maxlength="300" placeholder={t('pub.descriptionPlaceholder')} spellcheck="false" />
    </label>
    <div class="tags">
      <span class="label">{t('pub.tags', MAX_TAGS)}</span>
      {#each tags as tag (tag)}
        <button
          type="button"
          class="chip"
          class:on={pubTags.includes(tag)}
          disabled={!pubTags.includes(tag) && pubTags.length >= MAX_TAGS}
          onclick={() => toggleTag(tag)}>{t('tag.' + tag)}</button
        >
      {/each}
    </div>
    {#each info.sounds ?? [] as s (s.group)}
      <p class="desc warn">{t('pub.soundSwap', groupLabel(s.group), s.file)}</p>
    {/each}
    <p class="desc hint">{t('pub.whatShared', info.account)}</p>
    <div class="row">
      <button type="button" class="primary" disabled={busy || !pubName.trim()} onclick={publish}>
        {info.listing ? t('pub.update') : t('pub.publish')}
      </button>
      {#if info.listing}
        <button
          type="button"
          class:confirm={confirmUnpublish}
          disabled={busy}
          onclick={() => (confirmUnpublish ? unpublish() : (confirmUnpublish = true))}
          onblur={() => (confirmUnpublish = false)}
        >
          {confirmUnpublish ? t('pub.unpublishConfirm') : t('pub.unpublish')}
        </button>
      {/if}
    </div>
  {/if}
  {#if shareMsg}<p class="desc ok">{shareMsg}</p>{/if}
  {#if shareErr}<p class="error">{shareErr}</p>{/if}
</section>

<section class="card pub">
  <h2>{t('pub.discoverTitle')}</h2>
  <p class="desc">{t('pub.discoverDesc')}</p>
  <input class="search" bind:value={query} oninput={search} maxlength="40" placeholder={t('pub.search')} spellcheck="false" />
  <div class="tags">
    {#each tags as tag (tag)}
      <button type="button" class="chip" class:on={tagFilter === tag} onclick={() => pickTag(tag)}>{t('tag.' + tag)}</button>
    {/each}
  </div>
  {#if listErr}<p class="error">{listErr}</p>{/if}
  {#if !loading && !listErr && list.length === 0}
    <p class="desc">{t('pub.empty')}</p>
  {/if}
  <ul class="list">
    {#each list as l (l.id)}
      <li>
        <div class="main">
          <div class="title">
            <strong>{l.name}</strong>
            <span class="muted">· {authorName(l.author)}</span>
            {#if ownIds.has(l.id)}<span class="badge">{t('pub.yours')}</span>{/if}
          </div>
          {#if l.description}<div class="text">{l.description}</div>{/if}
          <div class="meta">
            <span class="count">{t('pub.followers', l.followers)}</span>
            {#each l.tags ?? [] as tag (tag)}<span class="mini">{t('tag.' + tag)}</span>{/each}
          </div>
        </div>
        {#if followedIds.has(l.id)}
          <span class="state">{t('pub.followingNow')}</span>
        {:else if !ownIds.has(l.id)}
          <button type="button" disabled={busy} onclick={() => follow(l)}>{t('pub.follow')}</button>
        {/if}
      </li>
    {/each}
  </ul>
  {#if more}
    <button type="button" class="ghost more" disabled={loading} onclick={() => load(false)}>{t('pub.more')}</button>
  {/if}
  {#if loading}<p class="desc">{t('pub.loading')}</p>{/if}
</section>

<style>
  .pub { margin-top: 14px; padding: 12px 14px; border: 1px solid var(--line); border-radius: var(--radius-sm); background: var(--ui-tint, rgba(0, 0, 0, 0.22)); }
  .pub h2 { margin: -12px -14px 10px; padding: 7px 14px 6px; border-bottom: 1px solid var(--line); background: var(--ui-tint, rgba(255, 255, 255, 0.025)); font-family: var(--serif); font-weight: 600; font-size: 11.5px; letter-spacing: 0.12em; text-transform: uppercase; color: var(--gold); }
  .pub .desc { margin: 4px 0 8px; color: var(--muted); font-size: 12px; line-height: 1.5; }
  .pub .desc.warn { color: var(--gold-bright); }
  .pub .desc.ok { color: var(--ui-ok, #9fc48a); }
  .pub .hint { margin-top: 8px; }
  .pub .error { margin: 6px 0; color: var(--ui-bad, #e88b84); font-size: 12px; user-select: text; }
  .pub .who { margin: 2px 0 6px; color: var(--text-2); }
  .pub .muted { color: var(--muted); }
  .pub .live { margin-right: 6px; padding: 1px 6px; border: 1px solid var(--ui-ok, #9fc48a); color: var(--ui-ok, #9fc48a); font-size: 10.5px; letter-spacing: 0.08em; text-transform: uppercase; }
  .pub .field { display: grid; grid-template-columns: 110px 1fr; align-items: center; gap: 8px; margin: 6px 0; font-size: 12px; color: var(--text-2); }
  .pub input, .pub select { box-sizing: border-box; width: 100%; padding: 6px 8px; border: 1px solid var(--line-strong); border-radius: var(--radius-sm); background: var(--sunk); color: var(--text); font-size: 12px; }
  .pub .tags { display: flex; flex-wrap: wrap; align-items: center; gap: 5px; margin: 8px 0; }
  .pub .tags .label { margin-right: 4px; color: var(--text-2); font-size: 12px; }
  .pub button { padding: 6px 10px; border: 1px solid var(--line-strong); border-radius: var(--radius-sm); background: var(--surface-2); color: var(--text-2); font-size: 11.5px; }
  .pub button:hover:not(:disabled) { color: var(--gold-bright); border-color: var(--gold-dim); }
  .pub button:disabled { opacity: 0.45; }
  .pub button.ghost { background: transparent; }
  .pub button.confirm { border-color: var(--ui-bad, #e88b84); color: var(--ui-bad, #e88b84); }
  .pub button.primary { padding: 8px 14px; border-color: var(--ui-line-strong, #8d7f5c); background: linear-gradient(180deg, var(--ui-surface-3, #2c333b), var(--ui-surface-2, #191d22)); color: var(--gold-bright); font-family: var(--serif); font-weight: 600; letter-spacing: 0.1em; text-transform: uppercase; }
  .pub .chip { padding: 3px 9px; border-radius: 999px; font-size: 11px; }
  .pub .chip.on { border-color: var(--gold-dim); background: var(--surface-3); color: var(--gold-bright); }
  .pub .row { display: flex; flex-wrap: wrap; gap: 6px; margin: 8px 0 2px; }
  .pub .report { display: grid; grid-template-columns: 140px 1fr auto; gap: 6px; margin-top: 8px; }
  .pub .search { margin: 2px 0 4px; }
  .pub .list { margin: 6px 0 0; padding: 0; list-style: none; }
  .pub .list li { display: flex; align-items: center; gap: 10px; padding: 8px 2px; border-top: 1px solid var(--line); }
  .pub .list .main { flex: 1; min-width: 0; }
  .pub .list .title { overflow: hidden; color: var(--gold-bright); text-overflow: ellipsis; white-space: nowrap; }
  .pub .list .text { margin-top: 2px; overflow: hidden; color: var(--text-2); font-size: 11.5px; text-overflow: ellipsis; white-space: nowrap; }
  .pub .list .meta { display: flex; flex-wrap: wrap; gap: 5px; margin-top: 4px; font-size: 10.5px; color: var(--muted); }
  .pub .list .count { margin-right: 4px; color: var(--text-2); }
  .pub .mini { padding: 0 6px; border: 1px solid var(--line); border-radius: 999px; }
  .pub .badge { margin-left: 6px; padding: 0 6px; border: 1px solid var(--gold-dim); color: var(--gold); font-size: 10px; }
  .pub .state { color: var(--ui-ok, #9fc48a); font-size: 11.5px; white-space: nowrap; }
  .pub .more { margin-top: 8px; width: 100%; }
</style>
