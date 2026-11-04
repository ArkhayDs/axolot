<script lang="ts">
  import { onMount } from 'svelte';

  import CampaignPicker from '../components/CampaignPicker.svelte';
  import Icon from '../components/Icon.svelte';
  import LatencyStrip from '../components/LatencyStrip.svelte';
  import ModelName from '../components/ModelName.svelte';
  import Notice from '../components/Notice.svelte';
  import RunMatrix from '../components/RunMatrix.svelte';
  import ScoreScatter from '../components/ScoreScatter.svelte';
  import { api, errorMessage } from '../lib/api';
  import type { Case } from '../lib/api';
  import { campaignResults, picked } from '../lib/campaigns.svelte';
  import { categoryLabel, duration, number, score, seriesColor, statusLabel } from '../lib/format';

  let cases = $state<Case[]>([]);
  let casesError = $state('');

  const results = campaignResults(() => picked.id);
  const stats = $derived(results.detail?.stats ?? []);
  const models = $derived([...new Set(stats.map((stat) => stat.model))].sort());
  const rows = $derived([...stats].sort((a, b) => a.category.localeCompare(b.category) || a.model.localeCompare(b.model)));
  const playedCases = $derived(cases.filter((item) => results.detail?.runs.some((run) => run.case_id === item.id)));

  onMount(async () => {
    try {
      cases = (await api.cases()).cases;
    } catch (error) {
      casesError = errorMessage(error);
    }
  });
</script>

<div class="page">
  <div class="page-head">
    <div>
      <h1>Résultats</h1>
      {#if results.detail}
        <p>{results.detail.campaign.name}, {statusLabel(results.detail.campaign.status).toLowerCase()}</p>
      {/if}
    </div>
    <CampaignPicker bind:value={picked.id} campaigns={picked.list} />
  </div>

  <Notice message={results.error || picked.error || casesError} />

  {#if stats.length > 0 && results.detail}
    <ul class="legend" aria-label="Légende">
      {#each models as model (model)}
        <li><ModelName name={model} color={seriesColor(model, models)} /></li>
      {/each}
    </ul>

    <ScoreScatter {stats} {models} />
    <LatencyStrip runs={results.detail.runs} {models} />
    <RunMatrix cases={playedCases} {models} runs={results.detail.runs} />

    <div class="table-wrap">
      <table>
        <thead>
          <tr>
            <th scope="col">Tâche</th>
            <th scope="col">Modèle</th>
            <th scope="col" class="num">Réponses</th>
            <th scope="col" class="num">Échecs</th>
            <th scope="col" class="num">Score</th>
            <th scope="col" class="num">Temps médian</th>
            <th scope="col" class="num">1er jeton</th>
            <th scope="col" class="num">Jetons/s</th>
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
  {:else if !results.error && !picked.error}
    <div class="empty">
      <h2>Aucun résultat pour l'instant</h2>
      <a class="btn btn-primary" href="#/labo"><Icon name="play" /> Lancer une campagne</a>
    </div>
  {/if}
</div>

<style>
  .legend {
    display: flex;
    flex-wrap: wrap;
    gap: var(--s2) var(--s5);
    margin: 0;
    padding: 0;
    list-style: none;
  }

  td,
  th {
    white-space: nowrap;
  }
</style>
