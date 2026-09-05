<script>

  import * as api from './api.js'
  import { tintFor, inkOn } from './tint.js'
  import { duration, size, durationGap } from './format.js'
  import ReleasePicker from './ReleasePicker.svelte'
  import { reveal } from './reveal.js'
  import { compare, split, labelFor, linkFor } from './diff.js'
  import { t } from './i18n.svelte.js'

  let { albums = [], mode = 'move', inbox = '', onchange } = $props()

  let selectedId = $state(null)
  let album = $state(null)
  let releases = $state(null)
  let release = $state(null)
  let plan = $state(null)
  let tint = $state(null)
  let error = $state('')
  let busy = $state(false)
  let loading = $state(false)
  let pickerOpen = $state(false)
  let switching = $state(false)
  let filed = $state(null)
  let expanded = $state({})
  let allExpanded = $state(false)
  let showAllRows = $state(readFullView())

  const VIEW_KEY = 'gordi-tracks-full'

  function readFullView() {
    try {
      return localStorage.getItem(VIEW_KEY) === '1'
    } catch {
      return false
    }
  }

  function toggleView(complete) {
    showAllRows = complete
    try {
      localStorage.setItem(VIEW_KEY, complete ? '1' : '0')
    } catch {
    }
  }

  let token = 0
  let booted = false

  $effect(() => {
    if (!booted && albums.length > 0) {
      booted = true
      open(albums[0].id)
    }
  })

  async function open(id) {
    const mine = ++token
    selectedId = id
    album = null
    releases = null
    release = null
    plan = null
    filed = null
    expanded = {}
    allExpanded = false
    error = ''
    pickerOpen = false
    loading = true

    tintFor(`/api/albums/${id}/cover`).then((t) => {
      if (mine === token) tint = t
    })

    try {
      const [a, p] = await Promise.all([api.getAlbum(id), api.getCandidates(id)])
      if (mine !== token) return
      album = a
      releases = p.releases ?? []
      const obvious =
        releases.find((r) => r.from_tags) ??
        releases.find((r) => r.track_count === a.track_count) ??
        releases[0]
      if (obvious) await select(obvious, mine)
    } catch (e) {
      if (mine === token) error = e.message
    } finally {
      if (mine === token) loading = false
    }
  }

  async function select(chosen, mine = token) {
    pickerOpen = false
    switching = true
    try {
      const detail = await api.getRelease(chosen.id)
      if (mine !== token) return
      release = detail
      plan = await api.getPlan(selectedId, release.id, mode)
    } catch (e) {
      if (mine === token) error = e.message
    } finally {
      if (mine === token) switching = false
    }
  }

  const readyToFile = $derived(!!plan && !busy && !switching)

  async function file() {
    busy = true
    error = ''
    try {
      filed = await api.apply(selectedId, release.id, mode)
      onchange?.()
      const next = albums.find((a) => a.id !== selectedId)
      if (next) setTimeout(() => open(next.id), 900)
    } catch (e) {
      error = e.message
    } finally {
      busy = false
    }
  }

  function onKeydown(e) {
    if (e.target instanceof Element && e.target.closest('input, select, textarea')) return
    const i = albums.findIndex((a) => a.id === selectedId)
    if (e.key === 'ArrowDown' && i < albums.length - 1) open(albums[i + 1].id)
    if (e.key === 'ArrowUp' && i > 0) open(albums[i - 1].id)

    if (e.key === 'Enter' && readyToFile && e.target === document.body) file()
  }

  const rows = $derived.by(() => {
    if (!album) return []
    const files = album.tracks ?? []
    const tracks = release?.tracks ?? []
    const mapping = plan?.tracks?.map((p) => p.index) ?? null

    const row = (file, track, idx) => ({
      file,
      track,
      idx,
      changed: !!file && !!track && file.title !== track.title,
      gap: durationGap(file?.audio?.length, track?.length),
    })

    const matched = new Set()
    const out = files.map((file, i) => {
      const j = mapping ? mapping[i] : i
      const track = tracks[j] ?? null
      if (track) matched.add(j)
      return row(file, track, i)
    })

    tracks.forEach((track, j) => {
      if (!matched.has(j)) out.push(row(null, track, -1))
    })

    return out.sort((a, b) => (a.track?.position ?? Infinity) - (b.track?.position ?? Infinity))
  })

  const filesLength = $derived(album?.length ?? 0)
  const releaseLength = $derived(release?.tracks?.reduce((s, t) => s + (t.length ?? 0), 0) ?? 0)
  const scale = $derived(Math.max(filesLength, releaseLength, 1))
  const share = (ms) => `${((ms ?? 0) / scale) * 100}%`

  const verdict = $derived.by(() => {
    if (!album || !release) return null
    const gap = durationGap(filesLength, releaseLength) ?? 0
    const sameTrackCount = album.track_count === release.tracks.length
    const titleFixes = rows.filter((l) => l.changed).length

    if (!sameTrackCount) {
      return {
        tone: 'warn',
        title: t('verdict.checkTitle'),
        detail: t('verdict.checkDetail', {
          files: album.track_count,
          release: release.tracks.length,
        }),
      }
    }
    if (Math.abs(gap) > 5) {
      return {
        tone: 'warn',
        title: t('verdict.durationsTitle'),
        detail: t('verdict.durationsDetail', { n: Math.abs(gap) }),
      }
    }
    return {
      title: t('verdict.matches'),
      detail:
        t('verdict.matchesDetail', { n: album.track_count }) +
        (titleFixes > 0 ? ` · ${t('verdict.titlesToFix', { n: titleFixes })}` : '.'),
    }
  })

  const folder = $derived(plan?.tracks?.[0]?.destination.split('/').slice(0, -1).join('/') ?? '')

  const changes = $derived.by(() => {
    if (!album || !plan) return { shared: [], own: [] }
    const diffs = plan.tracks.map((track, i) => compare(album.tracks[i]?.raw, track.tags))
    return split(diffs)
  })

  // The header mark takes the raw tint, like the bars; --brand covers the rest.
  $effect(() => {
    const root = document.documentElement
    const clear = () => {
      root.style.removeProperty('--tint')
      root.style.removeProperty('--on-tint')
      root.style.removeProperty('--brand')
    }
    if (tint) {
      root.style.setProperty('--tint', tint)
      root.style.setProperty('--on-tint', inkOn(tint))
      root.style.setProperty('--brand', 'var(--tint)')
    } else {
      clear()
    }
    return clear
  })

  const renameOf = (i) => {
    const track = plan?.tracks?.[i]
    if (!track) return null
    const before = track.source.split('/').pop()
    const after = track.destination.split('/').pop()
    return before === after ? null : { before, after }
  }

  function toggle(i) {
    expanded[i] = !expanded[i]
  }

  function rowsFor(i, own) {
    const source = showAllRows ? allTags(i) : own.map((d) => ({ ...d, changed: true }))
    const rename = renameOf(i)
    if (rename) {
      source.push({
        key: '__filename',
        before: rename.before,
        after: rename.after,
        changed: true,
        mono: true,
      })
    }
    return source
  }

  function otherTagCount(i, own) {
    return Math.max(0, allTags(i).length - own.length)
  }

  function allTags(i) {
    const after = plan?.tracks?.[i]?.tags ?? {}
    const before = album?.tracks?.[i]?.raw ?? {}
    return Object.keys(after)
      .sort()
      .map((key) => {
        const a = (before[key] ?? []).join(' / ')
        const b = (after[key] ?? []).join(' / ')
        return { key, before: a, after: b, changed: a !== b }
      })
  }

  function toggleAll() {
    allExpanded = !allExpanded
    expanded = {}
  }

  const IDENTIFIER_TAGS = new Set([
    'MUSICBRAINZ_ALBUMID',
    'MUSICBRAINZ_ALBUMARTISTID',
    'MUSICBRAINZ_RELEASEGROUPID',
    'MUSICBRAINZ_TRACKID',
    'MUSICBRAINZ_RELEASETRACKID',
  ])

  function bucket(list = []) {
    return {
      replaced: list.filter((d) => d.kind !== 'added' && !IDENTIFIER_TAGS.has(d.key)),
      added: list.filter((d) => d.kind === 'added' && !IDENTIFIER_TAGS.has(d.key)),
      identifiers: list.filter((d) => IDENTIFIER_TAGS.has(d.key)),
    }
  }

  const shared = $derived(bucket(changes.shared))

  const unchangedTags = $derived.by(() => {
    const tracks = plan?.tracks ?? []
    if (!album || !tracks.length) return []

    const counts = new Map()
    tracks.forEach((track, i) => {
      const before = album.tracks?.[i]?.raw ?? {}
      for (const [key, values] of Object.entries(track.tags ?? {})) {
        const after = (values ?? []).join(' / ')
        if (!after || (before[key] ?? []).join(' / ') !== after) continue
        const signature = `${key}\u0000${after}`
        counts.set(signature, (counts.get(signature) ?? 0) + 1)
      }
    })

    return [...counts]
      .filter(([, n]) => n === tracks.length)
      .map(([signature]) => {
        const [key, after] = signature.split('\u0000')
        return { key, after }
      })
      .sort((a, b) => a.key.localeCompare(b.key))
  })

  const identifiers = $derived(
    [...shared.identifiers, ...unchangedTags.filter((d) => IDENTIFIER_TAGS.has(d.key))].sort((a, b) =>
      a.key.localeCompare(b.key),
    ),
  )
  const kept = $derived(unchangedTags.filter((d) => !IDENTIFIER_TAGS.has(d.key)))

</script>

<svelte:window onkeydown={onKeydown} />

<div class="workbench">
  <aside class="queue">
    <p class="eyebrow queue-title">{t('queue.title', { n: albums.length })}</p>

    {#if albums.length === 0}
      <p class="empty muted">{t('queue.empty', { folder: inbox })}</p>
    {:else}
      <ul>
        {#each albums as a (a.id)}
          <li>
            <button
              class="queue-item"
              class:active={selectedId === a.id}
              onclick={() => open(a.id)}
              title={`${a.title || a.rel_dir} — ${a.artist || t('queue.unknownArtist')}`}
            >
              <span class="name">{a.title || a.rel_dir}</span>
              <span class="sub muted">{a.artist || t('queue.unknownArtist')}</span>
              <span class="figures mono muted">{a.track_count} · {duration(a.length)}</span>
            </button>
          </li>
        {/each}
      </ul>
    {/if}
  </aside>

  <section class="workspace">
    {#if !album}
      <p class="empty muted">{error || (loading ? t('album.reading') : '')}</p>
    {:else}
      {#key album.id}
      <header class="hero" style:--cover={`url(/api/albums/${album.id}/cover)`}>
        <img class="cover" src={`/api/albums/${album.id}/cover`} alt="" />
        <div>
          <h1>{album.title || album.rel_dir}</h1>
          <p class="artist">{album.artist || t('album.unknownArtist')}</p>
          <p class="mono muted specs">
            {album.quality} · {size(album.size)}{#if album.untagged > 0}
              · {t('album.untagged', { n: album.untagged })}
            {/if}
          </p>
        </div>
      </header>

      {#if error}
        <p class="error">{error}</p>
      {/if}

      {#if filed}
        <div class="done">
          <p class="verdict-title">{t('action.filed')}</p>
          <p class="muted">
            {t('action.filedDetail', { n: filed.filed, folder: '' })}<span class="mono"
              >{folder}</span
            >{#if filed.deleted}, {t('action.originalsDeleted')}{/if}{#if filed.ignored?.length}, {t(
                'action.leftBehind',
                { n: filed.ignored.length },
              )}{/if}.
          </p>
        </div>
      {:else if loading && !release}
        <p class="empty muted">{t('album.searching')}</p>
      {:else if !release}
        <div class="panel">
          <p class="verdict-title">{t('album.noRelease')}</p>
          <p class="muted">{t('album.noReleaseHint')}</p>
        </div>
      {:else}
        <!-- The verdict: the only thing you need to read to decide. -->
        <div class="verdict" class:warn={verdict?.tone === 'warn'}>
          <p class="verdict-title">{verdict?.title}</p>
          <p class="verdict-detail">{verdict?.detail}</p>

          <div class="scale">
            <span class="eyebrow">{t('verdict.yourFiles')}</span>
            <span class="rail"><span class="bar yours" style:width={share(filesLength)}></span></span>
            <span class="mono figure">{duration(filesLength)}</span>

            <span class="eyebrow">{t('verdict.thisRelease')}</span>
            <span class="rail"><span class="bar" style:width={share(releaseLength)}></span></span>
            <span class="mono figure">{duration(releaseLength)}</span>
          </div>
        </div>

        {#if plan?.warnings?.length}
          <div class="warnings">
            <p class="eyebrow">{t('verdict.toCheck')}</p>
            <ul>
              {#each plan.warnings as a (a)}<li>{a}</li>{/each}
            </ul>
          </div>
        {/if}

        <div class="selected-release" class:open={pickerOpen}>
          <div>
            <p class="eyebrow">{t('release.selected')}</p>
            <p class="release-title">
              <a
                href={`https://musicbrainz.org/release/${release.id}`}
                target="_blank"
                rel="noreferrer"
                title={t('release.openMB')}
              >
                {release.title}
              </a>{#if release.disambiguation}<span class="muted"> ({release.disambiguation})</span
                >{/if}{#if releases.find((r) => r.id === release.id)?.from_tags}<span class="from-tags"
                  >{t('release.fromTags')}</span
                >{/if}
            </p>
            <p class="release-meta muted">
              {[
                release.format,
                release.country,
                release.date?.slice(0, 4),
                release.status,
                release.label,
                release.catalog,
              ]
                .filter((v) => v && v !== 'None')
                .join(' · ')}{#if release.release_group_id}{' · '}<a
                  class="group-link"
                  href={`https://musicbrainz.org/release-group/${release.release_group_id}`}
                  target="_blank"
                  rel="noreferrer"
                >{t('release.group')}</a>{/if}
            </p>
          </div>
          <button
            class="change"
            class:open={pickerOpen}
            aria-expanded={pickerOpen}
            aria-label={pickerOpen ? t('release.close') : t('release.change', { n: releases.length })}
            title={pickerOpen ? t('release.close') : t('release.change', { n: releases.length })}
            onclick={() => (pickerOpen = !pickerOpen)}
          >
            <span class="mono">{releases.length}</span>
            <svg viewBox="0 0 12 12" width="11" height="11" aria-hidden="true">
              <path
                d="M2.5 4.5 6 8l3.5-3.5"
                fill="none"
                stroke="currentColor"
                stroke-width="1.6"
                stroke-linecap="round"
                stroke-linejoin="round"
              />
            </svg>
          </button>
        </div>

        {#if pickerOpen}
          <ReleasePicker
            attached
            {releases}
            selected={release.id}
            expectedTracks={album.track_count}
            albumId={album.id}
            onselect={(r) => select(r)}
          />
        {/if}

        <div class="columns">
          <div class="tracks-column">
        <div class="tracks-header">
          <span class="eyebrow">{t('tracks.title')}</span>
          <button onclick={toggleAll}>
            {allExpanded ? t('tracks.collapseAll') : t('tracks.expandAll')}
          </button>
        </div>

        <ol class="tracks">
          {#each rows as l, i (i)}
            {@const own = l.idx >= 0 ? (changes.own[l.idx] ?? []) : []}
            {@const expandedRow = l.idx >= 0 && (allExpanded || !!expanded[l.idx])}
            <li class:missing={!l.file || !l.track}>
              <button
                class="row"
                onclick={() => l.idx >= 0 && toggle(l.idx)}
                aria-expanded={expandedRow}
              >
                <span class="rank mono">{String(l.track?.position ?? l.idx + 1).padStart(2, '0')}</span>
                <span class="title">
                  {#if l.changed}
                    <span class="old">{l.file.title}</span>
                    <span class="new">{l.track.title}</span>
                  {:else if l.track && !l.file}
                    {l.track.title}
                    <span class="missing-track">{t('tracks.noFile')}</span>
                  {:else if l.track}
                    {l.track.title}
                  {:else}
                    <span class="old">{l.file.title}</span>
                    <span class="missing-track">{t('tracks.notInRelease')}</span>
                  {/if}
                </span>
                {#if own.length}
                  <span class="count muted mono">{own.length}</span>
                {/if}
                {#if l.gap !== null && Math.abs(l.gap) > 5}
                  <span class="drift mono">{l.gap > 0 ? '+' : ''}{l.gap}s</span>
                {/if}
                <span class="time mono muted">{duration(l.track?.length ?? l.file?.audio?.length)}</span>
              </button>

              {#if expandedRow}
                {@const trackRows = rowsFor(l.idx, own)}
                <div class="reveal" transition:reveal={{ duration: 170 }}>
                  <div class="clip">
                    <div class="details">
                  {#if trackRows.length}
                    <dl class="track-tags">
                      {#each trackRows as d (d.key)}
                        <div class:unchanged={!d.changed}>
                          <dt>
                            {d.key === '__filename' ? t('tracks.fileName') : labelFor(d.key)}
                          </dt>
                          <dd class:mono={d.mono}>
                            {#if linkFor(d.key, d.after)}
                              <a
                                href={linkFor(d.key, d.after)}
                                target="_blank"
                                rel="noreferrer"
                                title={t('change.openMB')}
                              >
                                <code>{d.after}</code>
                              </a>
                            {:else if d.changed && d.before}
                              <span class="old">{d.before}</span>
                              {#if d.after}
                                <span class="new">{d.after}</span>
                              {:else}
                                <span class="removed">{t('change.removed')}</span>
                              {/if}
                            {:else if d.changed}
                              <span class="new">{d.after}</span>
                            {:else}
                              <span>{d.after}</span>
                            {/if}
                          </dd>
                        </div>
                      {/each}
                    </dl>
                  {:else}
                    <p class="muted small">{t('tracks.nothingSpecific')}</p>
                  {/if}

                      {#if otherTagCount(l.idx, own) > 0}
                        <button
                          class="show-all"
                          onclick={() => toggleView(!showAllRows)}
                        >
                          {showAllRows
                            ? t('tracks.showLess')
                            : t('tracks.otherTags', { n: otherTagCount(l.idx, own) })}
                        </button>
                      {/if}
                    </div>
                  </div>
                </div>
              {/if}
            </li>
          {/each}
        </ol>
          </div>

          <div class="diff-column">
        <section class="changes">
          {#if !shared.replaced.length && !shared.added.length && !identifiers.length && !kept.length}
            <p class="muted">{t('change.none')}</p>
          {/if}

          {#if shared.replaced.length}
            <p class="eyebrow section-label">{t('change.replaced')}</p>
            <dl class="replaced">
              {#each shared.replaced as d (d.key)}
                <div>
                  <dt>{labelFor(d.key)}</dt>
                  <dd>
                    <span class="old">{d.before}</span>
                    {#if d.after}
                      <span class="new">{d.after}</span>
                    {:else}
                      <span class="removed">{t('change.removed')}</span>
                    {/if}
                  </dd>
                </div>
              {/each}
            </dl>
          {/if}

          {#if shared.added.length}
            <p class="eyebrow section-label">{t('change.added')}</p>
            <dl class="added">
              {#each shared.added as d (d.key)}
                <div>
                  <dt class="muted">{labelFor(d.key)}</dt>
                  <dd>{d.after}</dd>
                </div>
              {/each}
            </dl>
          {/if}

          {#if kept.length}
            <p class="eyebrow section-label">{t('change.kept')}</p>
            <dl class="added">
              {#each kept as d (d.key)}
                <div>
                  <dt class="muted">{labelFor(d.key)}</dt>
                  <dd>{d.after}</dd>
                </div>
              {/each}
            </dl>
          {/if}

          {#if identifiers.length}
            <p class="eyebrow section-label">{t('change.identifiers')}</p>
            <dl class="identifiers">
              {#each identifiers as d (d.key)}
                <div>
                  <dt class="muted">{labelFor(d.key)}</dt>
                  <dd>
                    {#if linkFor(d.key, d.after)}
                      <a
                        href={linkFor(d.key, d.after)}
                        target="_blank"
                        rel="noreferrer"
                        title={t('change.openMB')}
                      >
                        <code>{d.after}</code>
                      </a>
                    {:else}
                      <code>{d.after}</code>
                    {/if}
                  </dd>
                </div>
              {/each}
            </dl>
          {/if}
        </section>
          </div>
        </div>

        <footer class="action">
          <p class="destination">
            <span class="eyebrow">
              {mode === 'move' ? t('action.moveTo') : t('action.copyTo')}
            </span>
            <span class="mono">{folder}</span>
            {#if mode === 'move' && plan?.ignored?.length}
              <span class="muted small">{t('action.leftBehind', { n: plan.ignored.length })}</span>
            {/if}
          </p>
          <button class="file-button" onclick={file} disabled={!readyToFile}>
            {busy ? t('action.filing') : t('action.file')}
          </button>
        </footer>
      {/if}
      {/key}
    {/if}
  </section>
</div>

<style>
  .workbench {
    display: grid;
    grid-template-columns: minmax(240px, 300px) 1fr;
    height: 100%;
    min-height: 0;
  }

  .queue {
    border-right: 1px solid var(--line);
    overflow-y: auto;
    padding: 16px 0 24px;
  }

  .queue-title {
    padding: 0 16px 8px;
    margin: 0;
  }

  .queue ul {
    list-style: none;
    margin: 0;
    padding: 0;
  }

  .queue-item {
    display: grid;
    grid-template-columns: 1fr auto;
    gap: 0 10px;
    width: 100%;
    text-align: left;
    border: none;
    border-radius: 0;
    padding: 8px 16px;
    border-left: 2px solid transparent;
  }

  .queue-item:hover {
    background: color-mix(in srgb, var(--tint) 7%, var(--surface));
  }

  .queue-item.active {
    background: var(--tint-soft);
    border-left-color: var(--tint);
  }

  .name {
    grid-column: 1 / -1;
    font-weight: 500;
    line-height: 1.3;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .sub {
    grid-row: 2;
    grid-column: 1;
    align-self: center;
    font-size: 12px;
    line-height: 1.35;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .figures {
    grid-row: 2;
    grid-column: 2;
    align-self: center;
    font-size: 11px;
    line-height: 1.35;
  }

  .workspace {
    overflow-y: auto;
    padding: 26px clamp(18px, 4vw, 44px) 60px;
  }

  .columns {
    display: grid;
    gap: 0 34px;
    align-items: start;
    margin-top: 22px;
  }

  .tracks-column {
    background: var(--surface);
    border: 1px solid var(--line);
    border-radius: 4px;
    padding: 14px 16px;
  }

  @media (min-width: 1180px) {
    .columns {
      grid-template-columns: minmax(0, 1.15fr) minmax(320px, 0.85fr);
    }

    .diff-column {
      position: sticky;
      top: 0;
    }
  }

  @media (max-width: 1179px) {
    .diff-column {
      order: -1;
      margin-bottom: 8px;
    }
  }

  .hero {
    position: relative;
    display: flex;
    gap: 18px;
    align-items: flex-end;
    margin-bottom: 26px;
    isolation: isolate;
  }

  .hero::before {
    content: '';
    position: absolute;
    inset: -26px -40px -18px -40px;
    z-index: -1;
    background-image: var(--cover);
    background-size: cover;
    background-position: center 35%;
    opacity: 0.16;
    filter: blur(34px) saturate(1.5);
    mask-image: linear-gradient(to bottom, #000 20%, transparent);
  }

  .cover {
    width: 96px;
    height: 96px;
    object-fit: cover;
    border-radius: 2px;
    background: var(--surface-sunken);
    flex-shrink: 0;
  }

  h1 {
    margin: 0;
    font-size: clamp(24px, 3.4vw, 34px);
    font-weight: 600;
    letter-spacing: -0.02em;
    line-height: 1.1;
  }

  .artist {
    margin: 2px 0 6px;
    font-size: 16px;
    color: var(--tint-ink);
  }

  .specs {
    margin: 0;
    font-size: 12px;
  }

  .hero,
  .verdict,
  .selected-release,
  .columns,
  .action {
    animation: rise 0.34s ease-out both;
  }

  .verdict {
    animation-delay: 0.05s;
  }

  .selected-release {
    animation-delay: 0.1s;
  }

  .columns {
    animation-delay: 0.14s;
  }

  .action {
    animation-delay: 0.18s;
  }

  @keyframes rise {
    from {
      opacity: 0;
      transform: translateY(6px);
    }
  }

  .verdict {
    background: var(--tint-soft);
    border-left: 3px solid var(--tint);
    border-radius: 0 4px 4px 0;
    padding: 16px 18px;
  }

  .verdict.attention {
    background: color-mix(in srgb, var(--alert) 8%, var(--surface));
    border-left-color: var(--alert);
  }

  .verdict-title {
    margin: 0;
    font-size: 19px;
    font-weight: 600;
    letter-spacing: -0.01em;
  }

  .verdict.attention .verdict-title {
    color: var(--alert);
  }

  .verdict-detail {
    margin: 2px 0 14px;
    color: var(--muted-ink);
  }

  .scale {
    display: grid;
    grid-template-columns: auto 1fr auto;
    align-items: center;
    gap: 5px 12px;
  }

  .rail {
    height: 9px;
    background: color-mix(in srgb, var(--ink) 8%, transparent);
    border-radius: 1px;
    overflow: hidden;
  }

  .bar {
    display: block;
    height: 100%;
    background: var(--tint);
    border-radius: 1px;
    transform-origin: left center;
    animation: fill 0.55s cubic-bezier(0.22, 0.7, 0.25, 1) both;
  }

  .bar.yours {
    animation-delay: 0.05s;
  }

  @keyframes fill {
    from {
      transform: scaleX(0);
    }
  }

  .bar.yours {
    background: color-mix(in srgb, var(--tint) 45%, var(--muted-ink));
    animation-delay: 0s;
  }

  .figure {
    font-size: 12px;
  }

  .selected-release {
    position: relative;
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 18px;
    flex-wrap: wrap;
    margin-top: 14px;
    padding: 12px 16px;
    background: var(--surface);
    border: 1px solid var(--line);
    border-radius: 4px;
  }

  .selected-release p {
    margin: 0;
  }

  .release-title {
    font-size: 17px;
    font-weight: 600;
    letter-spacing: -0.01em;
    margin-top: 2px;
  }

  .release-meta {
    font-size: 13px;
    margin-top: 1px;
  }

  .from-tags {
    display: inline-block;
    margin-left: 8px;
    padding: 1px 7px;
    border: 1px solid var(--tint);
    border-radius: 999px;
    font-size: 11px;
    font-weight: 400;
    color: var(--tint-ink);
    vertical-align: middle;
    white-space: nowrap;
  }

  .selected-release.open {
    border-radius: 4px 4px 0 0;
  }

  .change {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    border-color: var(--tint);
    color: var(--tint-ink);
    padding: 6px 11px;
    flex-shrink: 0;
  }

  .change::after {
    content: '';
    position: absolute;
    inset: 0;
  }

  .change:hover {
    background: var(--tint-soft);
  }

  .release-title a,
  .group-link {
    position: relative;
    z-index: 1;
  }

  .group-link {
    color: inherit;
    text-decoration: underline;
    text-decoration-color: var(--line);
    text-underline-offset: 2px;
  }

  .group-link:hover {
    color: var(--tint-ink);
    text-decoration-color: currentColor;
  }

  .change svg {
    transition: transform 0.16s ease;
  }

  .change.open svg {
    transform: rotate(180deg);
  }

  .warnings {
    margin-top: 14px;
    padding: 10px 14px;
    border-left: 2px solid var(--alert);
    background: color-mix(in srgb, var(--alert) 7%, var(--surface));
    border-radius: 0 3px 3px 0;
    font-size: 13px;
  }

  .warnings ul {
    margin: 2px 0 0;
    padding-left: 18px;
  }

  .changes {
    padding: 14px 16px;
    background: var(--surface);
    border: 1px solid var(--line);
    border-radius: 4px;
  }

  .section-label {
    margin: 14px 0 6px;
  }

  .section-label:first-child {
    margin-top: 0;
  }

  .replaced {
    display: grid;
    gap: 0;
    margin: 0;
  }

  .replaced > div {
    display: grid;
    grid-template-columns: minmax(120px, 170px) 1fr;
    gap: 4px 16px;
    align-items: baseline;
    padding: 4px 0;
    border-top: 1px solid var(--line);
  }

  .replaced dt {
    font-size: 13px;
    color: var(--muted-ink);
  }

  .replaced dd {
    margin: 0;
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 1px;
    word-break: break-word;
  }

  .added {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(230px, 1fr));
    gap: 2px 24px;
    margin: 0;
  }

  .added > div {
    display: flex;
    gap: 8px;
    align-items: baseline;
    font-size: 13px;
    padding: 2px 0;
  }

  .added dt {
    flex-shrink: 0;
  }

  .added dd {
    margin: 0;
    word-break: break-word;
  }

  .identifiers {
    display: grid;
    gap: 3px;
    margin: 0;
  }

  .identifiers > div {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 4px 10px;
    font-size: 13px;
  }

  .identifiers dd {
    margin: 0;
  }

  .identifiers code {
    display: inline-block;
    background: var(--surface-sunken);
    border: 1px solid var(--line);
    border-radius: 3px;
    padding: 1px 6px;
    font-size: 11px;
    color: var(--muted-ink);
    word-break: break-all;
    transition:
      border-color 0.12s ease,
      color 0.12s ease;
  }

  .identifiers a {
    text-decoration: none;
  }

  .identifiers a:hover code {
    border-color: var(--tint);
    color: var(--tint-ink);
  }

  .release-title a {
    color: inherit;
    text-decoration: none;
  }

  .release-title a:hover {
    color: var(--tint-ink);
    text-decoration: underline;
    text-underline-offset: 3px;
  }

  .tracks-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
  }

  .show-all {
    margin-top: 6px;
    font-size: 12px;
  }

  .track-tags {
    display: grid;
    gap: 0;
    margin: 0;
  }

  .track-tags > div {
    display: grid;
    grid-template-columns: minmax(110px, 150px) 1fr;
    gap: 4px 14px;
    align-items: baseline;
    padding: 2px 0;
    font-size: 13px;
  }

  .track-tags dt {
    color: var(--muted-ink);
  }

  .track-tags dd {
    margin: 0;
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 1px;
    word-break: break-word;
  }

  .track-tags dd.mono {
    font-family: var(--mono);
    font-size: 11px;
  }

  .track-tags .unchanged dd {
    color: var(--muted-ink);
  }

  .track-tags code {
    background: var(--surface-sunken);
    border: 1px solid var(--line);
    border-radius: 3px;
    padding: 1px 6px;
    font-size: 11px;
    color: var(--muted-ink);
    word-break: break-all;
  }

  .track-tags a {
    text-decoration: none;
  }

  .track-tags a:hover code {
    border-color: var(--tint);
    color: var(--tint-ink);
  }

  .tracks {
    list-style: none;
    margin: 6px 0 0;
    padding: 0;
    border-top: 1px solid var(--line);
  }

  .tracks li {
    border-bottom: 1px solid var(--line);
  }

  .tracks li:last-child {
    border-bottom: none;
  }

  .row {
    display: grid;
    grid-template-columns: 26px 1fr auto auto auto;
    gap: 12px;
    align-items: baseline;
    width: 100%;
    text-align: left;
    border: none;
    border-radius: 0;
    padding: 6px 0;
  }

  .row {
    transition: background 0.12s ease;
  }

  .row:hover {
    background: color-mix(in srgb, var(--tint) 7%, var(--surface));
  }

  .row:hover .rank {
    color: var(--tint-ink);
  }

  .queue-item {
    transition: background 0.12s ease;
  }

  .count {
    font-size: 11px;
    border: 1px solid var(--line);
    border-radius: 999px;
    padding: 0 7px;
  }

  .reveal {
    display: grid;
    grid-template-rows: 1fr;
  }

  .clip {
    min-height: 0;
    overflow: hidden;
  }

  .details {
    padding: 8px 12px 12px 38px;
    background: var(--surface);
  }

  .rank {
    font-size: 12px;
    color: var(--muted-ink);
  }

  .title {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
  }

  .old,
  .new {
    align-self: stretch;
    padding: 1px 6px 1px 4px;
    margin-left: -4px;
    border-radius: 2px;
  }

  .old::before,
  .new::before {
    display: inline-block;
    width: 1.1em;
    opacity: 0.75;
  }

  .old {
    color: var(--removed);
    background: var(--removed-bg);
  }

  .old::before {
    content: '\2212';
  }

  .new {
    color: var(--added);
    background: var(--added-bg);
  }

  .new::before {
    content: '+';
  }

  .removed {
    color: var(--muted-ink);
    font-style: italic;
  }

  .missing-track {
    color: var(--muted-ink);
    font-style: italic;
  }

  .time,
  .drift {
    font-size: 12px;
  }

  .drift {
    color: var(--alert);
  }

  .action {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 18px;
    margin-top: 24px;
    flex-wrap: wrap;
  }

  .destination {
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 1px;
    font-size: 13px;
    word-break: break-all;
  }

  .file-button {
    background: var(--tint);
    border-color: var(--tint);
    color: var(--on-tint, #fff);
    font-weight: 600;
    padding: 10px 22px;
    letter-spacing: 0.01em;
  }

  .file-button {
    transition:
      filter 0.15s ease,
      transform 0.1s ease;
  }

  .file-button:hover:not(:disabled) {
    filter: brightness(1.12);
  }

  .file-button:active:not(:disabled) {
    transform: translateY(1px);
  }

  .done,
  .panel {
    background: var(--surface);
    border: 1px solid var(--line);
    border-radius: 4px;
    padding: 16px 18px;
  }

  .done {
    border-color: var(--ok);
    animation: rise 0.3s ease-out both;
  }

  .done .verdict-title {
    color: var(--ok);
  }

  .done p,
  .panel p {
    margin: 2px 0 0;
  }

  .empty {
    padding: 24px 0;
  }

  @media (max-width: 760px) {
    .workbench {
      grid-template-columns: 1fr;
    }

    .queue {
      border-right: none;
      border-bottom: 1px solid var(--line);
      max-height: 30vh;
    }
  }
</style>
