<script lang="ts">
  import ModelName from './ModelName.svelte';
  import type { ModelStats, Verdict } from '../lib/api';
  import { NET_HINT, categoryLabel, duration, modelMs, score, seriesColor } from '../lib/format';

  let { stats, verdicts, models }: { stats: ModelStats[]; verdicts: Verdict[]; models: string[] } = $props();

  // The tag is only dropped while the family alone still identifies each model.
  const shortNames = $derived(new Set(models.map((model) => model.split(':')[0])).size === models.length);
  const byKey = $derived(new Map(stats.map((stat) => [`${stat.category}|${stat.model}`, stat])));

  /**
   * Colors what a model lost, not what it scored: perfect cells recede and the separating ones stand out
   * @param {number} value - score in 0-100
   * @returns {string} - cell background
   */
  function lossFill(value: number): string {
    return `color-mix(in srgb, var(--critical) ${Math.round(100 - value)}%, transparent)`;
  }

  /**
   * @param {string} category - task
   * @param {(stat: ModelStats) => number | null} value - figure compared
   * @param {'max' | 'min'} best - direction of the comparison
   * @returns {number | null} - best figure of the row, null when every model ties or only one answered
   */
  function bestOf(category: string, value: (stat: ModelStats) => number | null, best: 'max' | 'min'): number | null {
    const figures = models
      .map((model) => byKey.get(`${category}|${model}`))
      .map((stat) => (stat ? value(stat) : null))
      .filter((figure): figure is number => figure !== null);
    if (figures.length < 2 || figures.every((figure) => figure === figures[0])) return null;
    return best === 'max' ? Math.max(...figures) : Math.min(...figures);
  }
</script>

<div class="table-wrap">
  <table class="eval">
    <thead>
      <tr>
        <th scope="col" rowspan="2">Thème</th>
        <th scope="col" class="num" rowspan="2">Questions</th>
        <th scope="colgroup" colspan={models.length} class="group">Score sur 100</th>
        <th scope="colgroup" colspan={models.length} class="group" title={stats.some((stat) => stat.p50_net_total_ms !== null) ? NET_HINT : undefined}>Temps médian</th>
        <th scope="col" rowspan="2" class="group-start">Recommandé</th>
      </tr>
      <tr>
        {#each [0, 1] as block (block)}
          {#each models as model, index (model)}
            <th scope="col" class="num" class:group-start={index === 0} title={model}>
              <ModelName name={model} color={seriesColor(model, models)} short={shortNames} />
            </th>
          {/each}
        {/each}
      </tr>
    </thead>
    <tbody>
      {#each verdicts as verdict (verdict.category)}
        {@const category = verdict.category}
        {@const bestScore = bestOf(category, (stat) => stat.avg_score, 'max')}
        {@const bestTime = bestOf(category, modelMs, 'min')}
        {@const first = byKey.get(`${category}|${models[0]}`)}
        <tr class:overall={category === ''}>
          <th scope="row">{categoryLabel(category)}</th>
          <td class="num muted">{first?.questions ?? first?.runs ?? '-'}</td>
          {#each models as model, index (model)}
            {@const stat = byKey.get(`${category}|${model}`)}
            <td class="num heat" class:group-start={index === 0} class:best={bestScore !== null && stat?.avg_score === bestScore} style:background={stat && stat.avg_score !== null ? lossFill(stat.avg_score) : undefined}>
              {stat ? score(stat.avg_score) : '-'}
            </td>
          {/each}
          {#each models as model, index (model)}
            {@const stat = byKey.get(`${category}|${model}`)}
            <td class="num" class:group-start={index === 0} class:best={stat !== undefined && modelMs(stat) === bestTime}>{stat ? duration(modelMs(stat)) : '-'}</td>
          {/each}
          <td class="group-start">
            {#if verdict.compromise}
              <ModelName name={verdict.compromise.model} color={seriesColor(verdict.compromise.model, models)} short={shortNames} />
            {:else}
              <span class="muted">-</span>
            {/if}
          </td>
        </tr>
      {/each}
    </tbody>
  </table>
</div>

<style>
  .eval th,
  .eval td {
    padding: var(--s2) var(--s3);
    white-space: nowrap;
  }

  .eval thead tr:first-child th {
    border-bottom: 0;
    padding-bottom: 0;
  }

  .eval tbody th {
    color: var(--ink);
    font-size: var(--text-md);
    font-weight: 500;
  }

  .group {
    text-align: center;
  }

  .group-start {
    border-left: 1px solid var(--axis);
  }

  .heat {
    /* A 2px surface gap keeps neighbouring heat cells apart without drawing a border. */
    background-clip: padding-box;
    border-right: 2px solid var(--panel);
    font-weight: 500;
  }

  .best {
    color: var(--ink);
    font-weight: 700;
  }

  .overall {
    background: var(--raised);
  }

  .overall th,
  .overall td {
    font-size: var(--text-lg);
    font-weight: 650;
  }
</style>
