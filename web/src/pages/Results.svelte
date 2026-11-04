<script lang="ts">
  import { onMount } from 'svelte';

  import CampaignPicker from '../components/CampaignPicker.svelte';
  import EvalMatrix from '../components/EvalMatrix.svelte';
  import Icon from '../components/Icon.svelte';
  import LatencyStrip from '../components/LatencyStrip.svelte';
  import ModelName from '../components/ModelName.svelte';
  import Notice from '../components/Notice.svelte';
  import RunMatrix from '../components/RunMatrix.svelte';
  import ScoreScatter from '../components/ScoreScatter.svelte';
  import { api, errorMessage } from '../lib/api';
  import type { Answer, BaseDetail, Case, Hardware, ModelStats, Verdict } from '../lib/api';
  import { BASE_VIEW, campaignResults, picked } from '../lib/campaigns.svelte';
  import { benchmarkName, categoryLabel, duration, number, score, seriesColor, statusLabel, when } from '../lib/format';
  import { live } from '../lib/live.svelte';

  let cases = $state<Case[]>([]);
  let hardware = $state<Hardware | null>(null);
  let base = $state<BaseDetail | null>(null);
  let loadError = $state('');

  const wholeBase = $derived(picked.view === BASE_VIEW);
  const results = campaignResults(() => (wholeBase ? '' : picked.view));
  const stats = $derived<ModelStats[]>((wholeBase ? base?.stats : results.detail?.stats) ?? []);
  const verdicts = $derived<Verdict[]>((wholeBase ? base?.verdicts : results.detail?.verdicts) ?? []);
  const answers = $derived<Answer[]>((wholeBase ? base?.answers : results.detail?.runs) ?? []);
  const models = $derived([...new Set(stats.map((stat) => stat.model))].sort());
  const rows = $derived([...stats].sort((a, b) => a.category.localeCompare(b.category) || a.model.localeCompare(b.model)));
  const playedCases = $derived(cases.filter((item) => results.detail?.runs.some((run) => run.case_id === item.id)));
  const questions = $derived(cases.reduce((total, item) => total + item.items.length, 0));

  onMount(async () => {
    try {
      const [catalog, status] = await Promise.all([api.cases(), api.status()]);
      cases = catalog.cases;
      hardware = status.hardware;
    } catch (error) {
      loadError = errorMessage(error);
    }
  });

  // The cumulated figures only move when a benchmark ends: running ones are not counted yet.
  $effect(() => {
    void live.finished;
    if (!wholeBase) return;
    api
      .base()
      .then((loaded) => {
        base = loaded;
        loadError = '';
      })
      .catch((error: unknown) => {
        loadError = errorMessage(error);
      });
  });
</script>

<div class="page">
  <div class="page-head">
    <div>
      <h1>Résultats</h1>
      {#if wholeBase}
        <p>Toute la base : {answers.length} réponses aux questions actuelles, tous Benchmarks et machines confondus</p>
      {:else if results.detail}
        <p>
          {benchmarkName(results.detail.campaign.name)}, {statusLabel(results.detail.campaign.status).toLowerCase()}, {results.detail.campaign.done_items} réponses sur {results.detail.campaign.hardware.label}, {when(results.detail.campaign.created_at)}
        </p>
      {/if}
    </div>
    <CampaignPicker bind:value={picked.view} campaigns={picked.list} base />
  </div>

  <Notice message={results.error || picked.error || loadError} />

  {#if stats.length > 0}
    <section class="section" aria-labelledby="verdict">
      <h2 id="verdict">Quel modèle pour quelle tâche ?</h2>
      <div class="matrix">
        <EvalMatrix {stats} {verdicts} {models} />
        <p class="muted caption">Plus une case est rouge, plus le modèle a perdu de points. Recommandé : le plus rapide à moins de 10 points du meilleur score.</p>
      </div>
    </section>

    <ul class="legend" aria-label="Légende">
      {#each models as model (model)}
        <li><ModelName name={model} color={seriesColor(model, models)} /></li>
      {/each}
    </ul>

    <ScoreScatter {stats} {models} />
    <LatencyStrip runs={answers} {models} />

    {#if results.detail && !wholeBase}
      <RunMatrix cases={playedCases} {models} runs={results.detail.runs} />
    {:else}
      <p class="muted caption">La matrice des réponses appartient à un Benchmark précis : choisissez-en un dans le sélecteur ci-dessus.</p>
    {/if}

    <div class="table-wrap">
      <table>
        <thead>
          <tr>
            <th scope="col">Thème</th>
            <th scope="col">Modèle</th>
            <th scope="col" class="num">Réponses</th>
            {#if wholeBase}<th scope="col" class="num">Questions</th>{/if}
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
              {#if wholeBase}<td class="num">{stat.questions}</td>{/if}
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
  {:else if !results.error && !picked.error && !loadError}
    <div class="empty">
      <h2>Aucun résultat pour l'instant</h2>
      <p>Un premier Benchmark sur les questions actuelles remplit cette page.</p>
      <a class="btn btn-primary" href="#/"><Icon name="play" /> Lancer un Benchmark</a>
    </div>
  {/if}

  <section class="section" aria-labelledby="how">
    <h2 id="how">Comment ça marche</h2>
    <div class="how panel">
      <a class="step" href="#/cas">
        <Icon name="list" size={24} />
        <span><strong>Des scénarios à réponse connue</strong><span class="muted">{cases.length > 0 ? `${cases.length} scénarios, ${questions} questions` : 'Code, documents, tri...'}</span></span>
      </a>
      <span class="link"><Icon name="arrow" /></span>
      <a class="step" href="#/reglages">
        <Icon name="cpu" size={24} />
        <span><strong>Les modèles de cette machine</strong><span class="muted">{hardware ? hardware.label : 'Un à la fois'}</span></span>
      </a>
      <span class="link"><Icon name="arrow" /></span>
      <span class="step">
        <Icon name="grade" size={24} />
        <span><strong>Une note sans IA</strong><span class="muted">Vérifications automatiques, sources ouvrables</span></span>
      </span>
      <a class="btn btn-primary cta" href="#/"><Icon name="target" /> Voir la carte</a>
    </div>
  </section>
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

  .matrix {
    display: grid;
    gap: var(--s2);
    min-width: 0;
  }

  .caption {
    font-size: var(--text-sm);
  }

  .how {
    display: flex;
    flex-wrap: wrap;
    align-items: flex-start;
    gap: var(--s3) var(--s4);
  }

  .cta {
    align-self: center;
  }

  .step {
    display: flex;
    flex: 1 1 13rem;
    align-items: flex-start;
    gap: var(--s3);
    border-radius: var(--radius-sm);
    text-decoration: none;
  }

  a.step:hover strong {
    color: var(--accent);
  }

  .step strong {
    display: block;
    font-weight: 600;
    transition: color 0.15s;
  }

  .step span span {
    display: block;
    font-size: var(--text-sm);
  }

  .link {
    color: var(--axis);
  }

  @media (max-width: 760px) {
    .how {
      align-items: stretch;
      flex-direction: column;
    }

    .step {
      flex: none;
    }

    .link {
      display: none;
    }
  }
</style>
