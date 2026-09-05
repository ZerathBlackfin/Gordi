<script>
  import { paths, transform, viewBox } from './logo.js'

  let { height = 22 } = $props()
</script>

<svg {viewBox} style:height="{height}px" aria-hidden="true" focusable="false">
  <g {transform}>
    {#each paths as d}
      <path {d} />
    {/each}
  </g>
</svg>

<style>
  svg {
    display: block;
    width: auto;
    fill: var(--brand);
  }
</style>
