<script lang="ts">
  import { onMount } from 'svelte';

  import ConnectionsPanel from '../components/ConnectionsPanel.svelte';
  import CorpusPanel from '../components/CorpusPanel.svelte';
  import Icon from '../components/Icon.svelte';
  import LibraryPanel from '../components/LibraryPanel.svelte';
  import Notice from '../components/Notice.svelte';
  import { api, errorMessage } from '../lib/api';
  import type { Hardware, Models, Settings } from '../lib/api';

  let hardware = $state<Hardware | null>(null);
  let models = $state<Models | null>(null);
  let settings = $state<Settings | null>(null);
  let error = $state('');

  /** Reloads connections, library and downloads */
  async function loadModels(): Promise<void> {
    try {
      models = await api.models();
    } catch (loadError) {
      error = errorMessage(loadError);
    }
  }

  onMount(async () => {
    try {
      const [status, loaded] = await Promise.all([api.status(), api.settings()]);
      hardware = status.hardware;
      settings = loaded;
    } catch (loadError) {
      error = errorMessage(loadError);
    }
    await loadModels();
  });
</script>

<div class="page">
  <div class="page-head">
    <h1>Réglages</h1>
  </div>

  <Notice message={error} />

  {#if hardware}
    <section class="section" aria-labelledby="machine">
      <h2 id="machine">Machine</h2>
      <div class="panel machine">
        <Icon name="cpu" size={24} />
        {#if hardware.detected}
          <p><strong>{hardware.label}</strong></p>
          <p>{hardware.cores} cœurs</p>
          {#if hardware.gpu}<p>{hardware.gpu}</p>{/if}
          <p class="muted">{hardware.os} {hardware.arch}</p>
        {:else}
          <p class="state state-warning"><Icon name="alert" size={16} /> Machine non détectée : lancer <code>make up</code></p>
        {/if}
      </div>
    </section>
  {/if}

  {#if models}
    <ConnectionsPanel connections={models.connections} onchange={loadModels} />
    <LibraryPanel {models} onchange={loadModels} />
  {/if}

  {#if settings}
    <CorpusPanel bind:settings />
  {/if}
</div>

<style>
  .machine {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--s2) var(--s5);
  }
</style>
