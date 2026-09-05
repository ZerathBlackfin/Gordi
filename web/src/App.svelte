<script>
  import { onMount } from 'svelte'
  import * as api from './api.js'
  import TriageView from './TriageView.svelte'
  import SettingsPage from './SettingsPage.svelte'
  import FiledPage from './FiledPage.svelte'
  import Logo from './Logo.svelte'
  import { route, navigate } from './router.svelte.js'
  import { waiting } from './mb.svelte.js'
  import { t, setLanguage } from './i18n.svelte.js'
  import { reveal } from './reveal.js'
  import './theme.svelte.js'

  let status = $state(null)
  let albums = $state([])
  let error = $state('')
  let scanning = $state(false)
  let now = $state(Date.now())

  const onSettings = $derived(route.path === '/settings')
  const onFiled = $derived(route.path === '/filed')
  const onTriage = $derived(!onSettings && !onFiled)
  const prefetch = $derived(status?.mb ?? null)
  const waitSeconds = $derived(
    waiting.active ? Math.max(0, Math.round((now - waiting.since) / 1000)) : 0,
  )

  async function load() {
    try {
      ;[status, albums] = await Promise.all([api.getStatus(), api.getAlbums('pending')])
      setLanguage(status.lang)
      error = ''
    } catch (e) {
      error = e.message
    }
  }

  async function rescan() {
    scanning = true
    try {
      await api.scan()
      await load()
    } catch (e) {
      error = e.message
    } finally {
      scanning = false
    }
  }

  onMount(() => {
    load()
    const clockTimer = setInterval(load, 8000)
    const clock = setInterval(() => (now = Date.now()), 500)
    return () => {
      clearInterval(clockTimer)
      clearInterval(clock)
    }
  })
</script>

<header class="topbar">
  <a
    class="brand"
    href="/"
    onclick={(e) => (e.preventDefault(), navigate('/'))}
    aria-current={onTriage ? 'page' : undefined}
  >
    <Logo height={22} />
    <span class="wordmark">Gordi</span>
  </a>

  <div class="tools">
    {#if waiting.active}
      <span class="waiting" title={t('bar.waitingHint')}>
        {t('bar.waiting', { n: waitSeconds })}
      </span>
    {:else if prefetch && prefetch.prefetch_done < prefetch.prefetch_total}
      <span class="muted small">
        {t('bar.prepared', { done: prefetch.prefetch_done, total: prefetch.prefetch_total })}
      </span>
    {/if}

    {#if !onTriage}
      <button onclick={() => navigate('/')}>{t('bar.back')}</button>
    {:else}
      <button onclick={rescan} disabled={scanning || status?.scanning}>
        {scanning || status?.scanning ? t('bar.rescanning') : t('bar.rescan')}
      </button>
      <button onclick={() => navigate('/settings')}>{t('bar.settings')}</button>
    {/if}
  </div>
</header>

{#if error && onTriage}
  <div class="reveal" transition:reveal={{ duration: 180 }}>
    <div class="clip">
      <p class="error banner">{error}</p>
    </div>
  </div>
{/if}

<main class:settings={!onTriage}>
  {#if onSettings}
    <SettingsPage onchange={load} />
  {:else if onFiled}
    <FiledPage />
  {:else}
    <TriageView {albums} mode={status?.mode ?? 'move'} inbox={status?.input ?? ''} onchange={load} />
  {/if}
</main>

<style>
  .topbar {
    display: flex;
    align-items: center;
    gap: 16px;
    height: 46px;
    padding: 0 16px;
    border-bottom: 1px solid var(--line);
    background: var(--surface);
  }

  .brand {
    display: inline-flex;
    align-items: center;
    gap: 9px;
    font-weight: 600;
    letter-spacing: -0.01em;
    color: var(--ink);
    text-decoration: none;
  }

  /* "Gordi" has no descender, so its ink sits high; these recentre it. */
  .wordmark {
    font-size: 15px;
    line-height: 1;
    transform: translateY(0.7px);
  }

  .tools {
    margin-left: auto;
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .waiting {
    font-size: 12px;
    color: var(--tint-ink);
    border: 1px solid var(--line);
    border-radius: 999px;
    padding: 1px 10px;
  }

  .reveal {
    display: grid;
    grid-template-rows: 1fr;
  }

  .clip {
    min-height: 0;
    overflow: hidden;
  }

  .banner {
    margin: 0;
    border-radius: 0;
    border-left: none;
    border-bottom: 1px solid var(--line);
  }

  main {
    height: calc(100vh - 47px);
    min-height: 0;
  }

  main.settings {
    overflow-y: auto;
  }
</style>
