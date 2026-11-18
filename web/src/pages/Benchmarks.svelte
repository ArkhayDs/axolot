<script lang="ts">
  import { onMount } from 'svelte';

  import Notice from '../components/Notice.svelte';
  import ModelName from '../components/ModelName.svelte';
  import { api, errorMessage } from '../lib/api';
  import type { MapBenchmark } from '../lib/api';
  import { benchmarkName, duration, score, statusLabel, when } from '../lib/format';
  import { live } from '../lib/live.svelte';
  import { benchmarkScore } from '../lib/map/ranking';

  let benchmarks = $state<MapBenchmark[]>([]);
  let loaded = $state(false);
  let error = $state('');

  const played = $derived(benchmarks.filter((benchmark) => benchmark.done_items > 0));

  /** Reloads the history */
  async function load(): Promise<void> {
    try {
      benchmarks = (await api.map('')).benchmarks;
      error = '';
    } catch (loadError) {
      error = errorMessage(loadError);
    } finally {
      loaded = true;
    }
  }

  onMount(load);

  let seenFinished = live.finished;
  $effect(() => {
    if (live.finished === seenFinished) return;
    seenFinished = live.finished;
    void load();
  });
</script>

<div class="page">
  <div class="page-head">
    <div>
      <h1>Benchmarks</h1>
      <p>L'historique des exécutions. Chacune alimente la base cumulée.</p>
    </div>
  </div>

  <Notice message={error} />

  {#if played.length > 0}
    <div class="table-wrap">
      <table>
        <thead>
          <tr>
            <th scope="col">Nom</th>
            <th scope="col">Date</th>
            <th scope="col" class="num">Durée</th>
            <th scope="col">Machine</th>
            <th scope="col">Modèles</th>
            <th scope="col" class="num">Réponses</th>
            <th scope="col" class="num">Score moyen</th>
            <th scope="col">Statut</th>
          </tr>
        </thead>
        <tbody>
          {#each played as benchmark (benchmark.id)}
            {@const average = benchmarkScore(benchmark, { machine: '', model: '', category: '' })}
            <tr
              onclick={() => {
                location.hash = `#/benchmarks/${benchmark.id}`;
              }}
            >
              <td><a href="#/benchmarks/{benchmark.id}">{benchmarkName(benchmark.name)}</a></td>
              <td>{when(benchmark.created_at)}</td>
              <td class="num">{benchmark.finished_at ? duration(new Date(benchmark.finished_at).getTime() - new Date(benchmark.created_at).getTime()) : '-'}</td>
              <td>{benchmark.machine}</td>
              <td>
                <span class="models">
                  {#each benchmark.models as model (model)}
                    <ModelName name={model} short />
                  {/each}
                </span>
              </td>
              <td class="num">{benchmark.done_items}</td>
              <td class="num">{average === null ? '-' : score(average)}</td>
              <td>{statusLabel(benchmark.status)}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {:else if loaded && !error}
    <div class="empty">
      <h2>Aucun Benchmark pour l'instant</h2>
      <a class="btn btn-primary" href="#/">Lancer un Benchmark depuis l'Atlas</a>
    </div>
  {/if}
</div>

<style>
  tbody tr {
    cursor: pointer;
    transition: background-color 0.15s;
  }

  tbody tr:hover {
    background: var(--raised);
  }

  td,
  th {
    white-space: nowrap;
  }

  tbody a {
    text-decoration: none;
  }

  tbody tr:hover a {
    color: var(--accent);
  }

  .models {
    display: flex;
    flex-wrap: wrap;
    gap: 0 var(--s3);
  }
</style>
