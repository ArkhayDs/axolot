<script lang="ts">
  import Icon from './Icon.svelte';
  import Notice from './Notice.svelte';
  import { api, errorMessage } from '../lib/api';
  import type { McpServer } from '../lib/api';

  let { open, onclose }: { open: boolean; onclose: () => void } = $props();

  let dialog: HTMLDialogElement;
  let servers = $state<McpServer[] | null>(null);
  let name = $state('');
  let url = $state('');
  let token = $state('');
  let error = $state('');

  /** Lists the servers, each asked for its tools: an unreachable one waits out its timeout */
  async function load(): Promise<void> {
    try {
      servers = await api.mcpServers();
    } catch (loadError) {
      error = errorMessage(loadError);
    }
  }

  /** Saves the server and clears the form, the token never stays in the page */
  async function add(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    try {
      await api.createMcpServer({ name, url, token });
      name = url = token = '';
      error = '';
      await load();
    } catch (saveError) {
      error = errorMessage(saveError);
    }
  }

  /**
   * Saves the checkbox, unchecked again when the server refuses
   * @param {McpServer} server - row toggled, already holding the new value
   */
  async function toggle(server: McpServer): Promise<void> {
    try {
      await api.setMcpServerEnabled(server.id, server.enabled);
      error = '';
    } catch (saveError) {
      server.enabled = !server.enabled;
      error = errorMessage(saveError);
    }
  }

  /**
   * Deletes a server after confirmation
   * @param {McpServer} server - server to remove
   */
  async function remove(server: McpServer): Promise<void> {
    if (!confirm(`Supprimer le serveur MCP ${server.name} ?`)) return;
    try {
      await api.deleteMcpServer(server.id);
      error = '';
      await load();
    } catch (deleteError) {
      error = errorMessage(deleteError);
    }
  }

  $effect(() => {
    if (open) {
      error = '';
      servers = null;
      dialog.showModal();
      load();
    } else if (dialog.open) dialog.close();
  });
</script>

<dialog class="sheet glass" bind:this={dialog} onclose={onclose} aria-labelledby="mcp-title">
  <div class="sheet-head">
    <h2 id="mcp-title">Serveurs MCP</h2>
    <button class="btn" type="button" onclick={onclose} aria-label="Fermer"><Icon name="cross" /></button>
  </div>
  <p class="muted">Les serveurs cochés prêtent leurs outils aux modèles du Chat. Ce que renvoient les outils part chez les modèles cloud cochés.</p>
  <Notice message={error} />
  {#if servers === null}
    <p class="muted">Connexion aux serveurs...</p>
  {:else}
    <ul class="servers">
      {#each servers as server (server.id)}
        <li class="server">
          <label class="pick">
            <input type="checkbox" bind:checked={server.enabled} onchange={() => toggle(server)} />
            <span><strong>{server.name}</strong><br /><span class="muted">{server.url}</span></span>
          </label>
          <button class="btn" type="button" onclick={() => remove(server)} aria-label="Supprimer {server.name}"><Icon name="trash" /></button>
          {#if server.error}
            <p class="state state-critical detail"><Icon name="cross" size={16} /> Injoignable</p>
            <p class="muted detail">{server.error}</p>
          {:else if server.tools.length === 0}
            <p class="muted detail">Aucun outil</p>
          {:else}
            <p class="muted detail">{server.tools.length} outil{server.tools.length > 1 ? 's' : ''} : {server.tools.join(', ')}</p>
          {/if}
        </li>
      {:else}
        <li class="muted">Aucun serveur.</li>
      {/each}
    </ul>
  {/if}
  <form class="add" onsubmit={add}>
    <label class="field">Nom<input class="input" bind:value={name} required pattern={'[a-z0-9_\\-]{1,32}'} title="Minuscules, chiffres, - et _" placeholder="sentinelle" /></label>
    <label class="field">URL<input class="input" type="url" bind:value={url} required placeholder="https://exemple.fr/mcp" /></label>
    <label class="field">Jeton (facultatif)<input class="input" type="password" bind:value={token} autocomplete="off" /></label>
    <button class="btn" type="submit"><Icon name="plug" /> Ajouter</button>
  </form>
  <p class="warning">Tous les outils du serveur sont transmis aux modèles, y compris ceux qui modifient des données : n'ajoutez qu'un serveur de confiance.</p>
</dialog>

<style>
  .sheet {
    width: min(640px, calc(100vw - 2 * var(--s4)));
    margin: auto;
  }

  .sheet[open] {
    display: grid;
    gap: var(--s4);
    overflow: auto;
  }

  .servers {
    display: grid;
    gap: var(--s3);
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .server {
    display: grid;
    grid-template-columns: 1fr auto;
    align-items: start;
    gap: var(--s2) var(--s3);
    padding: var(--s3);
    border: 1px solid var(--hairline);
    border-radius: var(--radius);
  }

  .pick {
    display: flex;
    align-items: flex-start;
    gap: var(--s2);
    min-width: 0;
    overflow-wrap: anywhere;
    cursor: pointer;
  }

  .pick input {
    margin-top: 0.3em;
  }

  .detail {
    grid-column: 1 / -1;
    font-size: var(--text-sm);
    overflow-wrap: anywhere;
  }

  .add {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(10rem, 1fr));
    align-items: end;
    gap: var(--s3);
  }

  .warning {
    color: var(--ink-3);
    font-size: var(--text-sm);
  }
</style>
