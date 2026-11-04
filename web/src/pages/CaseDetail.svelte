<script lang="ts">
  import { onMount } from 'svelte';

  import CampaignPicker from '../components/CampaignPicker.svelte';
  import Icon from '../components/Icon.svelte';
  import ModelName from '../components/ModelName.svelte';
  import Notice from '../components/Notice.svelte';
  import RunColumn from '../components/RunColumn.svelte';
  import SourceDialog from '../components/SourceDialog.svelte';
  import { api, errorMessage } from '../lib/api';
  import type { CasesResponse, Models, Run, Selection, Settings, Source } from '../lib/api';
  import { campaignResults, picked, refreshCampaigns } from '../lib/campaigns.svelte';
  import { number, seriesColor, splitPrompt, trapLabel } from '../lib/format';
  import { launch, live } from '../lib/live.svelte';

  let { id }: { id: string } = $props();

  let catalog = $state<CasesResponse | null>(null);
  let models = $state<Models | null>(null);
  let settings = $state<Settings | null>(null);
  let campaignId = $state(picked.id);
  let error = $state('');
  let busy = $state(false);
  let source = $state<Source | null>(null);

  const results = campaignResults(() => campaignId);
  const current = $derived(catalog?.cases.find((item) => item.id === id));
  const selections = $derived((catalog?.selections ?? []).filter((selection) => selection.case_id === id));
  const embedding = $derived(new Set((models?.library ?? []).filter((entry) => entry.role === 'embedding').map((entry) => entry.name)));
  const runs = $derived((results.detail?.runs ?? []).filter((run) => run.case_id === id));
  const modelNames = $derived([...new Set((results.detail?.runs ?? []).map((run) => run.model))].sort());
  const corpus = $derived(results.detail?.campaign.rag_config.corpus_dir ?? settings?.corpus_dir ?? '');
  const runsByItem = $derived(
    runs.reduce((groups, run) => {
      groups.set(run.item_id, [...(groups.get(run.item_id) ?? []), run]);
      return groups;
    }, new Map<string, Run[]>()),
  );

  // The list arrives after this page opens on a fresh load.
  $effect(() => {
    if (!campaignId && picked.id) campaignId = picked.id;
  });

  /**
   * @param {number} providerId - connection id
   * @param {string} model - model tag
   * @returns {boolean} - whether the model is selected for this case
   */
  function isSelected(providerId: number, model: string): boolean {
    return selections.some((selection) => selection.provider_id === providerId && selection.model === model);
  }

  /**
   * Adds or removes a model from this case's selection
   * @param {number} providerId - connection id
   * @param {string} model - model tag
   */
  async function toggle(providerId: number, model: string): Promise<void> {
    if (!catalog) return;
    const next: Selection[] = isSelected(providerId, model)
      ? selections.filter((selection) => !(selection.provider_id === providerId && selection.model === model))
      : [...selections, { case_id: id, provider_id: providerId, model }];
    try {
      await api.setCaseModels(id, next);
      catalog = await api.cases();
      error = '';
    } catch (saveError) {
      error = errorMessage(saveError);
    }
  }

  /** Runs this case alone and follows its results here */
  async function runCase(): Promise<void> {
    if (!current) return;
    busy = true;
    try {
      campaignId = await launch(`${current.title} (direct)`, [id]);
      await refreshCampaigns();
      error = '';
    } catch (runError) {
      error = errorMessage(runError);
    } finally {
      busy = false;
    }
  }

  onMount(async () => {
    try {
      [catalog, models, settings] = await Promise.all([api.cases(), api.models(), api.settings()]);
    } catch (loadError) {
      error = errorMessage(loadError);
    }
  });
</script>

<div class="page">
  <div class="page-head">
    <div>
      <a class="back" href="#/cas">Tous les cas</a>
      <h1>{current?.title ?? id}</h1>
    </div>
    <CampaignPicker bind:value={campaignId} campaigns={picked.list} />
  </div>

  <Notice message={error || results.error} />

  {#if catalog && !current}
    <p class="empty">Ce cas n'existe pas.</p>
  {:else if current}
    <section class="panel controls" aria-label="Modèles du cas">
      <div class="chips">
        {#each models?.connections ?? [] as connection (connection.provider.id)}
          {#each connection.models.filter((model) => !embedding.has(model.name) && !embedding.has(model.name.split(':')[0])) as model (model.name)}
            <label class="chip" class:on={isSelected(connection.provider.id, model.name)}>
              <input type="checkbox" checked={isSelected(connection.provider.id, model.name)} onchange={() => toggle(connection.provider.id, model.name)} />
              <ModelName name={model.name} />
            </label>
          {/each}
        {/each}
      </div>
      {#if live.campaign}
        <a class="btn" href="#/labo"><Icon name="flask" /> Campagne en cours : voir le Labo</a>
      {:else}
        <button class="btn btn-primary" type="button" onclick={runCase} disabled={busy || selections.length === 0}>
          <Icon name="play" /> Lancer ce cas
        </button>
      {/if}
    </section>

    {#each current.items as item (item.id)}
      {@const itemRuns = runsByItem.get(item.id) ?? []}
      {@const retrieved = itemRuns.find((run) => run.retrieved)?.retrieved ?? []}
      {@const hit = itemRuns.find((run) => run.retrieval_hit !== null)?.retrieval_hit}
      {@const parts = splitPrompt(item.prompt)}
      <section class="item">
        {#if parts.context}
          <h3 class="part-label">Contexte</h3>
          <p class="context">{parts.context}</p>
        {/if}
        <h3 class="part-label">Demande</h3>
        <div class="question">
          <p class="prompt">{parts.request}</p>
          {#if item.rag}
            <div class="rag">
              <span class="state"><Icon name="alert" size={16} /> {trapLabel(item.rag.trap)}</span>
              {#each item.rag.sources as expected (expected.file + expected.quote)}
                <button class="btn" type="button" onclick={() => (source = expected)}><Icon name="doc" /> {expected.file}</button>
              {/each}
              {#if hit === true}
                <span class="state state-good"><Icon name="check" size={16} /> Passage retrouvé</span>
              {:else if hit === false}
                <span class="state state-critical"><Icon name="cross" size={16} /> Passage manqué</span>
              {/if}
            </div>
          {/if}
          {#if retrieved.length > 0}
            <details>
              <summary>Passages retrouvés ({retrieved.length})</summary>
              <ol>
                {#each retrieved as passage (passage.file + passage.position)}
                  <li>
                    <p class="muted">{passage.file}, similarité {number(passage.similarity * 100)} %</p>
                    <pre class="output">{passage.content}</pre>
                  </li>
                {/each}
              </ol>
            </details>
          {/if}
        </div>

        <h3 class="part-label">Résultats</h3>
        {#if itemRuns.length > 0}
          <div class="columns">
            {#each itemRuns as run (run.id || run.model)}
              <RunColumn {run} color={seriesColor(run.model, modelNames)} />
            {/each}
          </div>
        {:else}
          <p class="muted">Pas de résultat dans cette campagne.</p>
        {/if}
      </section>
    {/each}
  {/if}
</div>

<SourceDialog {corpus} {source} onclose={() => (source = null)} />

<style>
  .back {
    display: inline-block;
    margin-bottom: var(--s2);
    color: var(--ink-2);
    font-size: var(--text-sm);
  }

  .controls {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: var(--s3);
  }

  .chips {
    display: flex;
    flex-wrap: wrap;
    gap: var(--s2);
  }

  .chip {
    display: inline-flex;
    align-items: center;
    gap: var(--s2);
    min-height: 40px;
    padding: var(--s1) var(--s3);
    border: 1px solid var(--axis);
    border-radius: var(--radius);
    cursor: pointer;
  }

  .chip.on {
    border-color: var(--accent);
    background: color-mix(in srgb, var(--accent) 14%, transparent);
  }

  /* A label rail keeps context, request and results apart without one more box. */
  .item {
    display: grid;
    grid-template-columns: 6.5rem minmax(0, 1fr);
    gap: var(--s3) var(--s4);
    padding-top: var(--s5);
    border-top: 1px solid var(--hairline);
  }

  .part-label {
    padding-top: 2px;
    color: var(--ink-3);
    font-size: var(--text-sm);
    font-weight: 500;
  }

  .context {
    max-width: 75ch;
    color: var(--ink-2);
    white-space: pre-wrap;
  }

  .question {
    display: grid;
    gap: var(--s2);
  }

  .prompt {
    max-width: 75ch;
    font-size: var(--text-lg);
    font-weight: 550;
    white-space: pre-wrap;
  }

  .rag {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--s2) var(--s3);
  }

  summary {
    color: var(--ink-2);
    font-size: var(--text-sm);
    cursor: pointer;
  }

  ol {
    display: grid;
    gap: var(--s3);
    margin: var(--s2) 0 0;
    padding: 0;
    list-style: none;
    font-size: var(--text-sm);
  }

  .columns {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(min(100%, 320px), 1fr));
    gap: var(--s3);
  }

  @media (max-width: 760px) {
    .item {
      grid-template-columns: minmax(0, 1fr);
      gap: var(--s2);
    }
  }
</style>
