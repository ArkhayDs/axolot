<script lang="ts" module>
  // One path list per icon: pathLength lets the CSS redraw any stroke with the same animation.
  const PATHS = {
    target: ['M12 3a9 9 0 1 0 0 18a9 9 0 1 0 0-18', 'M12 8a4 4 0 1 0 0 8a4 4 0 1 0 0-8', 'M12 12h.01'],
    flask: ['M9 3h6', 'M10 3v6.5L4.8 18a2 2 0 0 0 1.7 3h11a2 2 0 0 0 1.7-3L14 9.5V3', 'M7.5 15h9'],
    chat: ['M4 5h16v11H9l-5 4z', 'M8 9h8', 'M8 12.5h5'],
    list: ['M8 6h12', 'M8 12h12', 'M8 18h12', 'M4 6h.01', 'M4 12h.01', 'M4 18h.01'],
    bars: ['M4 20h16', 'M7 20v-7', 'M12 20V5', 'M17 20v-10'],
    sliders: ['M4 7h9', 'M17 7h3', 'M4 17h3', 'M11 17h9', 'M15 5v4', 'M9 15v4'],
    star: ['M12 3.5l2.6 5.4 5.9.8-4.3 4.1 1 5.9L12 16.9l-5.2 2.8 1-5.9L3.5 9.7l5.9-.8z'],
    bolt: ['M13 2.5L5 13.5h6l-1 8 8-11h-6z'],
    balance: ['M12 4v16', 'M8 20h8', 'M5 7h14', 'M5 7l-3 7a3.2 3.2 0 0 0 6 0z', 'M19 7l-3 7a3.2 3.2 0 0 0 6 0z'],
    check: ['M5 12.5l4.5 4.5L19 7'],
    cross: ['M6 6l12 12', 'M18 6L6 18'],
    play: ['M7 4.5v15l12-7.5z'],
    stop: ['M6.5 6.5h11v11h-11z'],
    download: ['M12 4v11', 'M7 11l5 5 5-5', 'M5 20h14'],
    upload: ['M12 16V5', 'M7 9l5-5 5 5', 'M5 20h14'],
    trash: ['M4 7h16', 'M9 7V4h6v3', 'M6 7l1 13h10l1-13'],
    doc: ['M6 3h8l4 4v14H6z', 'M14 3v4h4', 'M9 13h6', 'M9 17h6'],
    arrow: ['M4 12h15', 'M13 6l6 6-6 6'],
    cpu: ['M7 7h10v10H7z', 'M10 3v4', 'M14 3v4', 'M10 17v4', 'M14 17v4', 'M3 10h4', 'M3 14h4', 'M17 10h4', 'M17 14h4'],
    plug: ['M9 3v5', 'M15 3v5', 'M6 8h12v3a6 6 0 0 1-12 0z', 'M12 17v4'],
    alert: ['M12 4L2.5 20h19z', 'M12 10v4', 'M12 17h.01'],
    grade: ['M4 5h16v14H4z', 'M8 12l3 3 5-6'],
    refresh: ['M20 12a8 8 0 1 1-2.3-5.7', 'M20 4v4h-4'],
  } as const;

  export type IconName = keyof typeof PATHS;
</script>

<script lang="ts">
  let { name, size = 18, live = false }: { name: IconName; size?: number; live?: boolean } = $props();
</script>

<svg class="icon" class:icon-live={live} width={size} height={size} viewBox="0 0 24 24" aria-hidden="true">
  {#each PATHS[name] as d (d)}
    <path {d} pathLength="1" />
  {/each}
</svg>

<style>
  .icon {
    flex: none;
    fill: none;
    stroke: currentColor;
    stroke-width: 1.75;
    stroke-linecap: round;
    stroke-linejoin: round;
  }
</style>
