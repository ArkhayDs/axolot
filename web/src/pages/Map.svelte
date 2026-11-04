<script lang="ts">
  import { onMount } from 'svelte';

  import Notice from '../components/Notice.svelte';
  import BenchmarkLauncher from '../components/map/BenchmarkLauncher.svelte';
  import BenchmarkStack from '../components/map/BenchmarkStack.svelte';
  import LivePanel from '../components/map/LivePanel.svelte';
  import MapCanvas from '../components/map/MapCanvas.svelte';
  import MapFilters from '../components/map/MapFilters.svelte';
  import RankingPanel from '../components/map/RankingPanel.svelte';
  import { api, errorMessage } from '../lib/api';
  import type { CasesResponse, MapData } from '../lib/api';
  import { categoryLabel } from '../lib/format';
  import type { GraphNode } from '../lib/map/graph';
  import { indexCells, listsOf, rankSections, shortModel } from '../lib/map/ranking';
  import type { MapFilter, RankKind } from '../lib/map/ranking';
  import { live } from '../lib/live.svelte';

  let data = $state<MapData | null>(null);
  let catalog = $state<CasesResponse | null>(null);
  let error = $state('');
  let launching = $state(false);
  let filter = $state<MapFilter>({ machine: '', model: '', category: '' });
  let pinned = $state('');
  let lastCampaign = '';
  let filtersEl = $state<HTMLDivElement | undefined>();
  let sideEl = $state<HTMLDivElement | undefined>();

  const phone = window.matchMedia('(max-width: 760px)');
  const running = $derived(live.campaign !== '');
  const lists = $derived(data ? listsOf(data) : null);
  const index = $derived(indexCells(data?.cells ?? []));
  const sections = $derived(lists ? rankSections(index, lists, filter) : []);
  const activeCategory = $derived(catalog?.cases.find((item) => item.id === live.caseId)?.category ?? '');
  const heading = $derived.by(() => {
    const parts = [
      filter.model && lists ? shortModel(filter.model, lists.models) : '',
      filter.category ? categoryLabel(filter.category) : '',
      filter.machine,
    ].filter(Boolean);
    return parts.length > 0 ? `Classement : ${parts.join(', ')}` : 'Classement général';
  });

  /** Loads the cumulated figures and the scenarios */
  async function load(): Promise<void> {
    try {
      [data, catalog] = await Promise.all([api.map(), api.cases()]);
      error = '';
    } catch (loadError) {
      error = errorMessage(loadError);
    }
  }

  /**
   * Same state as the filter bar: a second click on a node clears it
   * @param {RankKind} kind - node type
   * @param {string} key - machine label, model tag or category
   */
  function pick(kind: RankKind, key: string): void {
    if (kind === 'machine') filter.machine = filter.machine === key ? '' : key;
    if (kind === 'model') filter.model = filter.model === key ? '' : key;
    if (kind === 'theme') filter.category = filter.category === key ? '' : key;
  }

  /** @returns {{ top: number; right: number; bottom: number; left: number }} - canvas margins the panels cover */
  function reserve(): { top: number; right: number; bottom: number; left: number } {
    if (phone.matches || !filtersEl || !sideEl) return { top: 8, right: 8, bottom: 8, left: 8 };
    return { top: filtersEl.getBoundingClientRect().bottom + 12, right: window.innerWidth - sideEl.getBoundingClientRect().left + 12, bottom: 16, left: 16 };
  }

  onMount(load);

  $effect(() => {
    if (live.campaign) lastCampaign = live.campaign;
  });

  // The end of a benchmark refreshes the figures and brings its popup up, selection or not.
  let seenFinished = live.finished;
  $effect(() => {
    if (live.finished === seenFinished) return;
    seenFinished = live.finished;
    pinned = lastCampaign;
    launching = false;
    void load();
  });
</script>

<div class="map">
  <div class="filters" bind:this={filtersEl}>
    {#if lists}
      <MapFilters bind:filter {lists} {launching} onlaunch={() => (launching = !launching)} />
    {/if}
    <Notice message={error} />
  </div>

  {#if lists}
    <MapCanvas
      {lists}
      {index}
      {filter}
      {sections}
      {activeCategory}
      categoryOf={(caseId) => catalog?.cases.find((item) => item.id === caseId)?.category ?? ''}
      {reserve}
      onpick={(node: GraphNode) => pick(node.kind, node.key)}
    />
  {/if}

  <div class="side" bind:this={sideEl}>
    {#if running}
      <LivePanel {catalog} />
    {:else if launching}
      <BenchmarkLauncher category={filter.category} onclose={() => (launching = false)} onstarted={() => void load()} />
    {:else if lists}
      <RankingPanel {sections} {heading} models={lists.models} onpick={pick} />
    {/if}
  </div>

  <div class="popups">
    <BenchmarkStack benchmarks={data?.benchmarks ?? []} {filter} {pinned} />
  </div>
</div>

<style>
  /* Empty grid cells let the pointer through to the canvas, positioned behind them. */
  .map {
    position: fixed;
    inset: 0;
    display: grid;
    grid-template-columns: minmax(0, 1fr) clamp(320px, 27vw, 380px);
    grid-template-rows: auto minmax(0, 1fr);
    gap: var(--s3);
    padding: calc(var(--s3) + 58px + var(--s3)) max(var(--s5), calc((100vw - var(--page-width)) / 2)) var(--s4);
  }

  .filters,
  .side,
  .popups {
    position: relative;
    z-index: 1;
    min-width: 0;
    min-height: 0;
  }

  .filters {
    grid-area: 1 / 1;
    display: grid;
    gap: var(--s2);
  }

  .side {
    grid-area: 1 / 2 / 3 / 3;
    display: flex;
    flex-direction: column;
    justify-self: stretch;
    align-self: start;
    max-height: 100%;
  }

  .popups {
    grid-area: 2 / 1;
    align-self: end;
    justify-self: start;
    width: min(100%, 25rem);
  }

  /* On a phone the page scrolls: map first, then rankings and popups in the flow. */
  @media (max-width: 760px) {
    .map {
      position: static;
      display: flex;
      flex-direction: column;
      padding: var(--s3) var(--s3) calc(var(--s7) + 72px);
    }

    .side {
      max-height: none;
    }

    .popups {
      width: 100%;
    }
  }
</style>
