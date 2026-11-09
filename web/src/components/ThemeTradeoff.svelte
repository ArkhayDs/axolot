<script lang="ts">
  import type { ModelStats } from '../lib/api';
  import { categoryLabel, duration, score, seriesColor } from '../lib/format';

  let { stats, models }: { stats: ModelStats[]; models: string[] } = $props();

  const LABEL_WIDTH = 170;
  const RIGHT_MARGIN = 44;
  const TOP_MARGIN = 8;
  const ROW_HEIGHT = 46;
  const AXIS_HEIGHT = 48;
  const TICKS_MS = [100, 200, 500, 1000, 2000, 5000, 10_000, 20_000, 50_000, 100_000];
  // Under this score the number is flagged: it is what separates the models.
  const WEAK_SCORE = 60;
  // Two score labels closer than this would collide on one row.
  const LABEL_GAP = 44;

  let width = $state(0);

  const points = $derived(stats.filter((stat): stat is ModelStats & { avg_score: number } => stat.avg_score !== null && stat.p50_total_ms !== null && stat.p50_total_ms > 0));
  const categories = $derived([...new Set(points.map((stat) => stat.category))].sort());
  const times = $derived(points.map((stat) => stat.p50_total_ms as number));
  const minTime = $derived(Math.min(...times, 1000));
  const maxTime = $derived(Math.max(...times, 1000));
  const ticks = $derived(TICKS_MS.filter((tick) => tick >= minTime * 0.5 && tick <= maxTime * 2));
  const logMin = $derived(Math.log10(ticks[0] ?? minTime));
  const logMax = $derived(Math.log10(ticks.at(-1) ?? maxTime));
  const height = $derived(TOP_MARGIN + categories.length * ROW_HEIGHT + AXIS_HEIGHT);

  /**
   * @param {number} ms - duration
   * @returns {number} - horizontal position, log scale: times span two orders of magnitude
   */
  function x(ms: number): number {
    const ratio = (Math.log10(ms) - logMin) / Math.max(logMax - logMin, 0.001);
    return LABEL_WIDTH + ratio * (width - LABEL_WIDTH - RIGHT_MARGIN);
  }

  const rows = $derived(
    categories.map((category, index) => {
      const marks = points
        .filter((stat) => stat.category === category)
        .map((stat) => ({ stat, cx: x(stat.p50_total_ms as number) }))
        .sort((a, b) => a.cx - b.cx);
      return {
        category,
        y: TOP_MARGIN + index * ROW_HEIGHT + ROW_HEIGHT / 2 + 6,
        marks: marks.map((mark, position) => ({
          ...mark,
          // A label too close to the previous one drops under its point.
          below: position > 0 && mark.cx - marks[position - 1].cx < LABEL_GAP && position % 2 === 1,
        })),
      };
    }),
  );
</script>

<figure class="panel tradeoff" bind:clientWidth={width}>
  <figcaption>
    <h3>Score et temps de réponse, par thème</h3>
    <p class="muted">Plus un point est à gauche, plus le modèle répond vite. Le nombre est son score sur 100.</p>
  </figcaption>
  {#if width > 0}
    <svg {width} {height} role="img" aria-label="Score et temps médian de chaque modèle par thème, repris dans le tableau ci-dessous">
      {#each ticks as tick (tick)}
        <line class="grid" x1={x(tick)} x2={x(tick)} y1={TOP_MARGIN} y2={height - AXIS_HEIGHT} />
        <text class="tick" x={x(tick)} y={height - AXIS_HEIGHT + 18} text-anchor="middle">{duration(tick)}</text>
      {/each}
      <text class="axis-title" x={(LABEL_WIDTH + width - RIGHT_MARGIN) / 2} y={height - 8} text-anchor="middle">Temps médian par réponse, échelle logarithmique</text>

      {#each rows as row (row.category)}
        <text class="row-label" class:overall={row.category === ''} x="0" y={row.y + 4}>{categoryLabel(row.category)}</text>
        {#if row.marks.length > 1}
          <line class="link" x1={row.marks[0].cx} x2={row.marks[row.marks.length - 1].cx} y1={row.y} y2={row.y} />
        {/if}
        {#each row.marks as mark (mark.stat.model)}
          <circle class="mark" cx={mark.cx} cy={row.y} r={row.category === '' ? 7.5 : 6} fill={seriesColor(mark.stat.model, models)}>
            <title>{categoryLabel(row.category)}, {mark.stat.model} : {score(mark.stat.avg_score)}/100, {duration(mark.stat.p50_total_ms)}</title>
          </circle>
          <text class="value" class:weak={mark.stat.avg_score < WEAK_SCORE} x={mark.cx} y={mark.below ? row.y + 21 : row.y - 11} text-anchor="middle">{score(mark.stat.avg_score)}</text>
        {/each}
      {/each}
    </svg>
  {/if}
</figure>

<style>
  .tradeoff {
    display: grid;
    gap: var(--s3);
    margin: 0;
    min-width: 0;
  }

  figcaption p {
    margin-top: var(--s1);
    font-size: var(--text-sm);
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

  .row-label {
    fill: var(--ink-2);
    font-size: 13px;
  }

  .row-label.overall {
    fill: var(--ink);
    font-weight: 650;
  }

  .link {
    stroke: var(--axis);
    stroke-width: 1.5;
  }

  .mark {
    stroke: var(--panel);
    stroke-width: 2;
  }

  .value {
    fill: var(--ink);
    font-size: 12px;
    font-weight: 600;
    font-variant-numeric: tabular-nums;
  }

  .value.weak {
    fill: var(--critical);
  }
</style>
