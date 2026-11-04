<script lang="ts">
  import { untrack } from 'svelte';

  import FitBadge from './FitBadge.svelte';
  import Icon from './Icon.svelte';
  import ModelName from './ModelName.svelte';
  import Notice from './Notice.svelte';
  import { api, errorMessage } from '../lib/api';
  import type { Models, Pull } from '../lib/api';
  import { gigabytes } from '../lib/format';
  import { live } from '../lib/live.svelte';

  let { models, onchange }: { models: Models; onchange: () => Promise<void> } = $props();

  let customTag = $state('');
  let error = $state('');

  const puller = $derived(models.connections.find((connection) => connection.online && connection.provider.kind === 'ollama'));
  // Ollama reports an untagged library name as name:latest.
  const installed = $derived(new Set(models.connections.flatMap((connection) => connection.models.map((model) => model.name.replace(/:latest$/, '')))));
  const pulls = $derived.by(() => {
    const merged: Record<string, Pull> = {};
    for (const pull of models.pulls) merged[pull.name] = pull;
    return { ...merged, ...live.pulls };
  });
  const active = $derived(Object.values(pulls).filter((pull) => !pull.done));
  const finishedCount = $derived(Object.values(live.pulls).filter((pull) => pull.done).length);

  // A finished download changes the installed list.
  $effect(() => {
    if (finishedCount > 0) void untrack(onchange);
  });

  /**
   * @param {string} name - model tag
   */
  async function pull(name: string): Promise<void> {
    if (!puller) return;
    try {
      await api.pullModel(puller.provider.id, name);
      error = '';
    } catch (pullError) {
      error = errorMessage(pullError);
    }
  }
</script>

<section class="section" aria-labelledby="library">
  <h2 id="library">Bibliothèque de modèles</h2>
  <Notice message={error} />

  {#each active as download (download.name)}
    <div class="download">
      <p><ModelName name={download.name} /> <span class="muted">{download.status}</span></p>
      <div class="progress" role="progressbar" aria-label="Téléchargement de {download.name}" aria-valuemin="0" aria-valuemax={download.total} aria-valuenow={download.completed}>
        <span style:width="{download.total ? (100 * download.completed) / download.total : 0}%"></span>
      </div>
    </div>
  {/each}
  {#each Object.values(pulls).filter((download) => download.error) as failed (failed.name)}
    <Notice message="{failed.name} : {failed.error}" />
  {/each}

  <div class="table-wrap">
    <table>
      <thead>
        <tr>
          <th scope="col">Modèle</th>
          <th scope="col" class="num">Taille</th>
          <th scope="col">Sur cette machine</th>
          <th scope="col"><span class="visually-hidden">Action</span></th>
        </tr>
      </thead>
      <tbody>
        {#each models.library as entry (entry.name)}
          <tr>
            <td class="model-cell"><ModelName name={entry.name} />{#if entry.role === 'embedding'}<span class="muted">embeddings</span>{/if}</td>
            <td class="num">{gigabytes(entry.size_bytes)}</td>
            <td><FitBadge fit={entry.fit} /></td>
            <td class="num">
              {#if installed.has(entry.name)}
                <span class="state state-good"><Icon name="check" size={16} /> Installé</span>
              {:else}
                <button class="btn" type="button" disabled={!puller || pulls[entry.name]?.done === false} onclick={() => pull(entry.name)}>
                  <Icon name="download" /> Télécharger
                </button>
              {/if}
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
  </div>

  <form
    class="custom"
    onsubmit={(event) => {
      event.preventDefault();
      void pull(customTag.trim());
      customTag = '';
    }}
  >
    <label class="field">Autre modèle<input class="input" bind:value={customTag} required placeholder="nom:tag" /></label>
    <button class="btn" type="submit" disabled={!puller}><Icon name="download" /> Télécharger</button>
  </form>
</section>

<style>
  .download {
    display: grid;
    gap: var(--s1);
  }

  .custom {
    display: flex;
    flex-wrap: wrap;
    align-items: flex-end;
    gap: var(--s3);
  }

  .custom .field {
    flex: 1 1 14rem;
  }

  .visually-hidden {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip-path: inset(50%);
    white-space: nowrap;
  }

  td {
    white-space: nowrap;
  }

  .model-cell .muted {
    margin-left: var(--s2);
    font-size: var(--text-sm);
  }
</style>
