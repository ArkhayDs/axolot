<script lang="ts">
  import Icon from './Icon.svelte';
  import ModelName from './ModelName.svelte';
  import type { ModelStats } from '../lib/api';
  import { categoryLabel, duration, number, score, seriesColor } from '../lib/format';

  let { stats, models, showQuestions = false }: { stats: ModelStats[]; models: string[]; showQuestions?: boolean } = $props();

  const rows = $derived([...stats].sort((a, b) => a.category.localeCompare(b.category) || a.model.localeCompare(b.model)));
</script>

<div class="table-wrap">
  <table class="stats">
    <thead>
      <tr>
        <th scope="col">Thème</th>
        <th scope="col">Modèle</th>
        <th scope="col" class="num">Réponses</th>
        {#if showQuestions}<th scope="col" class="num">Questions</th>{/if}
        <th scope="col" class="num">Échecs</th>
        <th scope="col" class="num">Score</th>
        <th scope="col" class="num">Temps médian</th>
        <th scope="col" class="num">1er token</th>
        <th scope="col" class="num">Tokens/s</th>
        <th scope="col" class="num">Chargement</th>
        <th scope="col" class="num">Passages trouvés</th>
      </tr>
    </thead>
    <tbody>
      {#each rows as stat (stat.category + stat.provider + stat.model)}
        <tr>
          <td>{categoryLabel(stat.category)}</td>
          <td><ModelName name={stat.model} color={seriesColor(stat.model, models)} /></td>
          <td class="num">{stat.runs}</td>
          {#if showQuestions}<td class="num">{stat.questions}</td>{/if}
          <td class="num">
            {#if stat.errors > 0}
              <span class="state state-critical"><Icon name="alert" size={16} /> {stat.errors}</span>
            {:else}
              0
            {/if}
          </td>
          <td class="num">{score(stat.avg_score)}</td>
          <td class="num">{duration(stat.p50_total_ms)}</td>
          <td class="num">{duration(stat.p50_ttft_ms)}</td>
          <td class="num">{number(stat.p50_tok_per_sec)}</td>
          <td class="num">{duration(stat.max_load_ms)}</td>
          <td class="num">{stat.retrieval_total > 0 ? `${stat.retrieval_hits}/${stat.retrieval_total}` : '-'}</td>
        </tr>
      {/each}
    </tbody>
  </table>
</div>

<style>
  .stats td,
  .stats th {
    padding-inline: var(--s2);
    white-space: nowrap;
  }

  /* Headers may wrap: the eleven columns must fit the page without a hidden last one. */
  .stats thead th {
    white-space: normal;
  }
</style>
