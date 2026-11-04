<script lang="ts">
  import ModelName from './ModelName.svelte';
  import type { Run } from '../lib/api';
  import { categoryLabel, number, seriesColor } from '../lib/format';
  import { INDICATORS, qualityRows } from '../lib/qualimetry';

  let { runs, models }: { runs: Run[]; models: string[] } = $props();

  // Must follow maxTokens in internal/runner: reaching it is the only sign of a truncated answer the runs store.
  const RUNNER_MAX_TOKENS = 1024;

  const rows = $derived(qualityRows(runs, RUNNER_MAX_TOKENS));

  /**
   * @param {number | null} value - share in 0-1
   * @returns {string} - percentage, a dash where the indicator does not apply
   */
  function percent(value: number | null): string {
    return value === null ? '-' : `${number(value * 100)} %`;
  }
</script>

<div class="table-wrap">
  <table>
    <thead>
      <tr>
        <th scope="col">Thème</th>
        <th scope="col">Modèle</th>
        <th scope="col" class="num">Réponses</th>
        <th scope="col" class="num">Mots, médiane</th>
        <th scope="col" class="num">Mots, p95</th>
        <th scope="col" class="num">Mots par phrase</th>
        <th scope="col" class="num">Diversité lexicale</th>
        <th scope="col" class="num">JSON valide</th>
        <th scope="col" class="num">Source citée</th>
        <th scope="col" class="num">Vides ou tronquées</th>
      </tr>
    </thead>
    <tbody>
      {#each rows as row (row.category + row.model)}
        <tr class:overall={row.category === ''}>
          <td>{categoryLabel(row.category)}</td>
          <td><ModelName name={row.model} color={seriesColor(row.model, models)} /></td>
          <td class="num">{row.answers}</td>
          <td class="num">{number(row.wordsMedian)}</td>
          <td class="num">{number(row.wordsP95)}</td>
          <td class="num">{number(row.sentenceWords)}</td>
          <td class="num">{percent(row.diversity)}</td>
          <td class="num">{percent(row.jsonRate)}</td>
          <td class="num">{percent(row.citationRate)}</td>
          <td class="num">{percent(row.blankRate)}</td>
        </tr>
      {/each}
    </tbody>
  </table>
</div>

<dl class="hints">
  {#each INDICATORS as indicator (indicator.label)}
    <div>
      <dt>{indicator.label}</dt>
      <dd class="muted">{indicator.hint}</dd>
    </div>
  {/each}
</dl>

<style>
  td {
    white-space: nowrap;
  }

  .overall td {
    background: var(--raised);
    font-weight: 600;
  }

  .hints {
    display: grid;
    gap: var(--s1);
    margin: 0;
    font-size: var(--text-sm);
  }

  .hints div {
    display: flex;
    flex-wrap: wrap;
    gap: 0 var(--s2);
  }

  dt {
    font-weight: 600;
  }

  dd {
    margin: 0;
  }
</style>
