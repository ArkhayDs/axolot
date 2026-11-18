<script lang="ts">
  import CounterMenu from './CounterMenu.svelte';
  import SelectionRow from './SelectionRow.svelte';
  import Icon from '../Icon.svelte';
  import type { ConnectionType } from '../../lib/api';
  import { CONNECTIONS, categoryLabel } from '../../lib/format';
  import { live } from '../../lib/live.svelte';
  import type { MapContent } from '../../lib/map/content';
  import type { Findable } from '../../lib/map/findable';
  import type { NodeKind } from '../../lib/map/graph';
  import type { ResultState } from '../../lib/map/results';
  import type { MapFilter } from '../../lib/map/ranking';
  import { shortModel } from '../../lib/map/ranking';

  let {
    content,
    filter,
    focus,
    focusLabel,
    results,
    launching,
    findable,
    connection,
    onconnection,
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
    results: Record<ResultState, number> | null;
    launching: boolean;
    findable: Findable[];
    connection: ConnectionType;
    onconnection: (connection: ConnectionType) => void;
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
  {#if pills.length > 0 || focusLabel}
    <SelectionRow {pills} {focusLabel} {results} rearranged={Boolean(filter.model || filter.category)} {onremove} {onclearfocus} {onclearall} />
  {/if}
  <div class="main">
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

    <label class="connection">
      Connexion
      <select value={connection} onchange={(event) => onconnection(event.currentTarget.value as ConnectionType)}>
        {#each CONNECTIONS as option (option.value)}
          <option value={option.value}>{option.label}</option>
        {/each}
      </select>
    </label>

    {#if pills.length === 0 && !focusLabel}
      <span class="hint">Cliquez sur un élément pour filtrer</span>
    {/if}

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
    {#if !launching}
      <button type="button" class="btn btn-primary launch" disabled={live.campaign !== ''} onclick={onlaunch}>
        <Icon name="play" size={16} /> Lancer un Benchmark
      </button>
    {/if}
  </div>
</footer>

<style>
  .status {
    border-top: 1px solid var(--hairline);
    background: var(--panel);
    color: var(--ink-3);
    font-size: 0.75rem;
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
  }

  .main {
    display: flex;
    align-items: center;
    gap: var(--s3);
    min-height: 44px;
    padding: var(--s1) var(--s4);
  }

  ul {
    display: flex;
    flex: none;
    gap: var(--s1);
    margin: 0;
    padding: 0;
    list-style: none;
  }

  li {
    position: relative;
    display: inline-flex;
    align-items: center;
  }

  .counter {
    display: inline-flex;
    align-items: center;
    gap: 3px;
    height: 30px;
    padding: 0 var(--s2);
    border: 1px solid var(--axis);
    border-radius: var(--radius-sm);
    background: var(--raised);
    color: var(--ink-2);
    font: inherit;
    cursor: pointer;
    transition: border-color 0.15s;
  }

  .counter:hover,
  .counter[aria-expanded='true'] {
    border-color: var(--accent);
    color: var(--ink);
  }

  .connection {
    display: inline-flex;
    flex: none;
    align-items: center;
    gap: var(--s1);
  }

  .connection select {
    height: 30px;
    padding: 0 var(--s2);
    border: 1px solid var(--axis);
    border-radius: var(--radius-sm);
    background: var(--raised);
    color: var(--ink);
    font: inherit;
    cursor: pointer;
  }

  .connection select:hover,
  .connection select:focus-visible {
    border-color: var(--accent);
  }

  .hint {
    min-width: 0;
    overflow: hidden;
    color: var(--ink-3);
    text-overflow: ellipsis;
  }

  strong {
    color: var(--ink);
    font-size: var(--text-sm);
    font-weight: 650;
  }

  .tool {
    flex: none;
    border: 0;
    background: transparent;
    color: var(--ink-2);
    font-size: 0.75rem;
    cursor: pointer;
  }

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
    margin-inline-start: auto;
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
      width: 9rem;
    }
  }

  @media (max-width: 760px) {
    .main {
      flex-wrap: wrap;
      white-space: normal;
    }

    ul {
      flex-wrap: wrap;
    }
  }
</style>
