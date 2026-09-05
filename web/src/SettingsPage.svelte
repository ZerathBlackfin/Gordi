<script>
  import { fly } from 'svelte/transition'
  import { cubicOut } from 'svelte/easing'

  import * as api from './api.js'
  import { theme, choose, OPTIONS as THEMES } from './theme.svelte.js'
  import { t, setLanguage } from './i18n.svelte.js'
  import { day } from './format.js'
  import { navigate } from './router.svelte.js'

  let { onchange } = $props()

  let settings = $state(null)
  let draft = $state(null)
  let preview = $state([])
  let error = $state('')
  let busy = $state(false)
  let message = $state('')
  let filed = $state(null)

  const dirty = $derived(
    settings !== null &&
      Object.keys(draft ?? {}).some((c) => String(draft[c]) !== String(settings[c])),
  )

  let booted = false
  $effect(() => {
    if (!booted) {
      booted = true
      load()
    }
  })

  async function load() {
    try {
      const r = await api.getSettings()
      settings = r
      draft = {
        pattern: r.pattern,
        pattern_multi: r.pattern_multi,
        mode: r.mode,
        lang: r.lang,
      }
      setLanguage(r.lang)
      preview = r.preview
      filed = await api.getFiled(1)
      error = ''
    } catch (e) {
      error = e.message
    }
  }

  let timer
  function refresh() {
    clearTimeout(timer)
    timer = setTimeout(async () => {
      try {
        const r = await api.previewPattern({
          pattern: draft.pattern,
          pattern_multi: draft.pattern_multi,
        })
        preview = r.preview
        error = ''
      } catch (e) {
        preview = []
        error = e.message
      }
    }, 250)
  }

  function applyPreset(m) {
    draft.pattern = m.pattern
    draft.pattern_multi = m.pattern_multi
    refresh()
  }

  async function save() {
    busy = true
    message = ''
    try {
      settings = await api.saveSettings(draft)
      setLanguage(settings.lang)
      preview = settings.preview
      error = ''
      message = t('settings.saved')
      onchange?.()
    } catch (e) {
      error = e.message
    } finally {
      busy = false
    }
  }

  async function resetToServer() {
    busy = true
    try {
      await api.saveSettings({ pattern: '', pattern_multi: '' })
      await load()
      onchange?.()
    } catch (e) {
      error = e.message
    } finally {
      busy = false
    }
  }

  async function clearCache() {
    busy = true
    try {
      const r = await api.clearCache()
      message = t('settings.cacheCleared', { n: r.cleared })
      await load()
    } catch (e) {
      error = e.message
    } finally {
      busy = false
    }
  }

  const customized = $derived(
    (settings?.customized ?? []).some((c) => c === 'pattern' || c === 'pattern_multi'),
  )
</script>

<div class="page">
  {#if !settings}
    <p class="muted">{error || t('settings.loading')}</p>
  {:else}
    <h1>{t('settings.title')}</h1>

    <section>
      <h2>{t('settings.naming')}</h2>
      <p class="muted hint">{t('settings.namingHint')}</p>

      <p class="variables mono">
        {#each settings.fields as field (field)}<span>{field}</span>{/each}
      </p>
      <p class="muted hint">{settings.hint}</p>

      <div class="presets">
        {#each settings.templates as m (m.pattern)}
          <button
            class="preset"
            class:active={draft.pattern === m.pattern && draft.pattern_multi === m.pattern_multi}
            onclick={() => applyPreset(m)}
          >
            {m.name}
          </button>
        {/each}
      </div>

      <label class="pattern">
        <span class="eyebrow">{t('settings.singleDisc')}</span>
        <input class="mono" bind:value={draft.pattern} oninput={refresh} spellcheck="false" />
      </label>

      <label class="pattern">
        <span class="eyebrow">{t('settings.multiDisc')}</span>
        <input
          class="mono"
          bind:value={draft.pattern_multi}
          oninput={refresh}
          spellcheck="false"
        />
      </label>

      {#if preview.length}
        <div class="examples">
          <span class="eyebrow">{t('settings.preview')}</span>
          {#each preview as e (e.case)}
            <span class="example">
              <span class="case muted">{e.case}</span>
              <span class="path mono">{e.path}</span>
            </span>
          {/each}
        </div>
      {/if}

      {#if customized}
        <button class="quiet" onclick={resetToServer}>{t('settings.reset')}</button>
      {/if}
    </section>

    <section>
      <h2>{t('settings.originals')}</h2>
      <div class="segmented">
        {#each ['move', 'copy'] as v (v)}
          <label class:active={draft.mode === v}>
            <input
              class="sr"
              type="radio"
              name="mode"
              checked={draft.mode === v}
              onchange={() => (draft.mode = v)}
            />
            <span>{v === 'move' ? t('settings.move') : t('settings.copy')}</span>
            <span class="muted small">
              {v === 'move' ? t('settings.moveHint') : t('settings.copyHint')}
            </span>
          </label>
        {/each}
      </div>
    </section>

    <section>
      <h2>{t('settings.language')}</h2>
      <p class="muted hint">{t('settings.languageHint')}</p>
      <div class="segmented">
        {#each settings.languages as l (l.code)}
          <label class:active={draft.lang === l.code}>
            <input
              class="sr"
              type="radio"
              name="lang"
              checked={draft.lang === l.code}
              onchange={() => (draft.lang = l.code)}
            />
            <span>{l.name}</span>
          </label>
        {/each}
      </div>
    </section>

    <section>
      <h2>{t('settings.appearance')}</h2>
      <div class="segmented">
        {#each THEMES as themeChoice (themeChoice.value)}
          <label class:active={theme.choice === themeChoice.value}>
            <input
              class="sr"
              type="radio"
              name="theme"
              checked={theme.choice === themeChoice.value}
              onchange={() => choose(themeChoice.value)}
            />
            <span>{t(`theme.${themeChoice.value}`)}</span>
          </label>
        {/each}
      </div>
    </section>

    <section>
      <h2>{t('settings.filed')}</h2>
      {#if !filed || filed.total === 0}
        <p class="muted hint">{t('settings.filedNone')}</p>
      {:else}
        <p class="muted hint">
          {t('settings.filedSummary', { n: filed.total, date: day(filed.entries[0].date) })} ·
          <button class="quiet" onclick={() => navigate('/filed')}>{t('settings.filedOpen')}</button>
        </p>
      {/if}
    </section>

    {#if error}
      <p class="error">{error}</p>
    {/if}

    <p class="maintenance muted">
      {t('settings.cache', { n: settings.cache })} ·
      <button class="quiet" onclick={clearCache} disabled={busy || settings.cache === 0}>
        {t('settings.clear')}
      </button>
    </p>
  {/if}
</div>

{#if settings && (dirty || message)}
  <div class="savebar" transition:fly={{ y: 44, duration: 200, easing: cubicOut }}>
    {#if dirty}
      <span class="muted small">{t('settings.unsaved')}</span>
      <button onclick={load} disabled={busy}>{t('settings.cancel')}</button>
      <button class="primary" onclick={save} disabled={busy || !preview.length}>
        {t('settings.save')}
      </button>
    {:else}
      <span class="done">{message}</span>
    {/if}
  </div>
{/if}

<style>
  .page {
    max-width: 680px;
    margin: 0 auto;
    padding: 30px 20px 96px;
  }

  h1 {
    margin: 0 0 26px;
    font-size: 26px;
    font-weight: 600;
    letter-spacing: -0.02em;
  }

  section {
    padding: 22px 0;
    border-top: 1px solid var(--line);
  }

  h2 {
    margin: 0 0 4px;
    font-size: 15px;
    font-weight: 600;
  }

  .hint {
    margin: 0 0 10px;
    font-size: 13px;
  }

  .variables {
    display: flex;
    flex-wrap: wrap;
    gap: 5px;
    margin: 10px 0 6px;
    font-size: 12px;
  }

  .variables span {
    border: 1px solid var(--line);
    border-radius: 2px;
    padding: 1px 7px;
    color: var(--tint-ink);
  }

  .presets {
    display: flex;
    flex-wrap: wrap;
    gap: 5px;
    margin: 12px 0 4px;
  }

  .preset {
    font-size: 12px;
    padding: 3px 10px;
    border-radius: 999px;
  }

  .preset.active {
    border-color: var(--tint);
    color: var(--tint-ink);
  }

  .pattern {
    display: flex;
    flex-direction: column;
    gap: 5px;
    margin: 16px 0;
  }

  .pattern input {
    width: 100%;
    font-size: 13px;
  }

  .examples {
    display: grid;
    gap: 7px;
    margin: 18px 0 4px;
    padding: 13px 15px;
    background: var(--surface);
    border: 1px solid var(--line);
    border-radius: 4px;
  }

  /* The case is prose, the result is a path: they get their own column each, so
     the paths line up under one another. */
  .example {
    display: grid;
    grid-template-columns: 128px minmax(0, 1fr);
    gap: 2px 14px;
    font-size: 12px;
    line-height: 1.5;
  }

  .case {
    text-align: left;
  }

  .path {
    overflow-wrap: anywhere;
  }

  @media (max-width: 620px) {
    .example {
      grid-template-columns: minmax(0, 1fr);
      gap: 0;
    }
  }

  .segmented {
    display: inline-flex;
    border: 1px solid var(--line);
    border-radius: 4px;
    padding: 3px;
    gap: 3px;
    margin-top: 8px;
    background: var(--surface);
  }

  .segmented label {
    display: flex;
    flex-direction: column;
    border-radius: 3px;
    padding: 6px 14px;
    cursor: pointer;
    color: var(--muted-ink);
  }

  .segmented label.active {
    background: var(--tint-soft);
    color: var(--tint-ink);
  }

  .segmented label:has(.sr:focus-visible) {
    outline: 2px solid var(--tint-ink);
    outline-offset: 1px;
  }

  .sr {
    position: absolute;
    width: 1px;
    height: 1px;
    padding: 0;
    margin: -1px;
    overflow: hidden;
    clip-path: inset(50%);
  }

  dl {
    display: grid;
    gap: 8px;
    margin: 10px 0;
  }

  dd {
    margin: 1px 0 0;
    word-break: break-all;
  }

  .quiet {
    border: none;
    padding: 0;
    color: var(--tint-ink);
    text-decoration: underline;
    text-underline-offset: 3px;
  }

  .maintenance {
    margin-top: 30px;
    padding-top: 14px;
    border-top: 1px dashed var(--line);
    font-size: 13px;
  }

  .savebar {
    position: fixed;
    inset: auto 0 0 0;
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 10px;
    padding: 10px 20px;
    background: var(--surface);
    border-top: 1px solid var(--line);
  }

  .primary {
    background: var(--tint);
    border-color: var(--tint);
    color: #fff;
    font-weight: 600;
  }

  .done {
    color: var(--ok);
    font-size: 13px;
  }
</style>
