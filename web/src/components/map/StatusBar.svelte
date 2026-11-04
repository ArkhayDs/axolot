<script lang="ts">
  import Icon from '../Icon.svelte';
  import { categoryLabel } from '../../lib/format';
  import { live } from '../../lib/live.svelte';
  import type { MapContent } from '../../lib/map/content';
  import type { MapFilter } from '../../lib/map/ranking';
  import { shortModel } from '../../lib/map/ranking';

  export interface Findable {
    kind: 'machine' | 'model' | 'theme' | 'scenario' | 'question' | 'document' | 'benchmark';
    key: string;
    label: string;
    type: string;
  }

  let {
    content,
    filter,
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

  const counters = $derived([
    { value: content.machines.length, unit: 'machine' },
    { value: content.models.length, unit: 'modèle' },
    { value: content.themes.length, unit: 'thème' },
    { value: content.scenarios.length, unit: 'scénario' },
    { value: content.questions.length, unit: 'question' },
    { value: content.documents.size, unit: 'document' },
    { value: content.benchmarks.length, unit: 'benchmark' },
  ]);
  const pills = $derived(
    [
      { axis: 'machine' as const, label: filter.machine },
      { axis: 'model' as const, label: filter.model ? shortModel(filter.model, content.models) : '' },
      { axis: 'category' as const, label: filter.category ? categoryLabel(filter.category) : '' },
    ].filter((pill) => pill.label),
  );

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
    {#each counters as counter (counter.unit)}
      <li><strong>{counter.value}</strong> {counter.unit}{counter.value > 1 ? 's' : ''}</li>
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
    display: inline-flex;
    align-items: center;
    gap: 3px;
    padding-inline: var(--s2);
  }

  li + li {
    border-inline-start: 1px solid var(--hairline);
  }

  li:first-child {
    padding-inline-start: 0;
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
