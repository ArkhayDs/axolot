<script lang="ts">
  let { size = 32, active = false }: { size?: number; active?: boolean } = $props();

  // One stalk plus its fronds per gill: the feathery silhouette is what makes it read as an axolotl.
  const GILLS = [
    'M17 27C13 21 9 17 3 13M12 21L12 13M8 17L3 19',
    'M14 33C10 32 6 31 1 32M9 32L5 26M9 32L5 38',
    'M17 39C13 45 9 48 4 53M12 45L4 45M9 48L11 56',
  ];
</script>

<svg class="axolotl" class:active width={size} height={size} viewBox="0 0 64 64" aria-hidden="true">
  <g class="gills gills-left">
    {#each GILLS as d (d)}
      <path {d} />
    {/each}
  </g>
  <g class="gills gills-right">
    {#each GILLS as d (d)}
      <path {d} transform="matrix(-1 0 0 1 64 0)" />
    {/each}
  </g>
  <path class="head" d="M32 17C45 17 53 25 53 35C53 45 44 51 32 51C20 51 11 45 11 35C11 25 19 17 32 17Z" />
  <ellipse class="cheek" cx="19" cy="41" rx="4" ry="2.6" />
  <ellipse class="cheek" cx="45" cy="41" rx="4" ry="2.6" />
  <circle class="eye" cx="21" cy="32" r="3.6" />
  <circle class="eye" cx="43" cy="32" r="3.6" />
  <circle class="glint" cx="22.2" cy="30.8" r="1.1" />
  <circle class="glint" cx="44.2" cy="30.8" r="1.1" />
  <path class="mouth" d="M23 39C27 44 37 44 41 39" />
</svg>

<style>
  .axolotl {
    flex: none;
  }

  .head {
    fill: var(--axo-skin);
  }

  .cheek {
    fill: var(--accent);
    fill-opacity: 0.55;
  }

  .eye {
    fill: var(--axo-eye);
  }

  .glint {
    fill: var(--axo-skin);
  }

  .mouth {
    fill: none;
    stroke: var(--axo-eye);
    stroke-linecap: round;
    stroke-width: 2.4;
  }

  .gills {
    fill: none;
    stroke: var(--accent);
    stroke-linecap: round;
    stroke-linejoin: round;
    stroke-width: 3;
    transform-box: view-box;
    animation: sway 5s ease-in-out infinite alternate;
  }

  .gills-left {
    transform-origin: 17px 33px;
  }

  .gills-right {
    transform-origin: 47px 33px;
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
