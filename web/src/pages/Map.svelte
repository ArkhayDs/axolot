<script lang="ts">
  import { onMount, tick } from 'svelte';

  import Icon from '../components/Icon.svelte';
  import Notice from '../components/Notice.svelte';
  import BenchmarkLauncher from '../components/map/BenchmarkLauncher.svelte';
  import BenchmarkStack from '../components/map/BenchmarkStack.svelte';
  import LivePanel from '../components/map/LivePanel.svelte';
  import MapCanvas from '../components/map/MapCanvas.svelte';
  import NodeDetail from '../components/map/NodeDetail.svelte';
  import RankingPanel from '../components/map/RankingPanel.svelte';
  import StatusBar from '../components/map/StatusBar.svelte';
  import { api, errorMessage } from '../lib/api';
  import type { CasesResponse, ConnectionType, MapData } from '../lib/api';
  import { benchmarkName, categoryLabel } from '../lib/format';
  import { contentOf, rankOnQuestions } from '../lib/map/content';
  import { resultCounts } from '../lib/map/figures';
  import { findableOf } from '../lib/map/findable';
  import type { GraphNode, NodeKind } from '../lib/map/graph';
  import { indexCells, listsOf, rankSections, shortModel } from '../lib/map/ranking';
  import type { MapFilter } from '../lib/map/ranking';
  import { indexItems } from '../lib/map/results';
  import { live } from '../lib/live.svelte';

  type Focus = { kind: NodeKind; key: string };

  let data = $state<MapData | null>(null);
  let catalog = $state<CasesResponse | null>(null);
  let error = $state('');
  let launching = $state(false);
  let filter = $state<MapFilter>({ machine: '', model: '', category: '' });
  let connection = $state<ConnectionType>('');
  let asked = 0;
  let focus = $state.raw<Focus | null>(null);
  let pinned = $state('');
  let following = $state(false);
  let lastCampaign = '';
  let unreachable = $state(false);
  let mounted = false;
  let retry: ReturnType<typeof setTimeout> | undefined;
  let sideEl = $state<HTMLDivElement | undefined>();
  let statusEl = $state<HTMLDivElement | undefined>();
  let canvas = $state<MapCanvas | undefined>();

  const RETRY_MS = 3000;
  // Benchmark popups and selection row, which appear with a selection: the framing stays put when they do.
  const SELECTION_ROOM = 88;
  const phone = window.matchMedia('(max-width: 760px)');
  const running = $derived(live.campaign !== '');
  const lists = $derived(data ? listsOf(data) : null);
  const cells = $derived(indexCells(data?.cells ?? []));
  const items = $derived(indexItems(data?.items ?? []));
  const content = $derived(lists && catalog && data ? contentOf(lists, catalog.cases, data.benchmarks) : null);
  const sections = $derived(lists ? rankSections(cells, lists, filter) : []);
  const focusQuestions = $derived.by(() => {
    if (!content || !focus) return null;
    const key = focus.key;
    if (focus.kind === 'scenario') return content.questions.filter((question) => question.caseId === key);
    return focus.kind === 'document' ? (content.documents.get(key) ?? []) : null;
  });
  const shownSections = $derived(content && focusQuestions ? rankOnQuestions(items, content.models, filter.machine, focusQuestions) : sections);
  const focusLabel = $derived.by(() => {
    if (!focus || !content) return '';
    const key = focus.key;
    if (focus.kind === 'scenario') return content.scenarios.find((scenario) => scenario.id === key)?.title ?? key;
    if (focus.kind === 'question') return key.split('/').at(-1) ?? key;
    if (focus.kind === 'benchmark') return benchmarkName(content.benchmarks.find((benchmark) => benchmark.id === key)?.name ?? key);
    return key;
  });
  const heading = $derived.by(() => {
    if (focusQuestions) return 'Modèles sur ces questions';
    const parts = [
      filter.model && lists ? shortModel(filter.model, lists.models) : '',
      filter.category ? categoryLabel(filter.category) : '',
      filter.machine,
    ].filter(Boolean);
    return parts.length > 0 ? `Classement : ${parts.join(', ')}` : 'Classement général';
  });
  const results = $derived(content && filter.model ? resultCounts(content, items, filter) : null);
  const findable = $derived(content ? findableOf(content, cells, items, filter) : []);

  /**
   * Loads the cumulated figures of the connection type and the scenarios; the last ones stay on screen when the
   * server is away. Only the last request lands, an earlier one answering late would show a type already left.
   */
  async function load(): Promise<void> {
    clearTimeout(retry);
    const request = ++asked;
    try {
      const loaded = await Promise.all([api.map(connection), api.cases()]);
      if (request !== asked) return;
      [data, catalog] = loaded;
      unreachable = false;
      error = '';
    } catch (loadError) {
      if (request !== asked) return;
      // The API words its own errors; a bare status comes from the dev proxy while the app restarts.
      const message = errorMessage(loadError);
      unreachable = message === 'Serveur injoignable' || /^Erreur 5\d\d$/.test(message);
      error = unreachable ? '' : message;
      if (mounted) retry = setTimeout(load, RETRY_MS);
    }
  }

  /**
   * Machine, model and theme combine as filters; the finer nodes open their detail. A second click clears
   * @param {NodeKind} kind - node type
   * @param {string} key - identity within the type
   */
  function pick(kind: NodeKind, key: string): void {
    if (kind === 'machine') filter.machine = filter.machine === key ? '' : key;
    else if (kind === 'model') filter.model = filter.model === key ? '' : key;
    else if (kind === 'theme') filter.category = filter.category === key ? '' : key;
    else focus = focus?.kind === kind && focus.key === key ? null : { kind, key };
  }

  /** Closes the launcher and gives focus back to the button it replaced */
  async function closeLauncher(): Promise<void> {
    launching = false;
    await tick();
    statusEl?.querySelector<HTMLButtonElement>('.launch')?.focus();
  }

  /** @returns {{ top: number; right: number; bottom: number; left: number }} - canvas margins the panels cover */
  function reserve(): { top: number; right: number; bottom: number; left: number } {
    if (phone.matches || !statusEl || !sideEl) return { top: 8, right: 8, bottom: 8, left: 8 };
    const stage = statusEl.getBoundingClientRect();
    return { top: sideEl.getBoundingClientRect().top + 8, right: window.innerWidth - sideEl.getBoundingClientRect().left + 12, bottom: window.innerHeight - stage.top + SELECTION_ROOM, left: 16 };
  }

  onMount(() => {
    mounted = true;
    void load();
    return () => {
      mounted = false;
      clearTimeout(retry);
    };
  });

  let seenReconnects = live.reconnects;
  $effect(() => {
    if (live.reconnects === seenReconnects) return;
    seenReconnects = live.reconnects;
    void load();
  });

  let seenConnection: ConnectionType = '';
  $effect(() => {
    if (connection === seenConnection) return;
    seenConnection = connection;
    void load();
  });

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
    following = false;
    void load();
  });
</script>

<div class="map">
  {#if content}
    <MapCanvas
      bind:this={canvas}
      {content}
      {cells}
      {items}
      {filter}
      {connection}
      {focus}
      {sections}
      follow={following}
      {reserve}
      onpick={(node: GraphNode) => pick(node.kind, node.key)}
      onclear={() => (focus = null)}
      onunfollow={() => (following = false)}
    />
  {/if}

  <div class="side" bind:this={sideEl}>
    <Notice message={error} />
    {#if unreachable || !live.connected}
      <p class="reconnecting glass" role="status"><Icon name="refresh" size={16} /> Reconnexion...</p>
    {/if}
    {#if running}
      <LivePanel {catalog} {following} onfollow={() => (following = !following)} />
    {:else if launching}
      <BenchmarkLauncher onclose={closeLauncher} />
    {:else if content}
      {#if focus}
        <NodeDetail {focus} {content} {items} {filter} onclose={() => (focus = null)} />
      {/if}
      {#if focus?.kind !== 'question' && focus?.kind !== 'benchmark'}
        <RankingPanel sections={shownSections} {heading} models={content.models} onpick={pick} />
      {/if}
    {/if}
  </div>

  <div class="popups">
    <BenchmarkStack benchmarks={data?.benchmarks ?? []} {filter} {pinned} />
  </div>

  <div class="footer" bind:this={statusEl}>
    {#if content}
      <StatusBar
        {content}
        {filter}
        {focus}
        {focusLabel}
        {results}
        {launching}
        {findable}
        {connection}
        onconnection={(value) => (connection = value)}
        onfind={(entry) => pick(entry.kind, entry.key)}
        onremove={(axis) => {
          filter[axis] = '';
        }}
        onclearfocus={() => (focus = null)}
        onclearall={() => {
          filter = { machine: '', model: '', category: '' };
          focus = null;
        }}
        onrecenter={() => canvas?.recenter()}
        onlaunch={() => (launching = true)}
      />
    {/if}
  </div>
</div>

<style>
  /* Empty grid cells let the pointer through to the canvas, positioned behind them. */
  .map {
    position: fixed;
    inset: 0;
    display: grid;
    grid-template-columns: minmax(0, 1fr) clamp(320px, 27vw, 380px);
    grid-template-rows: minmax(0, 1fr) auto;
    gap: var(--s3);
    padding-top: calc(var(--s3) + 58px + var(--s3));
  }

  .side,
  .popups,
  .footer {
    position: relative;
    z-index: 1;
    min-width: 0;
    min-height: 0;
  }

  .side {
    grid-area: 1 / 2;
    display: flex;
    flex-direction: column;
    justify-self: stretch;
    align-self: start;
    max-height: 100%;
    margin-inline-end: var(--s3);
  }

  .reconnecting {
    display: flex;
    align-items: center;
    gap: var(--s2);
    margin-bottom: var(--s2);
    padding: var(--s2) var(--s3);
    border-radius: var(--radius);
    font-size: var(--text-sm);
  }

  .popups {
    grid-area: 1 / 1;
    align-self: end;
    justify-self: start;
    width: calc(100% - 2 * var(--s3));
    margin-inline-start: var(--s3);
    pointer-events: none;
  }

  .footer {
    grid-area: 2 / 1 / 3 / 3;
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
      margin-inline-end: 0;
    }

    .popups {
      width: 100%;
      margin-inline-start: 0;
    }
  }
</style>
