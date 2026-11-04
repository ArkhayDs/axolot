<script lang="ts">
  import { onMount } from 'svelte';

  import Icon from './Icon.svelte';
  import Notice from './Notice.svelte';
  import { api, errorMessage } from '../lib/api';
  import type { CorpusResponse, Settings } from '../lib/api';
  import { number } from '../lib/format';
  import { live } from '../lib/live.svelte';

  let { settings = $bindable() }: { settings: Settings } = $props();

  let corpus = $state<CorpusResponse | null>(null);
  let newFolder = $state('');
  let error = $state('');
  let saved = $state(false);

  const folder = $derived(corpus?.folders.find((item) => item.name === settings.corpus_dir));
  const index = $derived(corpus?.indexes.find((item) => item.corpus === settings.corpus_dir && item.embed_model === settings.embed_model));
  const ingesting = $derived(live.ingest ? !live.ingest.finished : (corpus?.ingesting ?? false));
  const stale = $derived(Boolean(index && folder && index.files !== folder.files.length));

  /** Reloads folders and indexes */
  async function load(): Promise<void> {
    try {
      corpus = await api.corpus();
    } catch (loadError) {
      error = errorMessage(loadError);
    }
  }

  /**
   * Runs an action, then shows its failure or the fresh state
   * @param {() => Promise<void>} action - API call
   */
  async function run(action: () => Promise<void>): Promise<void> {
    try {
      await action();
      error = '';
      await load();
    } catch (actionError) {
      error = errorMessage(actionError);
    }
  }

  /** Saves the settings and briefly confirms it */
  function save(event: SubmitEvent): void {
    event.preventDefault();
    saved = false;
    void run(async () => {
      settings = await api.saveSettings(settings);
      saved = true;
    });
  }

  /**
   * Sends the chosen documents, then clears the input so the same file can be picked again
   * @param {Event & { currentTarget: HTMLInputElement }} event - input change
   */
  function upload(event: Event & { currentTarget: HTMLInputElement }): void {
    const input = event.currentTarget;
    const files = [...(input.files ?? [])];
    void run(async () => {
      for (const file of files) await api.uploadCorpusFile(settings.corpus_dir, file);
      input.value = '';
    });
  }

  onMount(load);

  $effect(() => {
    if (live.ingest?.finished) void load();
  });
</script>

<section class="section" aria-labelledby="corpus">
  <h2 id="corpus">Corpus et paramètres RAG</h2>
  <Notice message={error || live.ingest?.error || ''} />

  <form class="params" onsubmit={save}>
    <label class="field">
      Dossier
      <select class="input" bind:value={settings.corpus_dir}>
        {#each corpus?.folders ?? [] as item (item.name)}
          <option value={item.name}>{item.name}</option>
        {/each}
      </select>
    </label>
    <label class="field">Taille de découpe<input class="input" type="number" min="100" max="8000" bind:value={settings.chunk_size} /></label>
    <label class="field">Chevauchement<input class="input" type="number" min="0" bind:value={settings.chunk_overlap} /></label>
    <label class="field">Extraits retenus<input class="input" type="number" min="1" max="20" bind:value={settings.top_k} /></label>
    <label class="field">Modèle d'embeddings<input class="input" bind:value={settings.embed_model} required /></label>
    <label class="field">RAM réservée (Go)<input class="input" type="number" min="0" max="256" bind:value={settings.reserved_gb} /></label>
    <div class="save">
      <button class="btn" type="submit">Enregistrer</button>
      {#if saved}<span class="state state-good" role="status"><Icon name="check" size={16} /> Enregistré</span>{/if}
    </div>
  </form>

  <div class="panel index">
    <div class="index-state">
      {#if ingesting}
        <p>Indexation en cours{live.ingest ? ` : ${number(live.ingest.done)} / ${number(live.ingest.total)} extraits` : ''}</p>
        <div class="progress" role="progressbar" aria-label="Indexation" aria-valuemin="0" aria-valuemax={live.ingest?.total ?? 0} aria-valuenow={live.ingest?.done ?? 0}>
          <span style:width="{live.ingest?.total ? (100 * live.ingest.done) / live.ingest.total : 0}%"></span>
        </div>
      {:else if index}
        <p class="state state-good"><Icon name="check" size={16} /> Indexé : {index.chunks} extraits, {index.files} fichiers</p>
        {#if stale}<p class="state state-warning"><Icon name="alert" size={16} /> {folder?.files.length} fichiers dans le dossier : à réindexer</p>{/if}
      {:else}
        <p class="state state-warning"><Icon name="alert" size={16} /> Non indexé avec {settings.embed_model}</p>
      {/if}
    </div>
    <button class="btn btn-primary" type="button" disabled={ingesting} onclick={() => run(api.ingestCorpus)}><Icon name="refresh" /> Indexer</button>
  </div>

  {#if folder}
    <ul class="files">
      {#each folder.files as file (file)}
        <li><Icon name="doc" size={16} /> {file}</li>
      {:else}
        <li class="muted">Dossier vide.</li>
      {/each}
    </ul>
  {/if}

  <div class="file-actions">
    <label class="btn upload">
      <Icon name="upload" /> Envoyer des fichiers (.md, .txt)
      <input type="file" accept=".md,.txt" multiple onchange={upload} />
    </label>
    <form
      class="new-folder"
      onsubmit={(event) => {
        event.preventDefault();
        void run(async () => {
          await api.createCorpus(newFolder);
          newFolder = '';
        });
      }}
    >
      <label class="field">Nouveau dossier<input class="input" bind:value={newFolder} required pattern="[a-zA-Z0-9][a-zA-Z0-9_\-]*" /></label>
      <button class="btn" type="submit">Créer</button>
    </form>
  </div>
</section>

<style>
  .params {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(10rem, 1fr));
    align-items: end;
    gap: var(--s3);
  }

  .save {
    display: flex;
    align-items: center;
    gap: var(--s3);
  }

  .index {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: var(--s3);
  }

  .index-state {
    display: grid;
    flex: 1 1 16rem;
    gap: var(--s2);
  }

  .files {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(16rem, 1fr));
    gap: var(--s1) var(--s4);
    margin: 0;
    padding: 0;
    color: var(--ink-2);
    font-size: var(--text-sm);
    list-style: none;
  }

  .files li {
    display: flex;
    align-items: center;
    gap: var(--s2);
    overflow-wrap: anywhere;
  }

  .file-actions {
    display: flex;
    flex-wrap: wrap;
    align-items: flex-end;
    justify-content: space-between;
    gap: var(--s4);
  }

  .upload input {
    position: absolute;
    width: 1px;
    height: 1px;
    opacity: 0;
  }

  .upload:focus-within {
    outline: 2px solid var(--accent);
    outline-offset: 2px;
  }

  .new-folder {
    display: flex;
    align-items: flex-end;
    gap: var(--s2);
  }
</style>
