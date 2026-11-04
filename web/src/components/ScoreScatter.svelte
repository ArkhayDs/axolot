<script lang="ts">
  import type { ModelStats } from '../lib/api';
  import { categoryLabel, duration, score, seriesColor } from '../lib/format';

  let { stats, models }: { stats: ModelStats[]; models: string[] } = $props();

  const HEIGHT = 380;
  const MARGIN = { top: 14, right: 24, bottom: 44, left: 44 };
  const TIME_TICKS_MS = [100, 200, 500, 1000, 2000, 5000, 10_000, 20_000, 50_000, 100_000];
  const SCORE_TICKS = [0, 25, 50, 75, 100];
  // Under this score a task tells a story, so it earns a direct label.
  const LABEL_BELOW = 90;
  const LABEL_ROOM = 150;

  let width = $state(0);
  let hovered = $state<ModelStats | null>(null);

  const points = $derived(stats.filter((stat) => stat.p50_total_ms !== null && stat.p50_total_ms > 0));
  const times = $derived(points.map((stat) => stat.p50_total_ms as number));
  const minTime = $derived(Math.min(...times, 1000));
  const maxTime = $derived(Math.max(...times, 1000));
  const ticks = $derived(TIME_TICKS_MS.filter((tick) => tick >= minTime * 0.5 && tick <= maxTime * 2));
  const logMin = $derived(Math.log10(ticks[0] ?? minTime));
  const logMax = $derived(Math.log10(ticks.at(-1) ?? maxTime));

  /**
   * @param {number} ms - duration
   * @returns {number} - horizontal position, log scale: times span two orders of magnitude
   */
  function x(ms: number): number {
    const ratio = (Math.log10(ms) - logMin) / Math.max(logMax - logMin, 0.001);
    return MARGIN.left + ratio * (width - MARGIN.left - MARGIN.right);
  }

  /**
   * @param {number} value - score
   * @returns {number} - vertical position
   */
  function y(value: number): number {
    return MARGIN.top + (1 - value / 100) * (HEIGHT - MARGIN.top - MARGIN.bottom);
  }

  // One label per task, on its weakest point: the segment links it to the other model.
  const labelled = $derived(
    new Set(
      [...new Set(points.map((stat) => stat.category))].flatMap((category) => {
        const pair = points.filter((stat) => stat.category === category);
        const weakest = pair.reduce((low, stat) => (stat.avg_score < low.avg_score ? stat : low));
        return weakest.avg_score < LABEL_BELOW ? [weakest] : [];
      }),
    ),
  );

  // One segment per task between its models: its slope is the price of speed on that task.
  const links = $derived(
    [...new Set(points.map((stat) => stat.category))].flatMap((category) => {
      const pair = points.filter((stat) => stat.category === category);
      return pair.slice(1).map((stat) => ({ category, from: pair[0], to: stat }));
    }),
  );
</script>

<figure class="panel scatter" bind:clientWidth={width}>
  <figcaption>
    <h3>Qualité et temps de réponse</h3>
    <p class="muted">Un point par modèle et par tâche. En haut à gauche : juste et rapide.</p>
  </figcaption>
  {#if width > 0}
    <div class="plot">
      <svg {width} height={HEIGHT} role="img" aria-label="Score moyen selon le temps médian, repris dans le tableau ci-dessous">
        {#each SCORE_TICKS as tick (tick)}
          <line class="grid" x1={MARGIN.left} x2={width - MARGIN.right} y1={y(tick)} y2={y(tick)} />
          <text class="tick" x={MARGIN.left - 8} y={y(tick) + 4} text-anchor="end">{tick}</text>
        {/each}
        {#each ticks as tick (tick)}
          <line class="grid" x1={x(tick)} x2={x(tick)} y1={MARGIN.top} y2={HEIGHT - MARGIN.bottom} />
          <text class="tick" x={x(tick)} y={HEIGHT - MARGIN.bottom + 18} text-anchor="middle">{duration(tick)}</text>
        {/each}
        <text class="axis-title" x={(width + MARGIN.left - MARGIN.right) / 2} y={HEIGHT - 6} text-anchor="middle">Temps médian par réponse, échelle logarithmique</text>

        {#each links as link (link.category + link.to.model)}
          <line
            class="link"
            x1={x(link.from.p50_total_ms as number)}
            y1={y(link.from.avg_score)}
            x2={x(link.to.p50_total_ms as number)}
            y2={y(link.to.avg_score)}
          />
        {/each}

        {#each points as stat (stat.model + stat.category)}
          {@const cx = x(stat.p50_total_ms as number)}
          {@const cy = y(stat.avg_score)}
          <!-- svelte-ignore a11y_no_noninteractive_tabindex: keyboard focus shows the same readout as hover -->
          <g
            class="point"
            tabindex="0"
            role="img"
            aria-label="{categoryLabel(stat.category)}, {stat.model} : {score(stat.avg_score)} sur 100, {duration(stat.p50_total_ms)}"
            onpointerenter={() => (hovered = stat)}
            onpointerleave={() => (hovered = null)}
            onfocus={() => (hovered = stat)}
            onblur={() => (hovered = null)}
          >
            <circle class="hit" {cx} {cy} r="13" />
            <circle class="mark" {cx} {cy} r={stat.category === '' ? 8 : 5.5} fill={seriesColor(stat.model, models)} />
          </g>
          {#if labelled.has(stat)}
            {@const nearEdge = cx > width - LABEL_ROOM}
            <text class="point-label" x={nearEdge ? cx - 10 : cx + 10} y={cy - 9} text-anchor={nearEdge ? 'end' : 'start'}>{categoryLabel(stat.category)}</text>
          {/if}
        {/each}
      </svg>
      {#if hovered}
        <div class="tip" style:left="{x(hovered.p50_total_ms as number)}px" style:top="{y(hovered.avg_score)}px">
          <strong>{categoryLabel(hovered.category)}</strong><br />
          {hovered.model}<br />
          {score(hovered.avg_score)}/100, {duration(hovered.p50_total_ms)}
        </div>
      {/if}
    </div>
  {/if}
</figure>

<style>
  .scatter {
    display: grid;
    gap: var(--s3);
    margin: 0;
    min-width: 0;
  }

  figcaption p {
    margin-top: var(--s1);
    font-size: var(--text-sm);
  }

  .plot {
    position: relative;
  }

  svg {
    display: block;
    overflow: visible;
  }

  .grid {
    stroke: var(--hairline);
  }

  .tick {
    fill: var(--ink-3);
    font-size: 12px;
    font-variant-numeric: tabular-nums;
  }

  .axis-title {
    fill: var(--ink-2);
    font-size: 12px;
  }

  .link {
    stroke: var(--axis);
    stroke-width: 1.5;
  }

  .hit {
    fill: transparent;
  }

  .mark {
    stroke: var(--panel);
    stroke-width: 2;
  }

  .point {
    outline: none;
  }

  .point:focus-visible .mark {
    stroke: var(--accent);
  }

  .point-label {
    fill: var(--ink-2);
    font-size: 12px;
    pointer-events: none;
  }

  .tip {
    position: absolute;
    z-index: 2;
    width: max-content;
    max-width: 16rem;
    padding: var(--s2) var(--s3);
    border: 1px solid var(--axis);
    border-radius: var(--radius-sm);
    background: var(--raised);
    box-shadow: var(--shadow-float);
    font-size: var(--text-sm);
    overflow-wrap: anywhere;
    pointer-events: none;
    transform: translate(-50%, calc(-100% - 12px));
  }
</style>
