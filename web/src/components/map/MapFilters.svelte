<script lang="ts">
  import Icon from '../Icon.svelte';
  import { categoryLabel } from '../../lib/format';
  import { live } from '../../lib/live.svelte';
  import type { MapFilter, MapLists } from '../../lib/map/ranking';

  let {
    filter = $bindable(),
    lists,
    launching,
    onlaunch,
  }: { filter: MapFilter; lists: MapLists; launching: boolean; onlaunch: () => void } = $props();

  const active = $derived(Boolean(filter.machine || filter.model || filter.category));
</script>

<form class="filters glass" aria-label="Filtres de la carte" onsubmit={(event) => event.preventDefault()}>
  <label class="field">
    Machine
    <select class="input" bind:value={filter.machine}>
      <option value="">Toutes</option>
      {#each lists.machines as machine (machine)}
        <option value={machine}>{machine}</option>
      {/each}
    </select>
  </label>
  <label class="field">
    Modèle
    <select class="input" bind:value={filter.model}>
      <option value="">Tous</option>
      {#each lists.models as model (model)}
        <option value={model}>{model}</option>
      {/each}
    </select>
  </label>
  <label class="field">
    Thème
    <select class="input" bind:value={filter.category}>
      <option value="">Tous</option>
      {#each lists.themes as theme (theme)}
        <option value={theme}>{categoryLabel(theme)}</option>
      {/each}
    </select>
  </label>
  <button
    class="btn"
    type="button"
    disabled={!active}
    onclick={() => {
      filter = { machine: '', model: '', category: '' };
    }}
  >
    <Icon name="cross" /> Tout effacer
  </button>
  <button class="btn btn-primary launch" type="button" aria-expanded={launching} disabled={live.campaign !== ''} onclick={onlaunch}>
    <Icon name="play" /> Lancer un Benchmark
  </button>
</form>

<style>
  .filters {
    display: flex;
    flex-wrap: wrap;
    align-items: flex-end;
    gap: var(--s2) var(--s3);
    padding: var(--s3);
    border-radius: var(--radius-lg);
    pointer-events: auto;
  }

  .field {
    flex: 1 1 8.5rem;
    min-width: 0;
  }

  .btn {
    white-space: nowrap;
  }

  .launch {
    margin-inline-start: auto;
  }
</style>
