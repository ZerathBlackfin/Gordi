<script>
  import * as api from './api.js'
  import { t } from './i18n.svelte.js'
  import { day, time } from './format.js'

  let entries = $state([])
  let total = $state(0)
  let counts = $state({ filed: 0, undone: 0 })
  let error = $state('')
  let loaded = $state(false)
  let undoing = $state('')
  let failed = $state('')

  let booted = false
  $effect(() => {
    if (booted) return
    booted = true
    load()
  })

  async function load() {
    try {
      const r = await api.getFiled(500)
      entries = r.entries
      total = r.total
      counts = { filed: r.filed, undone: r.undone }
    } catch (e) {
      error = e.message
    } finally {
      loaded = true
    }
  }

  async function undo(entry) {
    undoing = entry.id
    failed = ''
    try {
      await api.undoFiled(entry.id)
      await load()
    } catch (e) {
      failed = e.message
    } finally {
      undoing = ''
    }
  }

  // Grouped on the local day, the one the heading shows: keying on the UTC date
  // split a single local day across two groups.
  const days = $derived.by(() => {
    const out = []
    for (const entry of entries) {
      if (entry.undoes) continue
      const key = new Date(entry.date).toDateString()
      if (out.at(-1)?.key !== key) out.push({ key, date: entry.date, entries: [] })
      out.at(-1).entries.push(entry)
    }
    return out
  })

  const timeWidth = $derived(Math.max(0, ...entries.map((e) => time(e.date).length)))

  function undoneWhen(entry) {
    const sameDay = new Date(entry.undone_at).toDateString() === new Date(entry.date).toDateString()
    return sameDay ? time(entry.undone_at) : `${day(entry.undone_at)} ${time(entry.undone_at)}`
  }
</script>

<div class="page">
  <h1>{t('filed.title')}</h1>

  {#if error}
    <p class="error">{error}</p>
  {:else if !loaded}
    <p class="muted">{t('filed.loading')}</p>
  {:else if total === 0}
    <p class="muted">{t('filed.none')}</p>
  {:else}
    <p class="muted count">
      {t('filed.count', { n: counts.filed })}
      {#if counts.undone}· {t('filed.undoneCount', { n: counts.undone })}{/if}
    </p>

    {#if failed}
      <p class="error">{failed}</p>
    {/if}

    {#each days as group (group.key)}
      <section>
        <h2 class="eyebrow">{day(group.date)}</h2>
        <ul>
          {#each group.entries as entry (entry.date)}
            <li title={entry.destination} class:undone={!!entry.undone_at}>
              <span class="at mono muted" style:min-width={`${timeWidth}ch`}>{time(entry.date)}</span>
              <span class="album">{entry.album}</span>
              <span class="artist muted">{entry.artist}</span>
              {#if entry.undone_at}
                <span class="tracks undone-at small">
                  {t('filed.undoneAt', { when: undoneWhen(entry) })}
                </span>
              {:else}
                <span class="tracks muted small">{t('filed.tracks', { n: entry.tracks })}</span>
              {/if}
              {#if entry.undo}
                <button
                  class="undo"
                  onclick={() => undo(entry)}
                  disabled={!!undoing}
                  title={t('filed.undoHint')}
                >
                  {undoing === entry.id ? t('filed.undoing') : t('filed.undo')}
                </button>
              {/if}
            </li>
          {/each}
        </ul>
      </section>
    {/each}

    {#if entries.length < total}
      <p class="muted small">{t('filed.more', { n: total - entries.length })}</p>
    {/if}
  {/if}
</div>

<style>
  .page {
    max-width: 900px;
    margin: 0 auto;
    padding: 22px 16px 60px;
  }

  h1 {
    margin: 0;
    font-size: 22px;
    letter-spacing: -0.02em;
  }

  .count {
    margin: 4px 0 26px;
  }

  section {
    margin-bottom: 20px;
  }

  h2 {
    position: sticky;
    top: 0;
    z-index: 1;
    margin: 0 0 6px;
    padding: 7px 0;
    background: var(--bg);
  }

  ul {
    margin: 0;
    padding: 0;
    list-style: none;
    background: var(--surface);
    border: 1px solid var(--line);
    border-radius: 4px;
  }

  li {
    display: flex;
    align-items: baseline;
    gap: 10px;
    padding: 8px 14px;
    border-top: 1px solid var(--line);
    transition: background 0.12s ease;
  }

  li:first-child {
    border-top: none;
  }

  li:hover {
    background: color-mix(in srgb, var(--tint) 7%, var(--surface));
  }

  .at {
    flex-shrink: 0;
    font-size: 12px;
    text-align: right;
  }

  .album {
    font-weight: 600;
  }

  .album,
  .artist {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .artist {
    flex-shrink: 2;
  }

  .tracks {
    margin-left: auto;
    padding-left: 8px;
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
  }

  .undo {
    flex-shrink: 0;
    font-size: 12px;
    padding: 2px 9px;
  }

  li.undone .album {
    color: var(--muted-ink);
    text-decoration: line-through;
    text-decoration-color: var(--removed);
    text-decoration-thickness: 1.5px;
  }

  .undone-at {
    color: var(--removed);
  }

  @media (max-width: 560px) {
    li {
      flex-wrap: wrap;
    }

    .artist {
      flex-basis: 100%;
    }
  }
</style>
