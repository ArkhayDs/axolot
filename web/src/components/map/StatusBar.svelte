<script lang="ts">
  import CounterMenu from './CounterMenu.svelte';
  import Icon from '../Icon.svelte';
  import { categoryLabel } from '../../lib/format';
  import { live } from '../../lib/live.svelte';
  import type { MapContent } from '../../lib/map/content';
  import type { Findable } from '../../lib/map/findable';
  import type { NodeKind } from '../../lib/map/graph';
  import type { MapFilter } from '../../lib/map/ranking';
  import { shortModel } from '../../lib/map/ranking';

  let {
    content,
    filter,
    focus,
    focusLabel,
    launching,
    findable,
    onfind,
    onremove,
    onclearfocus,
    onclearall,
    onrecenter,
    onlaunch,
  }: {
    content: MapContent;
    filter: MapFilter;
    focus: { kind: NodeKind; key: string } | null;
    focusLabel: string;
    launching: boolean;
    findable: Findable[];
    onfind: (entry: Findable) => void;
    onremove: (axis: keyof MapFilter) => void;
    onclearfocus: () => void;
    onclearall: () => void;
    onrecenter: () => void;
    onlaunch: () => void;
  } = $props();

  let query = $state('');
  let missing = $state(false);
  let openKind = $state<NodeKind | ''>('');

  const counters = $derived<{ kind: NodeKind; value: number; unit: string; secondary: boolean }[]>([
    { kind: 'machine', value: content.machines.length, unit: 'machine', secondary: true },
    { kind: 'model', value: content.models.length, unit: 'modèle', secondary: false },
    { kind: 'theme', value: content.themes.length, unit: 'thème', secondary: false },
    { kind: 'scenario', value: content.scenarios.length, unit: 'scénario', secondary: true },
    { kind: 'question', value: content.questions.length, unit: 'question', secondary: false },
    { kind: 'document', value: content.documents.size, unit: 'document', secondary: false },
    { kind: 'benchmark', value: content.benchmarks.length, unit: 'benchmark', secondary: true },
  ]);
  const pills = $derived(
    [
      { axis: 'machine' as const, label: filter.machine },
      { axis: 'model' as const, label: filter.model ? shortModel(filter.model, content.models) : '' },
      { axis: 'category' as const, label: filter.category ? categoryLabel(filter.category) : '' },
    ].filter((pill) => pill.label),
  );

  /**
   * @param {Findable} entry - line of a counter list
   * @returns {boolean} - whether the selection holds it, same state as a click on its node
   */
  function isSelected(entry: Findable): boolean {
    if (entry.kind === 'machine') return filter.machine === entry.key;
    if (entry.kind === 'model') return filter.model === entry.key;
    if (entry.kind === 'theme') return filter.category === entry.key;
    return focus?.kind === entry.kind && focus.key === entry.key;
  }

  /**
   * @param {boolean} refocus - whether the keyboard goes back to the counter
   */
  function closeMenu(refocus: boolean): void {
    const kind = openKind;
    openKind = '';
    if (refocus) document.querySelector<HTMLButtonElement>(`[data-counter="${kind}"]`)?.focus();
  }

  /** @param {SubmitEvent} event - opens the node named in the search field */
  function find(event: SubmitEvent): void {
    event.preventDefault();
    const wanted = query.trim().toLowerCase();
    if (!wanted) return;
    const entry = findable.find((candidate) => candidate.label.toLowerCase() === wanted) ?? findable.find((candidate) => candidate.label.toLowerCase().includes(wanted));
    missing = !entry;
    if (!entry) return;
    onfind(entry);
    query = '';
  }
</script>

<footer class="status">
  <ul class="counters" aria-label="Contenu de l'Atlas">
    {#each counters as counter (counter.kind)}
      <li class:secondary={counter.secondary}>
        <button
          type="button"
          class="counter"
          data-counter={counter.kind}
          aria-expanded={openKind === counter.kind}
          onclick={() => {
            openKind = openKind === counter.kind ? '' : counter.kind;
          }}
        >
          <strong>{counter.value}</strong> {counter.unit}{counter.value > 1 ? 's' : ''}
        </button>
        {#if openKind === counter.kind}
          <CounterMenu title="{counter.unit}s de l'Atlas" entries={findable.filter((entry) => entry.kind === counter.kind)} {isSelected} onpick={onfind} onclose={closeMenu} />
        {/if}
      </li>
    {/each}
  </ul>

  <div class="pills" aria-label="Sélection active">
    {#each pills as pill (pill.axis)}
      <button type="button" class="pill" title={pill.label} aria-label="Retirer {pill.label}" onclick={() => onremove(pill.axis)}><span>{pill.label}</span> <Icon name="cross" size={13} /></button>
    {/each}
    {#if focusLabel}
      <button type="button" class="pill" title={focusLabel} aria-label="Retirer {focusLabel}" onclick={onclearfocus}><span>{focusLabel}</span> <Icon name="cross" size={13} /></button>
    {/if}
    {#if pills.length > 0 || focusLabel}
      <button type="button" class="link" onclick={onclearall}>Tout effacer</button>
    {:else}
      <span class="hint">Cliquez sur un élément pour filtrer</span>
    {/if}
  </div>

  <form class="search" role="search" onsubmit={find}>
    <Icon name="search" size={15} />
    <input
      type="search"
      list="atlas-nodes"
      placeholder="Chercher un nœud"
      aria-label="Chercher un nœud par son nom"
      aria-invalid={missing}
      bind:value={query}
      oninput={() => {
        missing = false;
      }}
    />
    <datalist id="atlas-nodes">
      {#each findable as entry (`${entry.kind}:${entry.key}`)}
        <option value={entry.label} label={entry.type}></option>
      {/each}
    </datalist>
    {#if missing}<span class="state state-warning" role="status">Aucun nœud</span>{/if}
  </form>

  <button type="button" class="tool" onclick={onrecenter}><Icon name="center" size={16} /> Recentrer</button>
  <button type="button" class="btn btn-primary launch" aria-expanded={launching} disabled={live.campaign !== ''} onclick={onlaunch}>
    <Icon name="play" size={16} /> Lancer un Benchmark
  </button>
</footer>

<style>
  .status {
    display: flex;
    align-items: center;
    gap: var(--s3);
    min-height: 44px;
    padding: var(--s1) var(--s4);
    border-top: 1px solid var(--hairline);
    background: var(--panel);
    color: var(--ink-3);
    font-size: 0.75rem;
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
  }

  ul {
    display: flex;
    flex: none;
    margin: 0;
    padding: 0;
    list-style: none;
  }

  li {
    position: relative;
    display: inline-flex;
    align-items: center;
    padding-inline: var(--s1);
  }

  .counter {
    display: inline-flex;
    align-items: center;
    gap: 3px;
    height: 28px;
    padding: 0 var(--s1);
    border: 0;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--ink-3);
    font: inherit;
    cursor: pointer;
  }

  .counter:hover,
  .counter[aria-expanded='true'] {
    background: var(--raised);
    color: var(--ink);
  }

  .hint {
    overflow: hidden;
    color: var(--ink-3);
    text-overflow: ellipsis;
  }

  li + li {
    border-inline-start: 1px solid var(--hairline);
  }

  strong {
    color: var(--ink);
    font-size: var(--text-sm);
    font-weight: 650;
  }

  /* One row whatever the selection: the framing of the graph relies on the height of this bar. */
  .pills {
    display: flex;
    flex: 1;
    align-items: center;
    gap: var(--s1);
    min-width: 0;
    overflow: hidden;
  }

  .pill {
    display: inline-flex;
    align-items: center;
    flex: 0 1 auto;
    gap: var(--s1);
    min-width: 4.5rem;
    height: 26px;
    padding: 0 var(--s2);
    border: 1px solid var(--accent);
    border-radius: var(--radius-sm);
    background: color-mix(in srgb, var(--accent) 14%, transparent);
    color: var(--ink);
    font-size: 0.75rem;
    cursor: pointer;
  }

  .pill span {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .link,
  .tool {
    flex: none;
    border: 0;
    background: transparent;
    color: var(--ink-2);
    font-size: 0.75rem;
    cursor: pointer;
  }

  .link {
    text-decoration: underline;
    text-underline-offset: 2px;
  }

  .link:hover,
  .tool:hover {
    color: var(--ink);
  }

  .tool {
    display: inline-flex;
    align-items: center;
    gap: var(--s1);
    height: 30px;
    padding: 0 var(--s2);
  }

  .search {
    display: flex;
    flex: none;
    align-items: center;
    gap: var(--s1);
    height: 30px;
    padding-inline: var(--s2);
    border: 1px solid var(--hairline);
    border-radius: var(--radius-sm);
    background: var(--ground);
  }

  .search:focus-within {
    border-color: var(--accent);
  }

  .search input {
    width: 9.5rem;
    border: 0;
    outline: 0;
    background: transparent;
    color: var(--ink);
    font-size: 0.75rem;
  }

  .search input::placeholder {
    color: var(--ink-3);
  }

  .launch {
    flex: none;
    min-height: 32px;
    padding: var(--s1) var(--s3);
    font-size: 0.8125rem;
  }

  /* The selection pills need the room more than the finer counters do. */
  @media (max-width: 1320px) {
    .secondary {
      display: none;
    }

    .search input {
      width: 8rem;
    }

    li:not(.secondary) + li:not(.secondary) {
      border-inline-start: 1px solid var(--hairline);
    }
  }

  @media (max-width: 760px) {
    .status {
      flex-wrap: wrap;
      white-space: normal;
    }

    ul {
      flex-wrap: wrap;
    }
  }
</style>
