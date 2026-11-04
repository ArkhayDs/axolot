<script lang="ts">
  import Icon from '../Icon.svelte';
  import ModelName from '../ModelName.svelte';
  import Notice from '../Notice.svelte';
  import RunMatrix from '../RunMatrix.svelte';
  import { api, errorMessage } from '../../lib/api';
  import type { CasesResponse, Estimate } from '../../lib/api';
  import { duration, estimateLabel, number, score } from '../../lib/format';
  import { live } from '../../lib/live.svelte';

  let { catalog }: { catalog: CasesResponse | null } = $props();

  let error = $state('');
  let outputBox = $state<HTMLPreElement | undefined>();
  let remaining = $state<Estimate | null>(null);

  const currentCase = $derived(catalog?.cases.find((item) => item.id === live.caseId));
  const lastRun = $derived(live.runs.at(-1));
  const matrixModels = $derived([...new Set([...live.runs.map((run) => run.model), ...(live.model ? [live.model] : [])])].sort());
  const matrixCases = $derived((catalog?.cases ?? []).filter((item) => item.id === live.caseId || live.runs.some((run) => run.case_id === item.id)));

  /** Stops the running benchmark; the current question is abandoned */
  async function cancel(): Promise<void> {
    try {
      await api.cancelCampaign(live.campaign);
    } catch (cancelError) {
      error = errorMessage(cancelError);
    }
  }

  $effect(() => {
    void live.done;
    api
      .remaining()
      .then((loaded) => {
        remaining = loaded;
      })
      .catch(() => undefined);
  });

  $effect(() => {
    void live.output;
    if (outputBox) outputBox.scrollTop = outputBox.scrollHeight;
  });
</script>

<aside class="live glass" aria-label="Benchmark en cours">
  <div class="head">
    <h2>{currentCase?.title ?? 'Démarrage'}</h2>
    <button class="btn" type="button" onclick={cancel}><Icon name="stop" /> Arrêter</button>
  </div>
  <Notice message={error} />
  {#if live.model}<ModelName name={live.model} />{/if}
  <div class="progress" role="progressbar" aria-valuemin="0" aria-valuemax={live.total} aria-valuenow={live.done} aria-label="Progression">
    <span style:width="{live.total ? (100 * live.done) / live.total : 0}%"></span>
  </div>
  <p class="counts">
    <span>{live.done} / {live.total} réponses</span>
    <span>{live.tokens} fragments</span>
  </p>
  {#if remaining && remaining.answers > 0}
    <p class="left">Reste : <strong>{estimateLabel(remaining)}</strong>{remaining.known && remaining.partial ? ', estimation partielle' : ''}</p>
  {/if}
  <pre class="output" bind:this={outputBox} aria-label="Sortie du modèle">{live.output || (live.model ? 'Chargement du modèle...' : '')}</pre>
  <div aria-live="polite">
    {#if lastRun}
      <p class="counts">
        {#if lastRun.error}
          <span class="state state-critical"><Icon name="cross" size={16} /> Échec</span>
        {:else}
          <strong>{score(lastRun.score)}/100</strong>
        {/if}
        <span class="muted">{duration(lastRun.total_ms)}, {number(lastRun.tok_per_sec)} jetons/s</span>
      </p>
    {/if}
  </div>
  <RunMatrix cases={matrixCases} models={matrixModels} runs={live.runs} activeItem={live.item} activeModel={live.model} />
</aside>

<style>
  .live {
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
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--s3);
  }

  .counts {
    display: flex;
    flex-wrap: wrap;
    justify-content: space-between;
    gap: var(--s2);
    font-size: var(--text-sm);
    font-variant-numeric: tabular-nums;
  }

  .counts strong {
    font-size: var(--text-lg);
  }

  .left {
    font-size: var(--text-sm);
  }

  .left strong {
    font-weight: 650;
  }

  .output {
    flex: none;
    max-height: 9rem;
  }

  /* The matrix panel keeps a solid ground but loses its roomy page padding here. */
  .live :global(.matrix) {
    flex: none;
    padding: var(--s3);
  }
</style>
