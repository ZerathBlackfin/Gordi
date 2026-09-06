<script>
  import { onMount } from 'svelte'
  import { paths, transform, viewBox } from './logo.js'
  import { show } from './loading.svelte.js'

  const [band, grooves, studs, rightCup, leftCup] = paths

  let { label = '', hint = '', height = 96, compact = false } = $props()

  onMount(() => (compact ? undefined : show()))
</script>

<div class="loading" class:compact role="status" aria-live="polite" style:--mark="{height}px">
  <span class="mark">
    <svg {viewBox} aria-hidden="true" focusable="false">
      <g {transform}>
        <path d={band} />
        <path d={grooves} />
        <path d={studs} />
        <path d={leftCup} />
        <path d={rightCup} />
      </g>
    </svg>

    <svg class="needle" {viewBox} aria-hidden="true" focusable="false">
      <g {transform}>
        <path d={grooves} />
      </g>
    </svg>
  </span>

  <div class="text">
    {#if label}
      <p class="label">{label}</p>
    {/if}
    {#if hint}
      <p class="hint muted">{hint}</p>
    {/if}
  </div>
</div>

<style>
  .loading {
    display: grid;
    align-content: center;
    justify-items: center;
    gap: 20px;
    min-height: calc(100vh - 150px);
    animation: appear 0.4s ease 0.15s both;
  }

  .loading.compact {
    min-height: 0;
    padding: clamp(28px, 7vh, 64px) 0;
  }

  @keyframes appear {
    from {
      opacity: 0;
    }
  }

  .mark {
    position: relative;
    display: block;
  }

  svg {
    display: block;
    height: var(--mark);
    width: auto;
    fill: var(--line);
  }

  .needle {
    position: absolute;
    top: 0;
    left: 0;
    fill: var(--brand);
    mask-image: linear-gradient(100deg, transparent 40%, #000 50%, transparent 60%);
    mask-size: 280% 100%;
    mask-repeat: no-repeat;
    animation: sweep 3.15s linear infinite;
  }

  @keyframes sweep {
    from {
      mask-position: 100% 0;
    }
    72%,
    to {
      mask-position: 0 0;
    }
  }

  .text {
    text-align: center;
  }

  .label {
    margin: 0;
    font-size: 15px;
  }

  .hint {
    margin: 3px 0 0;
    font-size: 12px;
  }

  @media (prefers-reduced-motion: reduce) {
    .needle {
      mask-image: none;
      opacity: 0.45;
      animation-name: none;
    }
  }
</style>
