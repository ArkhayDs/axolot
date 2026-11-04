<script lang="ts">
  import { onMount } from 'svelte';

  import Icon from '../Icon.svelte';
  import ModelName from '../ModelName.svelte';
  import Notice from '../Notice.svelte';
  import { api, errorMessage } from '../../lib/api';
  import type { Case, CasesResponse, Estimate, Models, Selection } from '../../lib/api';
  import { refreshCampaigns } from '../../lib/campaigns.svelte';
  import { categoryLabel, estimateLabel } from '../../lib/format';
  import { launch } from '../../lib/live.svelte';

  let { category, onclose }: { category: string; onclose: () => void } = $props();

  let catalog = $state<CasesResponse | null>(null);
  let models = $state<Models | null>(null);
  let name = $state('Benchmark de démo');
  let chosenModels = $state<string[]>([]);
  let chosenCases = $state<string[]>([]);
  let busy = $state(false);
  let error = $state('');
  let estimate = $state<Estimate | null>(null);

  const embedding = $derived(new Set((models?.library ?? []).filter((entry) => entry.role === 'embedding').map((entry) => entry.name)));
  const available = $derived(
    (models?.connections ?? [])
      .filter((connection) => connection.online)
      .flatMap((connection) =>
        connection.models
          .filter((model) => !embedding.has(model.name) && !embedding.has(model.name.split(':')[0]))
          .map((model) => ({ key: `${connection.provider.id}|${model.name}`, provider: connection.provider, name: model.name })),
      ),
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
      await launch(name, chosenCases);
      await refreshCampaigns();
    } catch (startError) {
      error = errorMessage(startError);
    } finally {
      busy = false;
    }
  }

  onMount(load);

  $effect(() => {
    const names = available.filter((model) => chosenModels.includes(model.key)).map((model) => model.name);
    const scenarios = [...chosenCases];
    estimate = null;
    if (names.length === 0 || scenarios.length === 0) return;
    let stale = false;
    api
      .estimate(names, scenarios)
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
  <Notice message={error} />
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
      <legend class="legend-row">
        Scénarios
        <button class="link" type="button" onclick={() => (chosenCases = (catalog?.cases ?? []).map((item) => item.id))}>Tous</button>
        <button class="link" type="button" onclick={() => (chosenCases = [])}>Aucun</button>
      </legend>
      <div class="case-list">
        {#each catalog?.cases ?? [] as item (item.id)}
          <label class="check">
            <input type="checkbox" bind:group={chosenCases} value={item.id} />
            <span>{item.title} <span class="muted">{categoryLabel(item.category)}</span></span>
          </label>
        {/each}
      </div>
    </fieldset>

    <p class="muted hint">Ces modèles remplacent la sélection enregistrée des scénarios cochés. Les réponses s'ajoutent à la base.</p>
    {#if estimate}
      <p class="estimate" aria-live="polite">
        <strong>{estimateLabel(estimate)}</strong>, {estimate.answers} réponses prévues{estimate.known && estimate.partial ? ', estimation partielle' : ''}
      </p>
    {/if}
    <button class="btn btn-primary" type="submit" disabled={busy || chosenModels.length === 0 || chosenCases.length === 0}>
      <Icon name="play" />
      {busy ? 'Démarrage...' : 'Lancer'}
    </button>
  </form>
</aside>

<style>
  .launcher {
    display: flex;
    flex-direction: column;
    gap: var(--s3);
    min-height: 0;
    overflow-y: auto;
    padding: var(--s4);
    border-radius: var(--radius-lg);
    scrollbar-color: var(--axis) transparent;
  }

  .head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--s3);
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

  /* Scrolls on its own so the launch button stays in view. */
  .case-list {
    display: grid;
    gap: var(--s2);
    max-height: 32vh;
    overflow-y: auto;
    scrollbar-color: var(--axis) transparent;
  }

  .check {
    display: flex;
    align-items: baseline;
    gap: var(--s2);
    cursor: pointer;
  }

  .hint,
  .estimate {
    font-size: var(--text-sm);
  }

  .estimate strong {
    font-weight: 650;
  }
</style>
