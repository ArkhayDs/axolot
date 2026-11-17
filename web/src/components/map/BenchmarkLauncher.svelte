<script lang="ts">
  import { onMount } from 'svelte';

  import Icon from '../Icon.svelte';
  import ModelName from '../ModelName.svelte';
  import Notice from '../Notice.svelte';
  import CheckGroup from './CheckGroup.svelte';
  import { api, errorMessage } from '../../lib/api';
  import type { Case, CasesResponse, Estimate, Models, Selection } from '../../lib/api';
  import { refreshCampaigns } from '../../lib/campaigns.svelte';
  import { categoryLabel, estimateLabel } from '../../lib/format';
  import { launch } from '../../lib/live.svelte';

  let { category, onclose }: { category: string; onclose: () => void } = $props();

  let catalog = $state<CasesResponse | null>(null);
  let models = $state<Models | null>(null);
  let name = $state('');
  let chosenModels = $state<string[]>([]);
  let chosenCases = $state<string[]>([]);
  let busy = $state(false);
  let error = $state('');
  let estimate = $state<Estimate | null>(null);

  const embedding = $derived(new Set((models?.library ?? []).filter((entry) => entry.role === 'embedding').map((entry) => entry.name)));
  // Offline connections stay listed, empty, so a missing model reads as a state rather than an absence.
  const connections = $derived(
    (models?.connections ?? []).map((connection) => ({
      provider: connection.provider,
      online: connection.online,
      models: connection.online
        ? connection.models
            .filter((model) => !embedding.has(model.name) && !embedding.has(model.name.split(':')[0]))
            .map((model) => ({ key: `${connection.provider.id}|${model.name}`, provider: connection.provider, name: model.name, label: model.label }))
        : [],
    })),
  );
  const available = $derived(connections.flatMap((connection) => connection.models));
  const themes = $derived(
    [...new Set((catalog?.cases ?? []).map((item) => item.category))].map((category) => ({
      category,
      cases: (catalog?.cases ?? []).filter((item) => item.category === category).map((item) => ({ key: item.id, title: item.title })),
    })),
  );

  /** Loads what the form offers; the saved models come checked, and the filtered theme's scenarios if any */
  async function load(): Promise<void> {
    try {
      [catalog, models] = await Promise.all([api.cases(), api.models()]);
      const saved = new Set(catalog.selections.map((selection) => `${selection.provider_id}|${selection.model}`));
      chosenModels = available.filter((model) => saved.has(model.key)).map((model) => model.key);
      chosenCases = catalog.cases.filter((item: Case) => !category || item.category === category).map((item: Case) => item.id);
    } catch (loadError) {
      error = errorMessage(loadError);
    }
  }

  /** @param {SubmitEvent} event - saves the selection on the checked scenarios, then starts the benchmark */
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
      // A benchmark is a batch of answers: its name is optional, the date and content already identify it.
      const modelWord = picks.length > 1 ? 'modèles' : 'modèle';
      const caseWord = chosenCases.length > 1 ? 'scénarios' : 'scénario';
      await launch(name.trim() || `${picks.length} ${modelWord}, ${chosenCases.length} ${caseWord}`, chosenCases);
      await refreshCampaigns();
    } catch (startError) {
      error = errorMessage(startError);
    } finally {
      busy = false;
    }
  }

  onMount(load);

  $effect(() => {
    const targets = available
      .filter((model) => chosenModels.includes(model.key))
      .map((model) => ({ provider_id: model.provider.id, model: model.name }));
    const scenarios = [...chosenCases];
    estimate = null;
    if (targets.length === 0 || scenarios.length === 0) return;
    let stale = false;
    api
      .estimate(targets, scenarios)
      .then((loaded) => {
        if (!stale) estimate = loaded;
      })
      .catch(() => undefined);
    return () => {
      stale = true;
    };
  });
</script>

<aside class="launcher glass" aria-label="Lancer un Benchmark">
  <div class="head">
    <h2>Lancer un Benchmark</h2>
    <button class="btn" type="button" aria-label="Fermer et revenir au classement" onclick={onclose}><Icon name="cross" /></button>
  </div>
  <form onsubmit={start}>
    <div class="body">
      <label class="field">
        Nom (facultatif)
        <input class="input" bind:value={name} maxlength="80" placeholder="Généré automatiquement" />
      </label>

      <fieldset>
        <legend>Modèles</legend>
        {#each connections as connection (connection.provider.id)}
          <CheckGroup title={connection.provider.name} items={connection.models} bind:chosen={chosenModels} note={connection.online ? '' : 'Hors ligne'}>
            {#snippet row(model)}<ModelName name={model.label} />{/snippet}
          </CheckGroup>
        {/each}
        {#if models && available.length === 0}
          <p class="muted">Aucun modèle en ligne. <a href="#/reglages">Ouvrir les réglages</a></p>
        {/if}
      </fieldset>

      <fieldset>
        <legend class="legend-row">
          Scénarios
          <button class="link" type="button" onclick={() => (chosenCases = (catalog?.cases ?? []).map((item) => item.id))}>Tous</button>
          <button class="link" type="button" onclick={() => (chosenCases = [])}>Aucun</button>
        </legend>
        {#each themes as theme (theme.category)}
          <CheckGroup title={categoryLabel(theme.category)} items={theme.cases} bind:chosen={chosenCases}>
            {#snippet row(item)}<span>{item.title}</span>{/snippet}
          </CheckGroup>
        {/each}
      </fieldset>
    </div>

    <div class="foot">
      <Notice message={error} />
      <p class="muted hint">Ces modèles remplacent la sélection enregistrée des scénarios cochés. Les réponses s'ajoutent à la base.</p>
      <div class="go">
        <p class="estimate" aria-live="polite">
          {#if estimate}
            <strong>{estimateLabel(estimate)}</strong>, {estimate.answers} réponses prévues{estimate.known && estimate.partial ? ', estimation partielle' : ''}
          {/if}
        </p>
        <button class="btn btn-primary" type="submit" disabled={busy || chosenModels.length === 0 || chosenCases.length === 0}>
          <Icon name="play" />
          {busy ? 'Démarrage...' : 'Lancer'}
        </button>
      </div>
    </div>
  </form>
</aside>

<style>
  .launcher {
    display: flex;
    flex-direction: column;
    gap: var(--s3);
    min-height: 0;
    padding: var(--s4);
    border-radius: var(--radius-lg);
  }

  .head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--s3);
  }

  /* Only the body scrolls: the launch button and its error stay in view. */
  form {
    display: flex;
    flex: 1;
    flex-direction: column;
    min-height: 0;
  }

  .body {
    display: grid;
    flex: 1;
    align-content: start;
    gap: calc(var(--s4) * 1.5);
    min-height: 0;
    overflow-y: auto;
    padding-bottom: var(--s4);
    scrollbar-color: var(--axis) transparent;
  }

  .foot {
    display: grid;
    gap: var(--s2);
    padding-top: var(--s3);
    border-top: 1px solid var(--hairline);
  }

  .go {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--s3);
  }

  fieldset {
    display: grid;
    gap: var(--s3);
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

  .legend-row {
    display: flex;
    align-items: baseline;
    gap: var(--s3);
  }

  .link {
    padding: 0;
    border: 0;
    background: none;
    color: var(--ink);
    font-size: var(--text-sm);
    text-decoration: underline;
    text-decoration-color: var(--axis);
    text-underline-offset: 3px;
    cursor: pointer;
  }

  .link:hover {
    text-decoration-color: var(--accent);
  }

  .hint,
  .estimate {
    font-size: var(--text-sm);
  }

  .estimate strong {
    font-weight: 650;
  }
</style>
