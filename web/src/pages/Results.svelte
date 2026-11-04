<script lang="ts">
  import CampaignPicker from '../components/CampaignPicker.svelte';
  import GroupedBars from '../components/GroupedBars.svelte';
  import type { BarGroup } from '../components/GroupedBars.svelte';
  import Icon from '../components/Icon.svelte';
  import ModelName from '../components/ModelName.svelte';
  import Notice from '../components/Notice.svelte';
  import type { ModelStats } from '../lib/api';
  import { campaignResults, picked } from '../lib/campaigns.svelte';
  import { categoryLabel, duration, number, score, seriesColor, statusLabel } from '../lib/format';

  const results = campaignResults(() => picked.id);
  const stats = $derived(results.detail?.stats ?? []);
  const models = $derived([...new Set(stats.map((stat) => stat.model))].sort());
  const categories = $derived([...new Set(stats.map((stat) => stat.category))].sort());
  const rows = $derived([...stats].sort((a, b) => a.category.localeCompare(b.category) || a.model.localeCompare(b.model)));
  const maxTime = $derived(Math.max(0, ...stats.map((stat) => stat.p50_total_ms ?? 0)));

  /**
   * @param {ModelStats} stat - model figures
   * @returns {string} - tooltip content
   */
  function detail(stat: ModelStats): string {
    const errors = stat.errors > 0 ? `, ${stat.errors} échecs` : '';
    return `${stat.runs} réponses${errors}, ${score(stat.avg_score)}/100, ${duration(stat.p50_total_ms)}`;
  }

  /**
   * Builds one chart's groups, one per task
   * @param {(stat: ModelStats) => number} value - bar length
   * @param {(stat: ModelStats) => string} text - value shown at the tip
   * @returns {BarGroup[]} - chart groups
   */
  function groups(value: (stat: ModelStats) => number, text: (stat: ModelStats) => string): BarGroup[] {
    return categories.map((category) => ({
      label: categoryLabel(category),
      bars: rows
        .filter((stat) => stat.category === category)
        .map((stat) => ({
          model: stat.model,
          value: value(stat),
          text: text(stat),
          color: seriesColor(stat.model, models),
          detail: detail(stat),
        })),
    }));
  }
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

  <Notice message={results.error || picked.error} />

  {#if stats.length > 0}
    <ul class="legend" aria-label="Légende">
      {#each models as model (model)}
        <li><ModelName name={model} color={seriesColor(model, models)} /></li>
      {/each}
    </ul>

    <div class="charts">
      <GroupedBars title="Score moyen, sur 100" max={100} groups={groups((stat) => stat.avg_score, (stat) => score(stat.avg_score))} />
      <GroupedBars title="Temps médian par réponse" max={maxTime} groups={groups((stat) => stat.p50_total_ms ?? 0, (stat) => duration(stat.p50_total_ms))} />
    </div>

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

  .charts {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(min(100%, 420px), 1fr));
    gap: var(--s4);
  }

  td,
  th {
    white-space: nowrap;
  }
</style>
