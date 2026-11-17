<script lang="ts">
  import type { Answer } from '../lib/api';
  import { answerMs, categoryLabel, duration, seriesColor } from '../lib/format';

  let { runs, models }: { runs: Answer[]; models: string[] } = $props();

  const LABEL_WIDTH = 170;
  const RIGHT_MARGIN = 44;
  const LANE_HEIGHT = 18;
  const ROW_GAP = 14;
  const AXIS_HEIGHT = 26;
  const TICKS_MS = [100, 200, 500, 1000, 2000, 5000, 10_000, 20_000, 50_000, 100_000];

  let width = $state(0);

  const answered = $derived(runs.filter((run) => !run.error && answerMs(run) > 0));
  const categories = $derived([...new Set(answered.map((run) => run.category))].sort());
  const minTime = $derived(Math.min(...answered.map((run) => answerMs(run)), 1000));
  const maxTime = $derived(Math.max(...answered.map((run) => answerMs(run)), 1000));
  const ticks = $derived(TICKS_MS.filter((tick) => tick >= minTime * 0.5 && tick <= maxTime * 2));
  const logMin = $derived(Math.log10(ticks[0] ?? minTime));
  const logMax = $derived(Math.log10(ticks.at(-1) ?? maxTime));
  const rowHeight = $derived(models.length * LANE_HEIGHT + ROW_GAP);
  const height = $derived(categories.length * rowHeight + AXIS_HEIGHT);

  /**
   * @param {number} ms - duration
   * @returns {number} - horizontal position, log scale: answers span several orders of magnitude
   */
  function x(ms: number): number {
    const ratio = (Math.log10(ms) - logMin) / Math.max(logMax - logMin, 0.001);
    return LABEL_WIDTH + ratio * (width - LABEL_WIDTH - RIGHT_MARGIN);
  }

  /**
   * @param {number[]} values - durations
   * @returns {number} - median, the figure the rest of the app reports
   */
  function median(values: number[]): number {
    const sorted = [...values].sort((a, b) => a - b);
    const middle = sorted.length / 2;
    return Number.isInteger(middle) ? (sorted[middle - 1] + sorted[middle]) / 2 : sorted[Math.floor(middle)];
  }

  const rows = $derived(
    categories.map((category, rowIndex) => ({
      category,
      top: rowIndex * rowHeight,
      lanes: models
        .map((model, laneIndex) => {
          const laneRuns = answered.filter((run) => run.category === category && run.label === model);
          return { model, laneRuns, y: rowIndex * rowHeight + laneIndex * LANE_HEIGHT + LANE_HEIGHT / 2 };
        })
        .filter((lane) => lane.laneRuns.length > 0),
    })),
  );
</script>

<figure class="panel strip" bind:clientWidth={width}>
  <figcaption>
    <h3>Distribution des temps de réponse</h3>
    <p class="muted">Un point par réponse, un trait à la médiane. Échelle logarithmique.</p>
  </figcaption>
  {#if width > 0}
    <svg {width} {height} role="img" aria-label="Temps de chaque réponse par thème et par modèle, médianes reprises dans le tableau ci-dessous">
      {#each ticks as tick (tick)}
        <line class="grid" x1={x(tick)} x2={x(tick)} y1="0" y2={height - AXIS_HEIGHT} />
        <text class="tick" x={x(tick)} y={height - 8} text-anchor="middle">{duration(tick)}</text>
      {/each}
      {#each rows as row (row.category)}
        <text class="label" x="0" y={row.top + (models.length * LANE_HEIGHT) / 2 + 4}>{categoryLabel(row.category)}</text>
        {#each row.lanes as lane (lane.model)}
          {#each lane.laneRuns as run (run.id)}
            <circle class="run" cx={x(answerMs(run))} cy={lane.y} r="3.5" fill={seriesColor(lane.model, models)}>
              <title>{run.item_id}, {lane.model} : {duration(answerMs(run))}</title>
            </circle>
          {/each}
          {@const center = x(median(lane.laneRuns.map((run) => answerMs(run))))}
          <line class="median" x1={center} x2={center} y1={lane.y - 6} y2={lane.y + 6} />
        {/each}
      {/each}
    </svg>
  {/if}
</figure>

<style>
  .strip {
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

  .label {
    fill: var(--ink-2);
    font-size: 13px;
  }

  .run {
    fill-opacity: 0.65;
  }

  .median {
    stroke: var(--ink);
    stroke-linecap: round;
    stroke-width: 2.5;
  }
</style>
