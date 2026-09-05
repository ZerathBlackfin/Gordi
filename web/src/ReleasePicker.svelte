<script>

  import * as api from './api.js'
  import { reveal } from './reveal.js'
  import { t } from './i18n.svelte.js'

  let { releases = [], selected = null, expectedTracks = 0, albumId, onselect, attached = false } = $props()

  const FORMATS = ['', 'cd', 'digital', 'vinyl', 'cassette']

  const EMPTY = { country: '', format: '', year_min: '', year_max: '' }

  let list = $state(releases)
  let draft = $state({ ...EMPTY })
  let applied = $state({ ...EMPTY })
  let busy = $state(false)
  let error = $state('')

  const dirty = $derived(
    Object.keys(EMPTY).some((c) => String(draft[c] ?? '') !== String(applied[c] ?? '')),
  )

  const countries = $derived(
    [...new Set(['FR', 'GB', 'US', 'DE', 'JP', 'XW', 'XE', ...list.map((e) => e.country)])]
      .filter(Boolean)
      .sort(),
  )

  async function filter() {
    busy = true
    error = ''
    try {
      const p = await api.getCandidates(albumId, draft)
      list = p.releases ?? []
      applied = { ...draft }
    } catch (e) {
      error = e.message
    } finally {
      busy = false
    }
  }

  function describe(e) {
    return [e.date?.slice(0, 4), e.country, e.status, e.format, e.packaging]
      .filter((v) => v && v !== 'None')
      .join(' · ')
  }
</script>

<div class="reveal" transition:reveal>
  <div class="clip">
    <div class="picker" class:attached>
  <div class="filters">
    <select bind:value={draft.country} aria-label={t('picker.country')}>
      <option value="">{t('picker.allCountries')}</option>
      {#each countries as c (c)}<option value={c}>{c}</option>{/each}
    </select>

    <select bind:value={draft.format} aria-label={t('picker.format')}>
      {#each FORMATS as v (v)}
        <option value={v}>{v ? t(`picker.${v}`) : t('picker.allFormats')}</option>
      {/each}
    </select>

    <input
      type="number"
      placeholder={t('picker.from')}
      min="1900"
      max="2100"
      bind:value={draft.year_min}
    />
    <input
      type="number"
      placeholder={t('picker.to')}
      min="1900"
      max="2100"
      bind:value={draft.year_max}
    />

    <button onclick={filter} disabled={!dirty || busy}>
      {busy ? t('picker.searching') : t('picker.filter')}
    </button>
  </div>

  {#if error}
    <p class="error">{error}</p>
  {:else if list.length === 0}
    <p class="muted empty">{t('picker.none')}</p>
  {/if}

  <ul>
    {#each list as e (e.id)}
      <li>
        <button class="release" class:selected={e.id === selected} onclick={() => onselect(e)}>
          <span class="title">
            {e.title}{#if e.disambiguation}<span class="muted"> ({e.disambiguation})</span>{/if}{#if e.from_tags}<span
                class="from-tags">{t('release.fromTags')}</span>{/if}
          </span>
          <span class="tracks mono" class:exact={e.track_count === expectedTracks}>
            {e.track_count}
          </span>
          <span class="muted meta">{describe(e)}</span>
          {#if e.label || e.catalog}
            <span class="muted meta mono">{[e.label, e.catalog].filter(Boolean).join(' · ')}</span>
          {/if}
        </button>
        <a
          class="mb-link"
          href={`https://musicbrainz.org/release/${e.id}`}
          target="_blank"
          rel="noreferrer"
          title={t('release.openMB')}
          aria-label={t('release.openMB')}
        >
          <svg viewBox="0 0 24 24" width="13" height="13" aria-hidden="true">
            <path
              d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6M15 3h6v6M10 14 21 3"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
              stroke-linecap="round"
              stroke-linejoin="round"
            />
          </svg>
        </a>
      </li>
    {/each}
      </ul>
    </div>
  </div>
</div>

<style>
  .reveal {
    display: grid;
    grid-template-rows: 1fr;
    margin-bottom: 6px;
  }

  .clip {
    min-height: 0;
    overflow: hidden;
  }

  .picker {
    border: 1px solid var(--line);
    border-radius: 4px;
    background: var(--surface);
    padding: 12px;
  }

  .picker.attached {
    border-top: none;
    border-radius: 0 0 4px 4px;
    padding: 12px 16px;
  }

  .filters {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    margin-bottom: 10px;
  }

  .filters input {
    width: 74px;
  }

  ul {
    list-style: none;
    margin: 0;
    padding: 0;
    max-height: 300px;
    overflow-y: auto;
    display: grid;
    gap: 4px;
  }

  li {
    position: relative;
  }

  .release {
    display: grid;
    grid-template-columns: 1fr auto;
    gap: 1px 12px;
    width: 100%;
    text-align: left;
    border: none;
    border-radius: 3px;
    padding: 8px 34px 8px 10px;
  }

  .mb-link {
    position: absolute;
    top: 50%;
    right: 7px;
    transform: translateY(-50%);
    display: flex;
    padding: 5px;
    border-radius: 3px;
    color: var(--muted-ink);
    opacity: 0.5;
    transition:
      opacity 0.12s ease,
      color 0.12s ease,
      background 0.12s ease;
  }

  li:hover .mb-link {
    opacity: 0.75;
  }

  .mb-link:hover,
  .mb-link:focus-visible {
    opacity: 1;
    color: var(--tint-ink);
    background: var(--tint-soft);
  }

  .release:hover {
    background: color-mix(in srgb, var(--tint) 7%, var(--surface));
  }

  .release:hover .title {
    color: var(--tint-ink);
  }

  .release.selected {
    background: var(--tint-soft);
    box-shadow: inset 2px 0 0 var(--tint);
  }

  .release.selected:hover {
    background: color-mix(in srgb, var(--tint) 26%, var(--surface));
  }

  .title {
    font-weight: 500;
  }

  .tracks {
    grid-row: 1 / 4;
    grid-column: 2;
    align-self: center;
    font-size: 12px;
    color: var(--muted-ink);
  }

  .tracks.exact {
    color: var(--tint-ink);
    font-weight: 600;
  }

  .meta {
    grid-column: 1;
    font-size: 12px;
  }

  .empty {
    padding: 6px 0;
  }

  .from-tags {
    margin-left: 8px;
    padding: 1px 7px;
    border: 1px solid var(--tint);
    border-radius: 999px;
    font-size: 11px;
    font-weight: 400;
    color: var(--tint-ink);
    white-space: nowrap;
  }
</style>
