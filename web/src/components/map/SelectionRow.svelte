<script lang="ts">
  import Icon from '../Icon.svelte';
  import ResultMark from './ResultMark.svelte';
  import type { MapFilter } from '../../lib/map/ranking';
  import { RESULT_LABELS } from '../../lib/map/results';
  import type { ResultState } from '../../lib/map/results';

  let {
    pills,
    focusLabel,
    results,
    rearranged,
    onremove,
    onclearfocus,
    onclearall,
  }: {
    pills: { axis: keyof MapFilter; label: string }[];
    focusLabel: string;
    results: Record<ResultState, number> | null;
    rearranged: boolean;
    onremove: (axis: keyof MapFilter) => void;
    onclearfocus: () => void;
    onclearall: () => void;
  } = $props();
</script>

<div class="selection" aria-label="Sélection active">
  {#each pills as pill (pill.axis)}
    <button type="button" class="pill" title={pill.label} aria-label="Retirer {pill.label}" onclick={() => onremove(pill.axis)}><span>{pill.label}</span> <Icon name="cross" size={13} /></button>
  {/each}
  {#if focusLabel}
    <button type="button" class="pill" title={focusLabel} aria-label="Retirer {focusLabel}" onclick={onclearfocus}><span>{focusLabel}</span> <Icon name="cross" size={13} /></button>
  {/if}
  <button type="button" class="clear" onclick={onclearall}>Tout effacer</button>
  {#if results}
    <ul aria-label="Résultats du modèle par question">
      {#each Object.entries(RESULT_LABELS) as [state, label] (state)}
        <li><ResultMark state={state as ResultState} /> <strong>{results[state as ResultState]}</strong> {label}</li>
      {/each}
    </ul>
  {/if}
  {#if rearranged}<span class="distance">Plus près = meilleur score</span>{/if}
</div>

<style>
  /* Its own row, shown with a selection only: whatever it holds wraps rather than crushing the bar below. */
  .selection {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--s1) var(--s2);
    padding: var(--s1) var(--s4);
    border-bottom: 1px solid var(--hairline);
  }

  .pill {
    display: inline-flex;
    align-items: center;
    gap: var(--s1);
    max-width: 22rem;
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

  .clear {
    border: 0;
    background: transparent;
    color: var(--ink-2);
    font-size: 0.75rem;
    text-decoration: underline;
    text-underline-offset: 2px;
    cursor: pointer;
  }

  .clear:hover {
    color: var(--ink);
  }

  ul {
    display: flex;
    flex-wrap: wrap;
    margin: 0 0 0 var(--s2);
    padding: 0 0 0 var(--s2);
    border-inline-start: 1px solid var(--axis);
    color: var(--ink-2);
    list-style: none;
  }

  li {
    display: inline-flex;
    align-items: center;
    gap: var(--s1);
    padding-inline: var(--s1);
  }

  .distance {
    margin-inline-start: auto;
    color: var(--ink-2);
  }

  strong {
    color: var(--ink);
    font-size: var(--text-sm);
    font-weight: 650;
  }
</style>
