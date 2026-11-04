<script lang="ts">
  let { size = 32, active = false }: { size?: number; active?: boolean } = $props();

  const LEFT_GILLS = ['M18 26C13 22 9 21 4 15', 'M15 33C10 33 6 33 1 30', 'M18 40C13 43 9 45 4 52'];
  const RIGHT_GILLS = ['M46 26C51 22 55 21 60 15', 'M49 33C54 33 58 33 63 30', 'M46 40C51 43 55 45 60 52'];
</script>

<svg class="axolotl" class:active width={size} height={size} viewBox="0 0 64 64" aria-hidden="true">
  <g class="gills gills-left">
    {#each LEFT_GILLS as d (d)}
      <path {d} />
    {/each}
  </g>
  <g class="gills gills-right">
    {#each RIGHT_GILLS as d (d)}
      <path {d} />
    {/each}
  </g>
  <ellipse class="head" cx="32" cy="36" rx="19" ry="16" />
  <circle class="eye" cx="24" cy="33" r="3" />
  <circle class="eye" cx="40" cy="33" r="3" />
  <path class="mouth" d="M26 41q6 5 12 0" />
</svg>

<style>
  .axolotl {
    flex: none;
  }

  .head {
    fill: var(--axo-skin);
  }

  .eye {
    fill: var(--axo-eye);
  }

  .mouth {
    fill: none;
    stroke: var(--axo-eye);
    stroke-linecap: round;
    stroke-width: 2.5;
  }

  .gills {
    fill: none;
    stroke: var(--accent);
    stroke-linecap: round;
    stroke-width: 4;
    transform-box: view-box;
    animation: sway 5s ease-in-out infinite alternate;
  }

  .gills-left {
    transform-origin: 18px 33px;
  }

  .gills-right {
    transform-origin: 46px 33px;
    animation-direction: alternate-reverse;
  }

  .active .gills {
    animation-duration: 0.9s;
  }

  @keyframes sway {
    from {
      transform: rotate(-5deg);
    }
    to {
      transform: rotate(5deg);
    }
  }
</style>
