<script lang="ts">
  import Icon from '../Icon.svelte';
  import type { IconName } from '../Icon.svelte';
  import { BASE_VIEW, picked } from '../../lib/campaigns.svelte';
  import { benchmarkName, score, statusLabel, when } from '../../lib/format';
  import type { MapBenchmark } from '../../lib/api';
  import { benchmarkScore } from '../../lib/map/ranking';
  import type { MapFilter } from '../../lib/map/ranking';

  let {
    benchmarks,
    filter,
    pinned,
  }: { benchmarks: MapBenchmark[]; filter: MapFilter; pinned: string } = $props();

  const SHOWN = 4;
  const STATES: Record<MapBenchmark['status'], { icon: IconName; tone: string }> = {
    running: { icon: 'refresh', tone: 'state-warning' },
    done: { icon: 'check', tone: 'state-good' },
    cancelled: { icon: 'cross', tone: 'state-warning' },
    interrupted: { icon: 'alert', tone: 'state-warning' },
  };

  let closed = $state<string[]>([]);

  const selecting = $derived(Boolean(filter.machine || filter.model || filter.category));
  const matching = $derived(
    benchmarks
      .map((benchmark) => ({ benchmark, value: benchmarkScore(benchmark, filter) }))
      .filter(({ benchmark, value }) => !closed.includes(benchmark.id) && value !== null && (selecting || benchmark.id === pinned)),
  );
  const shown = $derived(matching.slice(0, SHOWN));

  // A new selection brings back what was closed for the previous one.
  $effect(() => {
    void [filter.machine, filter.model, filter.category, pinned];
    closed = [];
  });

  /** @param {string} id - opens this benchmark in the results page */
  function open(id: string): void {
    picked.id = id;
    picked.view = id;
  }
</script>

{#if shown.length > 0}
  <section class="stack" aria-label="Benchmarks de la sélection">
    {#if matching.length > SHOWN}
      <p class="more glass">+{matching.length - SHOWN} autres dans Résultats</p>
    {/if}
    <ul>
      {#each shown as { benchmark, value }, position (benchmark.id)}
        <li class="card glass" style:--i={position}>
          <a href="#/resultats" onclick={() => open(benchmark.id)}>
            <span class="head">
              <strong>{benchmarkName(benchmark.name)}</strong>
              <span class="figure">{value === null ? '-' : score(value)}<small>/100</small></span>
            </span>
            <span class="meta">
              <span class="where">{when(benchmark.created_at)}, {benchmark.machine}</span>
              <span class="state {STATES[benchmark.status].tone}"><Icon name={STATES[benchmark.status].icon} size={14} /> {statusLabel(benchmark.status)}</span>
            </span>
          </a>
          <button
            type="button"
            class="close"
            aria-label="Fermer {benchmarkName(benchmark.name)}"
            onclick={() => {
              closed = [...closed, benchmark.id];
            }}
          >
            <Icon name="cross" size={16} />
          </button>
        </li>
      {/each}
    </ul>
  </section>
{/if}

<style>
  .stack {
    display: grid;
    gap: var(--s1);
    width: 100%;
  }

  ul {
    display: grid;
    gap: var(--s1);
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .more {
    justify-self: start;
    padding: var(--s1) var(--s3);
    border-radius: var(--radius-sm);
    color: var(--ink-2);
    font-size: var(--text-sm);
  }

  /* Corner brackets give the heads-up display look; the rest stays the app's glass. */
  .card {
    position: relative;
    border-radius: var(--radius-sm);
    animation: card-in 0.5s var(--ease-out) backwards;
    animation-delay: calc(var(--i) * 60ms);
  }

  .card::before,
  .card::after {
    position: absolute;
    width: 14px;
    height: 14px;
    border: solid var(--accent);
    content: '';
    pointer-events: none;
  }

  .card::before {
    top: -1px;
    left: -1px;
    border-width: 2px 0 0 2px;
    border-top-left-radius: var(--radius-sm);
  }

  .card::after {
    right: -1px;
    bottom: -1px;
    border-width: 0 2px 2px 0;
    border-bottom-right-radius: var(--radius-sm);
  }

  @keyframes card-in {
    from {
      opacity: 0;
      filter: blur(6px);
      transform: translateX(-24px);
    }
  }

  a {
    display: grid;
    gap: 0;
    padding: var(--s1) calc(var(--s2) + 28px) var(--s1) var(--s3);
    border-radius: var(--radius-sm);
    text-decoration: none;
    transition: background-color 0.15s;
  }

  a:hover {
    background: color-mix(in srgb, var(--accent) 12%, transparent);
  }

  .head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: var(--s3);
  }

  strong {
    overflow: hidden;
    font-weight: 600;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .figure {
    flex: none;
    font-family: var(--mono);
    font-size: var(--text-lg);
    font-variant-numeric: tabular-nums;
    font-weight: 650;
  }

  small {
    color: var(--ink-2);
    font-size: var(--text-sm);
    font-weight: 400;
  }

  .meta {
    display: flex;
    justify-content: space-between;
    gap: var(--s2);
    color: var(--ink-2);
    font-size: var(--text-sm);
  }

  .where {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .state {
    flex: none;
  }

  .close {
    position: absolute;
    top: 50%;
    right: var(--s1);
    translate: 0 -50%;
    display: grid;
    place-items: center;
    width: 28px;
    height: 28px;
    padding: 0;
    border: 0;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--ink-2);
    cursor: pointer;
  }

  .close:hover {
    color: var(--ink);
    background: color-mix(in srgb, var(--ink) 12%, transparent);
  }
</style>
