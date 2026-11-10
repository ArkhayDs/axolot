<script lang="ts">
  import ModelName from './ModelName.svelte';
  import type { ModelStats } from '../lib/api';
  import { NET_HINT, UNMEASURED_HINT, categoryLabel, duration, number, score, seriesColor } from '../lib/format';

  let { stats, models, showQuestions = false }: { stats: ModelStats[]; models: string[]; showQuestions?: boolean } = $props();

  const rows = $derived([...stats].sort((a, b) => a.category.localeCompare(b.category) || a.model.localeCompare(b.model)));
  const viaBridge = $derived(stats.some((stat) => stat.p50_net_total_ms !== null));
</script>

<div class="table-wrap">
  <table class="stats">
    <thead>
      <tr>
        <th scope="col">Thème</th>
        <th scope="col">Modèle</th>
        <th scope="col" class="num">Réponses</th>
        {#if showQuestions}<th scope="col" class="num">Questions</th>{/if}
        <th scope="col" class="num" title={UNMEASURED_HINT}>Non mesurées</th>
        <th scope="col" class="num">Score</th>
        <th scope="col" class="num">Temps médian</th>
        {#if viaBridge}<th scope="col" class="num" title={NET_HINT} aria-label="Temps médian net">Net</th>{/if}
        <th scope="col" class="num">1er token</th>
        {#if viaBridge}<th scope="col" class="num" title={NET_HINT} aria-label="1er token net">Net</th>{/if}
        <th scope="col" class="num">t/s</th>
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
          <td class="num">{stat.errors}</td>
          <td class="num">{score(stat.avg_score)}</td>
          <td class="num">{duration(stat.p50_total_ms)}</td>
          {#if viaBridge}<td class="num">{duration(stat.p50_net_total_ms)}</td>{/if}
          <td class="num">{duration(stat.p50_ttft_ms)}</td>
          {#if viaBridge}<td class="num">{duration(stat.p50_net_ttft_ms)}</td>{/if}
          <td class="num">{number(stat.p50_tok_per_sec)}</td>
          <td class="num">{duration(stat.max_load_ms)}</td>
          <td class="num">{stat.retrieval_total > 0 ? `${stat.retrieval_hits}/${stat.retrieval_total}` : '-'}</td>
        </tr>
      {/each}
    </tbody>
  </table>
</div>
{#if viaBridge}
  <p class="muted note">
    Modèles Claude (abonnement, via le pont) : scores comparables aux autres. Colonnes « Net » : temps sans le lancement ni la fermeture de Claude Code,
    mesurés à chaque appel ; la vitesse se juge sur elles. 1er token retardé par la réflexion du modèle, débit (t/s) possiblement surestimé.
  </p>
{/if}

<style>
  .stats td,
  .stats th {
    padding-inline: var(--s2);
    white-space: nowrap;
  }

  /* Headers may wrap: up to thirteen columns must fit the page without a hidden last one. */
  .stats thead th {
    white-space: normal;
  }

  .note {
    margin-top: var(--s2);
    font-size: var(--text-sm);
  }
</style>
