<script lang="ts">
  import { onMount } from 'svelte';

  import Icon from '../components/Icon.svelte';
  import ModelName from '../components/ModelName.svelte';
  import NeuralCanvas from '../components/NeuralCanvas.svelte';
  import Notice from '../components/Notice.svelte';
  import RunMatrix from '../components/RunMatrix.svelte';
  import { api, errorMessage } from '../lib/api';
  import type { CasesResponse, Models, Selection } from '../lib/api';
  import { refreshCampaigns } from '../lib/campaigns.svelte';
  import { duration, number, score } from '../lib/format';
  import { launch, live } from '../lib/live.svelte';

  let catalog = $state<CasesResponse | null>(null);
  let models = $state<Models | null>(null);
  let name = $state('Campagne de démo');
  let chosenModels = $state<string[]>([]);
  let chosenCases = $state<string[]>([]);
  let busy = $state(false);
  let error = $state('');
  let outputBox = $state<HTMLPreElement | undefined>();

  const running = $derived(live.campaign !== '');
  const embedding = $derived(new Set((models?.library ?? []).filter((entry) => entry.role === 'embedding').map((entry) => entry.name)));
  const available = $derived(
    (models?.connections ?? [])
      .filter((connection) => connection.online)
      .flatMap((connection) =>
        connection.models
          .filter((model) => !embedding.has(model.name) && !embedding.has(model.name.split(':')[0]))
          .map((model) => ({ key: `${connection.provider.id}|${model.name}`, provider: connection.provider, name: model.name, fit: model.fit })),
      ),
  );
  const networkModels = $derived.by(() => {
    if (!running) return available.filter((model) => chosenModels.includes(model.key)).map((model) => model.name);
    const names = new Set((catalog?.selections ?? []).map((selection) => selection.model));
    for (const run of live.runs) names.add(run.model);
    if (live.model) names.add(live.model);
    return [...names].sort();
  });
  const networkCases = $derived.by(() => {
    const all = catalog?.cases ?? [];
    if (!running) return all.filter((item) => chosenCases.includes(item.id));
    const selected = new Set((catalog?.selections ?? []).map((selection) => selection.case_id));
    return all.filter((item) => selected.has(item.id) || item.id === live.caseId);
  });
  const currentCase = $derived(catalog?.cases.find((item) => item.id === live.caseId));
  const lastRun = $derived(live.runs.at(-1));
  const matrixRuns = $derived(running ? live.runs : live.lastRuns);
  const matrixModels = $derived([...new Set([...matrixRuns.map((run) => run.model), ...(running ? networkModels : [])])].sort());
  const matrixCases = $derived(
    (catalog?.cases ?? []).filter((item) => matrixRuns.some((run) => run.case_id === item.id) || (running && networkCases.includes(item))),
  );

  /** Loads what the launcher offers and restores the saved selection */
  async function load(): Promise<void> {
    try {
      [catalog, models] = await Promise.all([api.cases(), api.models()]);
      const saved = new Set(catalog.selections.map((selection) => `${selection.provider_id}|${selection.model}`));
      chosenModels = available.filter((model) => saved.has(model.key)).map((model) => model.key);
      chosenCases = catalog.cases.map((item) => item.id);
      error = '';
    } catch (loadError) {
      error = errorMessage(loadError);
    }
  }

  /** Saves the selection on the checked cases, then starts the campaign */
  async function start(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    busy = true;
    error = '';
    try {
      const picks = available.filter((model) => chosenModels.includes(model.key));
      await Promise.all(
        chosenCases.map((caseId) => {
          const selections: Selection[] = picks.map((model) => ({ case_id: caseId, provider_id: model.provider.id, model: model.name }));
          return api.setCaseModels(caseId, selections);
        }),
      );
      catalog = await api.cases();
      await launch(name, chosenCases);
      await refreshCampaigns();
    } catch (startError) {
      error = errorMessage(startError);
    } finally {
      busy = false;
    }
  }

  /** Stops the running campaign; the current item is abandoned */
  async function cancel(): Promise<void> {
    try {
      await api.cancelCampaign(live.campaign);
    } catch (cancelError) {
      error = errorMessage(cancelError);
    }
  }

  onMount(load);

  $effect(() => {
    void live.output;
    if (outputBox) outputBox.scrollTop = outputBox.scrollHeight;
  });
</script>

<div class="page">
  <div class="page-head">
    <h1>Labo</h1>
  </div>

  <Notice message={error} />

  <div class="layout">
    <div class="main">
      <NeuralCanvas models={networkModels} cases={networkCases} />
      {#if matrixRuns.length > 0 || running}
        <RunMatrix cases={matrixCases} models={matrixModels} runs={matrixRuns} activeItem={live.item} activeModel={live.model} />
      {/if}
    </div>

    <aside class="side glass" aria-label={running ? 'Campagne en cours' : 'Lancer une campagne'}>
      {#if running}
        <div class="run-head">
          <h2>{currentCase?.title ?? 'Démarrage'}</h2>
          <button class="btn" type="button" onclick={cancel}><Icon name="stop" /> Arrêter</button>
        </div>
        {#if live.model}<ModelName name={live.model} />{/if}
        <div class="progress" role="progressbar" aria-valuemin="0" aria-valuemax={live.total} aria-valuenow={live.done} aria-label="Progression">
          <span style:width="{live.total ? (100 * live.done) / live.total : 0}%"></span>
        </div>
        <p class="counts">
          <span>{live.done} / {live.total} réponses</span>
          <span>{live.tokens} fragments</span>
        </p>
        <pre class="output" bind:this={outputBox} aria-label="Sortie du modèle">{live.output || (live.model ? 'Chargement du modèle...' : '')}</pre>
        <div aria-live="polite">
          {#if lastRun}
            <p class="last">
              {#if lastRun.error}
                <span class="state state-critical"><Icon name="cross" size={16} /> Échec</span>
              {:else}
                <strong>{score(lastRun.score)}/100</strong>
              {/if}
              <span class="muted">{duration(lastRun.total_ms)}, {number(lastRun.tok_per_sec)} jetons/s</span>
            </p>
          {/if}
        </div>
      {:else}
        <h2>Lancer une campagne</h2>
        <form onsubmit={start}>
          <label class="field">
            Nom
            <input class="input" bind:value={name} required maxlength="80" />
          </label>

          <fieldset>
            <legend>Modèles</legend>
            {#each available as model (model.key)}
              <label class="check">
                <input type="checkbox" bind:group={chosenModels} value={model.key} />
                <ModelName name={model.name} />
              </label>
            {:else}
              <p class="muted">Aucun modèle en ligne. <a href="#/reglages">Ouvrir les réglages</a></p>
            {/each}
          </fieldset>

          <fieldset>
            <legend>Cas</legend>
            <div class="case-list">
              {#each catalog?.cases ?? [] as item (item.id)}
                <label class="check">
                  <input type="checkbox" bind:group={chosenCases} value={item.id} />
                  <span>{item.title}</span>
                </label>
              {/each}
            </div>
          </fieldset>

          <p class="muted hint">Ces modèles remplacent la sélection enregistrée des cas cochés.</p>
          <button class="btn btn-primary" type="submit" disabled={busy || chosenModels.length === 0 || chosenCases.length === 0}>
            <Icon name="play" />
            {busy ? 'Démarrage...' : 'Lancer'}
          </button>
        </form>
      {/if}
    </aside>
  </div>
</div>

<style>
  .layout {
    display: grid;
    grid-template-columns: minmax(0, 1fr) 360px;
    align-items: start;
    gap: var(--s4);
  }

  .main {
    display: grid;
    gap: var(--s4);
    min-width: 0;
  }

  .side {
    display: grid;
    gap: var(--s3);
    padding: var(--s4);
    border-radius: var(--radius-lg);
  }

  .run-head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--s3);
  }

  .counts,
  .last {
    display: flex;
    flex-wrap: wrap;
    justify-content: space-between;
    gap: var(--s2);
    font-size: var(--text-sm);
    font-variant-numeric: tabular-nums;
  }

  .last strong {
    font-size: var(--text-lg);
  }

  form {
    display: grid;
    gap: var(--s4);
  }

  fieldset {
    display: grid;
    gap: var(--s2);
    min-width: 0;
    margin: 0;
    padding: 0;
    border: 0;
  }

  legend {
    margin-bottom: var(--s2);
    padding: 0;
    color: var(--ink-2);
    font-size: var(--text-sm);
  }

  /* Scrolls on its own so the launch button stays in view beside the network. */
  .case-list {
    display: grid;
    gap: var(--s2);
    max-height: 30vh;
    overflow-y: auto;
  }

  .check {
    display: flex;
    align-items: baseline;
    gap: var(--s2);
    cursor: pointer;
  }

  .hint {
    font-size: var(--text-sm);
  }

  /* Under this width the network loses its case labels, so it needs the page's full width. */
  @media (max-width: 1040px) {
    .layout {
      grid-template-columns: minmax(0, 1fr);
    }
  }
</style>
