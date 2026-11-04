<script lang="ts">
  import { onMount } from 'svelte';

  import EvalMatrix from '../components/EvalMatrix.svelte';
  import LatencyStrip from '../components/LatencyStrip.svelte';
  import ModelName from '../components/ModelName.svelte';
  import Notice from '../components/Notice.svelte';
  import QualityTable from '../components/QualityTable.svelte';
  import RunMatrix from '../components/RunMatrix.svelte';
  import ThemeTradeoff from '../components/ThemeTradeoff.svelte';
  import StatsTable from '../components/StatsTable.svelte';
  import { api, errorMessage } from '../lib/api';
  import type { Case } from '../lib/api';
  import { campaignResults, picked } from '../lib/campaigns.svelte';
  import { benchmarkName, duration, seriesColor, statusLabel, when } from '../lib/format';

  let { id }: { id: string } = $props();

  const TABS = [
    { key: 'summary', label: 'Résumé' },
    { key: 'times', label: 'Temps' },
    { key: 'answers', label: 'Réponses' },
    { key: 'quality', label: 'Qualimétrie' },
  ] as const;

  let tab = $state<(typeof TABS)[number]['key']>('summary');
  let cases = $state<Case[]>([]);
  let casesError = $state('');

  const results = campaignResults(() => id);
  const detail = $derived(results.detail);
  const stats = $derived(detail?.stats ?? []);
  const models = $derived([...new Set(stats.map((stat) => stat.model))].sort());
  const playedCases = $derived(cases.filter((item) => detail?.runs.some((run) => run.case_id === item.id)));
  const elapsed = $derived(
    detail?.campaign.finished_at ? new Date(detail.campaign.finished_at).getTime() - new Date(detail.campaign.created_at).getTime() : null,
  );

  /**
   * Arrow keys move between tabs, as the tabs pattern expects
   * @param {KeyboardEvent} event - key press on a tab
   * @param {number} position - index of the focused tab
   */
  function onTabKey(event: KeyboardEvent, position: number): void {
    const moves: Record<string, number> = { ArrowRight: position + 1, ArrowLeft: position - 1, Home: 0, End: TABS.length - 1 };
    const target = moves[event.key];
    if (target === undefined) return;
    event.preventDefault();
    const next = (target + TABS.length) % TABS.length;
    tab = TABS[next].key;
    document.getElementById(`tab-${TABS[next].key}`)?.focus();
  }

  onMount(async () => {
    picked.id = id;
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
      <a class="back" href="#/benchmarks">Tous les Benchmarks</a>
      <h1>{detail ? benchmarkName(detail.campaign.name) : 'Benchmark'}</h1>
      {#if detail}
        <p>{when(detail.campaign.created_at)}, {statusLabel(detail.campaign.status).toLowerCase()}, {detail.campaign.done_items} réponses sur {detail.campaign.total_items} prévues</p>
      {/if}
    </div>
  </div>

  <Notice message={results.error || casesError} />

  {#if detail}
    <div class="tabs" role="tablist" aria-label="Vues du Benchmark">
      {#each TABS as item, position (item.key)}
        <button
          id="tab-{item.key}"
          type="button"
          role="tab"
          aria-selected={tab === item.key}
          aria-controls="panel"
          tabindex={tab === item.key ? 0 : -1}
          onclick={() => (tab = item.key)}
          onkeydown={(event) => onTabKey(event, position)}
        >
          {item.label}
        </button>
      {/each}
    </div>

    <div id="panel" class="section" role="tabpanel" aria-labelledby="tab-{tab}" tabindex="0">
      {#if tab === 'summary'}
        <dl class="facts panel">
          <div><dt>Durée</dt><dd>{elapsed === null ? 'En cours' : duration(elapsed)}</dd></div>
          <div><dt>Machine</dt><dd>{detail.campaign.hardware.label}</dd></div>
          <div>
            <dt>Modèles</dt>
            <dd class="models">
              {#each models as model (model)}
                <ModelName name={model} color={seriesColor(model, models)} />
              {/each}
            </dd>
          </div>
        </dl>
        {#if detail.verdicts.length > 0}
          <EvalMatrix {stats} verdicts={detail.verdicts} {models} />
        {/if}
      {:else if tab === 'times'}
        <ThemeTradeoff {stats} {models} />
        <LatencyStrip runs={detail.runs} {models} />
        <StatsTable {stats} {models} />
      {:else if tab === 'answers'}
        <RunMatrix cases={playedCases} {models} runs={detail.runs} />
      {:else}
        <QualityTable runs={detail.runs} {models} />
      {/if}
    </div>
  {/if}
</div>

<style>
  .back {
    display: inline-block;
    margin-bottom: var(--s2);
    color: var(--ink-2);
    font-size: var(--text-sm);
  }

  .tabs {
    display: flex;
    flex-wrap: wrap;
    gap: var(--s1);
    border-bottom: 1px solid var(--hairline);
  }

  [role='tab'] {
    min-height: 40px;
    margin-bottom: -1px;
    padding: var(--s2) var(--s4);
    border: 0;
    border-bottom: 2px solid transparent;
    background: none;
    color: var(--ink-2);
    cursor: pointer;
    transition: color 0.15s;
  }

  [role='tab']:hover {
    color: var(--ink);
  }

  [role='tab'][aria-selected='true'] {
    border-bottom-color: var(--accent);
    color: var(--ink);
    font-weight: 600;
  }

  [role='tabpanel']:focus-visible {
    outline-offset: 6px;
  }

  .facts {
    display: flex;
    flex-wrap: wrap;
    gap: var(--s3) var(--s6);
    margin: 0;
  }

  dt {
    color: var(--ink-3);
    font-size: var(--text-sm);
  }

  dd {
    margin: 0;
  }

  .models {
    display: flex;
    flex-wrap: wrap;
    gap: var(--s1) var(--s4);
  }
</style>
