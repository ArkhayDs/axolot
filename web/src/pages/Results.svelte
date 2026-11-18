<script lang="ts">
  import { onMount } from 'svelte';

  import EvalMatrix from '../components/EvalMatrix.svelte';
  import Icon from '../components/Icon.svelte';
  import LatencyStrip from '../components/LatencyStrip.svelte';
  import ModelName from '../components/ModelName.svelte';
  import Notice from '../components/Notice.svelte';
  import ThemeTradeoff from '../components/ThemeTradeoff.svelte';
  import StatsTable from '../components/StatsTable.svelte';
  import CounterMenu from '../components/map/CounterMenu.svelte';
  import { api, errorMessage } from '../lib/api';
  import type { BaseDetail, Case, ConnectionType, Hardware } from '../lib/api';
  import { CONNECTIONS, seriesColor } from '../lib/format';
  import type { Findable } from '../lib/map/findable';
  import { live } from '../lib/live.svelte';

  let cases = $state<Case[]>([]);
  let hardware = $state<Hardware | null>(null);
  let base = $state<BaseDetail | null>(null);
  let loadError = $state('');
  let connection = $state<ConnectionType>('');
  let picked = $state<string[]>([]);
  let menuOpen = $state(false);
  let asked = 0;

  const stats = $derived(base?.stats ?? []);
  const verdicts = $derived(base?.verdicts ?? []);
  const answers = $derived(base?.answers ?? []);
  const models = $derived([...new Set(stats.map((stat) => stat.model))].sort());
  const questions = $derived(cases.reduce((total, item) => total + item.items.length, 0));
  const filtered = $derived(connection !== '' || picked.length > 0);
  // A picked model absent from the connection type stays listed, so it can be unpicked.
  const choices = $derived([...new Set([...(base?.models ?? []), ...picked])].sort());
  const entries = $derived(choices.map((model): Findable => ({ kind: 'model', key: model, label: model, type: 'Modèle', note: '' })));

  /** @param {Findable} entry - model line of the menu, picked or unpicked */
  function toggle(entry: Findable): void {
    picked = picked.includes(entry.key) ? picked.filter((model) => model !== entry.key) : [...picked, entry.key];
  }

  onMount(async () => {
    try {
      const [catalog, status] = await Promise.all([api.cases(), api.status()]);
      cases = catalog.cases;
      hardware = status.hardware;
    } catch (error) {
      loadError = errorMessage(error);
    }
  });

  // The cumulated figures only move when a benchmark ends: running ones are not counted yet. Only the last request
  // lands, an earlier one answering late would show a filter already left.
  $effect(() => {
    void live.finished;
    const request = ++asked;
    api
      .base(connection, [...picked])
      .then((loaded) => {
        if (request !== asked) return;
        base = loaded;
        loadError = '';
      })
      .catch((error: unknown) => {
        if (request === asked) loadError = errorMessage(error);
      });
  });
</script>

<div class="page">
  <div class="page-head">
    <div>
      <h1>Résultats</h1>
      <p>{filtered ? 'Sélection filtrée' : 'Toute la base'} : {answers.length} réponses aux questions actuelles, tous Benchmarks et machines confondus</p>
    </div>
    {#if choices.length > 0 || filtered}
      <div class="filters" role="group" aria-label="Filtres">
        <label class="field">
          Connexion
          <select class="input" bind:value={connection}>
            {#each CONNECTIONS as option (option.value)}
              <option value={option.value}>{option.label}</option>
            {/each}
          </select>
        </label>
        <div class="field">
          <span id="models-filter">Modèles</span>
          <div class="picker">
            <button
              type="button"
              class="input"
              id="models-button"
              data-counter="models"
              aria-labelledby="models-filter models-button"
              aria-expanded={menuOpen}
              onclick={() => (menuOpen = !menuOpen)}
            >
              {picked.length > 0 ? `${picked.length} sur ${choices.length}` : `Tous (${choices.length})`}
            </button>
            {#if menuOpen}
              <CounterMenu
                title="Modèles"
                {entries}
                isSelected={(entry) => picked.includes(entry.key)}
                onpick={toggle}
                onclose={(refocus) => {
                  menuOpen = false;
                  if (refocus) document.querySelector<HTMLButtonElement>('[data-counter="models"]')?.focus();
                }}
                below
              />
            {/if}
          </div>
        </div>
      </div>
    {/if}
  </div>

  <Notice message={loadError} />

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

    <ThemeTradeoff {stats} {models} />
    <LatencyStrip runs={answers} {models} />

    <p class="muted caption">Les réponses une par une se consultent dans chaque <a href="#/benchmarks">Benchmark</a>.</p>

    <StatsTable {stats} {models} showQuestions />
  {:else if filtered && !loadError}
    <div class="empty">
      <h2>Aucun résultat pour ces filtres</h2>
      <p>Aucune réponse mesurée ne correspond à cette connexion et à ces modèles.</p>
      <button
        type="button"
        class="btn"
        onclick={() => {
          connection = '';
          picked = [];
        }}>Retirer les filtres</button
      >
    </div>
  {:else if base && !loadError}
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
      <a class="btn btn-primary cta" href="#/"><Icon name="target" /> Voir l'Atlas</a>
    </div>
  </section>
</div>

<style>
  .filters {
    display: flex;
    flex-wrap: wrap;
    gap: var(--s3);
  }

  .picker {
    position: relative;
  }

  .picker button {
    min-width: 9rem;
    text-align: start;
    cursor: pointer;
  }

  .legend {
    display: flex;
    flex-wrap: wrap;
    gap: var(--s2) var(--s5);
    margin: 0;
    padding: 0;
    list-style: none;
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
