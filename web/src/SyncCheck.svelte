<script>
  import * as api from './api.js'
  import { t } from './i18n.svelte.js'

  let { albumId, releaseId, dropped = $bindable([]) } = $props()

  let tracks = $state(null)
  let loading = $state(false)
  let error = $state('')
  let open = $state(false)
  let listening = $state(null)
  let at = $state(0)
  let span = $state(0)
  let paused = $state(true)
  let unplayable = $state('')
  let box = $state(null)

  const PATIENCE = 6000

  let player
  let frame

  $effect(() => {
    releaseId
    tracks = null
    open = false
    close()
  })

  const summary = $derived.by(() =>
    tracks && {
      total: tracks.length,
      found: tracks.filter((x) => x.found).length,
      synced: tracks.filter((x) => x.synced).length,
    },
  )

  const lines = $derived(tracks?.find((x) => x.source === listening)?.lines ?? [])

  const sung = $derived.by(() => {
    let index = -1
    while (index + 1 < lines.length && lines[index + 1].at <= at) index++
    return index
  })

  const toward = $derived.by(() => {
    const start = lines[sung]?.at ?? 0
    const next = lines[sung + 1]?.at ?? span
    if (!(next > start)) return 0
    return Math.min(1, Math.max(0, (at - start) / (next - start)))
  })

  $effect(() => {
    if (sung < 0 || !box) return
    box.querySelector(`[data-line="${sung}"]`)?.scrollIntoView({
      block: 'center',
      behavior: matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth',
    })
  })

  async function reveal() {
    open = true
    if (tracks || loading) return
    loading = true
    error = ''
    try {
      tracks = await api.getLyrics(albumId, releaseId)
    } catch (e) {
      error = e.message
    } finally {
      loading = false
    }
  }

  function follow() {
    at = player.currentTime
    frame = requestAnimationFrame(follow)
  }

  function stop() {
    cancelAnimationFrame(frame)
    player?.pause()
  }

  function close() {
    stop()
    listening = null
    at = 0
    span = 0
  }

  async function listen(track, from) {
    if (listening !== track.source) {
      close()
      listening = track.source
      unplayable = ''
    }
    at = from ?? at

    try {
      if (!player.src.endsWith(encodeURIComponent(track.source))) {
        player.src = api.audioURL(albumId, track.source)
        player.load()
        await new Promise((resolve, reject) => {
          const giveUp = setTimeout(() => reject(new Error('no metadata')), PATIENCE)
          const settle = (fn) => (e) => {
            clearTimeout(giveUp)
            fn(e)
          }
          player.addEventListener('loadedmetadata', settle(resolve), { once: true })
          player.addEventListener('error', settle(reject), { once: true })
        })
      }
      if (from != null) player.currentTime = from
      await player.play()
    } catch {
      player.removeAttribute('src')
      player.load()
      unplayable = track.source
      listening = null
    }
  }

  function toggle(source) {
    dropped = dropped.includes(source)
      ? dropped.filter((s) => s !== source)
      : [...dropped, source]
  }

  function clock(seconds) {
    const whole = Math.max(0, Math.floor(seconds || 0))
    return `${Math.floor(whole / 60)}:${String(whole % 60).padStart(2, '0')}`
  }

  function verdict(track) {
    if (track.instrumental) return 'instrumental'
    if (!track.found) return 'missing'
    if (track.refused) return 'refused'
    return 'plainOnly'
  }
</script>

<audio
  bind:this={player}
  preload="none"
  onloadedmetadata={() => (span = player.duration)}
  onplay={() => {
    paused = false
    follow()
  }}
  onpause={() => {
    paused = true
    cancelAnimationFrame(frame)
  }}
  onended={close}
></audio>

<section class="lyrics">
  <header>
    <div class="what">
      <p class="eyebrow">{t('lyrics.title')}</p>
      {#if summary}
        <p class="muted small tally">
          {t('lyrics.summary', summary)}{#if dropped.length}<span class="set-aside"
              >{t('lyrics.setAside', { n: dropped.length })}</span
            >{/if}
        </p>
      {:else}
        <p class="muted small tally">{t('lyrics.why')}</p>
      {/if}
    </div>
    <button
      class="check"
      class:calm={open}
      onclick={() => (open ? (open = false) : reveal())}
    >
      {open ? t('lyrics.hide') : t('lyrics.check')}
    </button>
  </header>

  {#if open}
    {#if loading}
      <p class="muted">{t('lyrics.loading')}</p>
    {:else if error}
      <p class="error">{error}</p>
    {:else if tracks}
      <ol>
        {#each tracks as track (track.source)}
          {@const off = dropped.includes(track.source)}
          {@const live = listening === track.source}
          <li class:off={off && !live} class:live>
            <span class="name">
              {track.title}
              {#if track.without_album}
                <span class="muted small caveat" title={t('lyrics.looseHint')}
                  >{t('lyrics.loose')}</span
                >
              {/if}
            </span>

            {#if live}
              <button class="play" onclick={() => (paused ? listen(track) : stop())}>
                <span class={paused ? 'triangle' : 'bars'} aria-hidden="true"></span>
                {paused ? t('lyrics.resume') : t('lyrics.pause')}
              </button>
              <span class="mono elapsed">{clock(at)} / {clock(span)}</span>
            {/if}

            {#if track.lines?.length}
              <span class="marks">
                {#each track.anchors as index (index)}
                  <button
                    class="mark"
                    title={t('lyrics.from')}
                    onclick={() => listen(track, track.lines[index].at)}
                  >
                    <span class="triangle" aria-hidden="true"></span>
                    <span class="mono">{clock(track.lines[index].at)}</span>
                  </button>
                {/each}
              </span>

              <span class="links">
                <button class="link" onclick={() => toggle(track.source)}>
                  {off ? t('lyrics.keep') : t('lyrics.drop')}
                </button>
                {#if live}
                  <button class="link" onclick={close}>{t('lyrics.close')}</button>
                {/if}
              </span>
            {:else}
              <span class="muted small state">{t(`lyrics.${verdict(track)}`)}</span>
            {/if}

            {#if live}
              <div class="stage">
                <div class="scroller" bind:this={box} tabindex="-1" aria-label={track.title}>
                  {#each lines as line, i (i)}
                    <p
                      class="line"
                      class:sung={i === sung}
                      class:past={i < sung}
                      aria-current={i === sung ? 'true' : undefined}
                      data-line={i}
                    >
                      {line.text}
                    </p>
                  {/each}
                </div>
                <div class="toward" aria-hidden="true">
                  <span style:transform={`scaleX(${toward})`}></span>
                </div>
              </div>
            {:else if unplayable === track.source}
              <p class="muted small aside">{t('lyrics.unplayable')}</p>
            {:else if off}
              <p class="muted small aside">{t('lyrics.droppedHint')}</p>
            {:else if track.refused}
              <p class="muted small aside">{t('lyrics.refusedHint')}</p>
            {/if}
          </li>
        {/each}
      </ol>
    {/if}
  {/if}
</section>

<style>
  .lyrics {
    margin-top: 26px;
  }

  header {
    display: flex;
    align-items: start;
    gap: 12px 16px;
    flex-wrap: wrap;
  }

  .what {
    flex: 1;
    min-width: 22ch;
  }

  header .eyebrow,
  .tally {
    margin: 0;
  }

  .check {
    font-weight: 600;
    padding: 7px 15px;
    color: var(--tint-ink);
    border-color: color-mix(in srgb, var(--tint) 45%, var(--line));
  }

  .check:hover {
    background: var(--tint-soft);
    border-color: var(--tint);
  }

  .check.calm {
    font-weight: 400;
    color: var(--muted-ink);
    border-color: var(--line);
  }

  .check.calm:hover {
    background: none;
    color: var(--ink);
    border-color: var(--tint);
  }

  .set-aside::before {
    content: ' · ';
  }

  ol {
    display: grid;
    grid-template-columns: 1fr auto auto;
    margin: 14px 0 0;
    padding: 0;
    list-style: none;
  }

  li {
    grid-column: 1 / -1;
    display: grid;
    grid-template-columns: subgrid;
    align-items: center;
    column-gap: 12px;
  }

  .links {
    display: flex;
    gap: 10px;
  }

  .state {
    grid-column: 2 / -1;
    justify-self: end;
  }

  .stage,
  .aside {
    grid-column: 1 / -1;
  }

  li {
    padding: 9px 2px;
    border-bottom: 1px solid var(--line);
  }

  li.live {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 12px;
  }

  li.live .stage {
    width: 100%;
  }

  li:last-child {
    border-bottom: 0;
  }

  li.off {
    opacity: 0.5;
  }

  li.live {
    padding: 11px 13px 13px;
    border-bottom: 0;
    border-radius: 9px;
    row-gap: 0;
    background: color-mix(in srgb, var(--tint) 13%, var(--surface));
    box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--tint) 30%, var(--line));
  }

  .name {
    flex: 1;
    min-width: 14ch;
  }

  .marks {
    display: flex;
    align-items: center;
    gap: 5px;
  }

  .state,
  .caveat {
    font-style: italic;
  }

  .caveat {
    margin-left: 7px;
  }

  .elapsed {
    font-size: 12px;
    color: var(--muted-ink);
    font-variant-numeric: tabular-nums;
  }

  .mark {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    padding: 3px 9px 3px 8px;
    border: 1px solid var(--line);
    border-radius: 999px;
    background: var(--surface);
    color: var(--muted-ink);
    font: inherit;
    font-size: 12px;
    cursor: pointer;
    transition:
      color 0.15s,
      border-color 0.15s;
  }

  .play {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 4px 13px 4px 11px;
    border: 0;
    border-radius: 999px;
    background: var(--tint-ink);
    color: var(--surface);
    font: inherit;
    font-size: 12px;
    cursor: pointer;
  }

  .play:hover {
    background: var(--ink);
  }

  .mark:hover {
    color: var(--ink);
    border-color: var(--tint-ink);
  }

  .triangle {
    width: 0;
    height: 0;
    border-left: 7px solid currentColor;
    border-top: 4.5px solid transparent;
    border-bottom: 4.5px solid transparent;
  }

  .bars {
    width: 7px;
    height: 9px;
    border-left: 2.5px solid currentColor;
    border-right: 2.5px solid currentColor;
  }

  .stage {
    --lyric-size: 16px;
    --lyric-leading: 1.4;
    --lyric-gap: 9px;
    --line-box: calc(var(--lyric-size) * var(--lyric-leading) + var(--lyric-gap));
    --band: calc(var(--line-box) * 5.5);
    margin-top: 9px;
    padding: 0 14px 11px;
    border-radius: 10px;
    background: var(--surface);
  }

  .scroller {
    height: var(--band);
    padding: calc((var(--band) - var(--line-box)) / 2) 0;
    overflow-y: auto;
    overscroll-behavior: contain;
    scrollbar-width: none;
    mask-image: linear-gradient(transparent, #000 34%, #000 66%, transparent);
  }

  .scroller::-webkit-scrollbar {
    display: none;
  }

  .scroller:focus-visible {
    outline: none;
  }

  .line {
    margin: 0 0 var(--lyric-gap);
    max-width: 56ch;
    font-size: var(--lyric-size);
    line-height: var(--lyric-leading);
    color: var(--muted-ink);
    opacity: 0.62;
    transform-origin: left center;
    transition:
      opacity 0.3s,
      color 0.3s,
      transform 0.3s;
  }

  .line.past {
    opacity: 0.38;
  }

  .line.sung {
    color: var(--ink);
    opacity: 1;
    font-weight: 600;
    transform: scale(1.18);
  }

  .toward {
    height: 3px;
    margin-top: 10px;
    border-radius: 3px;
    background: var(--line);
    overflow: hidden;
  }

  .toward span {
    display: block;
    height: 100%;
    background: var(--tint-ink);
    transform-origin: left;
  }

  .aside {
    margin: 5px 0 0;
  }

  .link {
    border: 0;
    background: none;
    padding: 0;
    font: inherit;
    font-size: 12px;
    color: var(--muted-ink);
    text-decoration: underline;
    text-underline-offset: 3px;
    cursor: pointer;
  }

  .link:hover {
    color: var(--ink);
  }

</style>
