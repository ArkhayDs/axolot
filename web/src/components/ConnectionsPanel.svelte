<script lang="ts">
  import ClaudeBridgeDialog from './ClaudeBridgeDialog.svelte';
  import Icon from './Icon.svelte';
  import Notice from './Notice.svelte';
  import { api, errorMessage } from '../lib/api';
  import type { Connection } from '../lib/api';

  let { connections, onchange }: { connections: Connection[]; onchange: () => Promise<void> } = $props();

  let name = $state('');
  let kind = $state('ollama');
  let baseUrl = $state('');
  let apiKey = $state('');
  let error = $state('');
  let guideOpen = $state(false);

  /** Saves the connection and clears the form, the key never stays in the page */
  async function add(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    try {
      await api.createProvider({ name, kind, base_url: baseUrl, api_key: apiKey });
      name = baseUrl = apiKey = '';
      error = '';
      await onchange();
    } catch (saveError) {
      error = errorMessage(saveError);
    }
  }

  /**
   * Deletes a connection after confirmation: its selections go with it, past results stay
   * @param {Connection} connection - connection to remove
   */
  async function remove(connection: Connection): Promise<void> {
    if (!confirm(`Supprimer la connexion ${connection.provider.name} et ses sélections de modèles ?`)) return;
    try {
      await api.deleteProvider(connection.provider.id);
      error = '';
      await onchange();
    } catch (deleteError) {
      error = errorMessage(deleteError);
    }
  }
</script>

<section class="section" aria-labelledby="connections">
  <h2 id="connections">Connexions</h2>
  <p class="muted">Une connexion vers un service en ligne, pont Claude compris, reçoit les questions, les fichiers joints et les extraits du corpus. Seul le corpus fictif part sans réserve.</p>
  <button class="btn guide" type="button" onclick={() => (guideOpen = true)}><Icon name="plug" /> Connecter un compte Claude Pro ou Max</button>
  <ClaudeBridgeDialog open={guideOpen} added={connections.some((connection) => connection.bridge)} onclose={() => (guideOpen = false)} onadded={onchange} />
  <Notice message={error} />
  <div class="table-wrap">
    <table>
      <tbody>
        {#each connections as connection (connection.provider.id)}
          <tr>
            <td><strong>{connection.provider.name}</strong><br /><span class="muted">{connection.provider.base_url}</span></td>
            <td>
              {#if connection.online}
                <span class="state state-good"><Icon name="check" size={16} /> En ligne, {connection.models.length} modèles</span>
              {:else}
                <span class="state state-critical"><Icon name="cross" size={16} /> Hors ligne{#if connection.bridge}, lancer <code>make bridge</code>{/if}</span>
              {/if}
            </td>
            <td class="num">
              <button class="btn" type="button" onclick={() => remove(connection)} aria-label="Supprimer {connection.provider.name}"><Icon name="trash" /></button>
            </td>
          </tr>
        {:else}
          <tr><td class="muted">Aucune connexion.</td></tr>
        {/each}
      </tbody>
    </table>
  </div>
  <form class="add" onsubmit={add}>
    <label class="field">Nom<input class="input" bind:value={name} required /></label>
    <label class="field">
      Type
      <select class="input" bind:value={kind}>
        <option value="ollama">Ollama</option>
        <option value="openai">Compatible OpenAI</option>
      </select>
    </label>
    <label class="field">URL<input class="input" type="url" bind:value={baseUrl} required placeholder="http://host.docker.internal:11434" /></label>
    <label class="field">Clé API<input class="input" type="password" bind:value={apiKey} autocomplete="off" /></label>
    <button class="btn" type="submit"><Icon name="plug" /> Ajouter</button>
  </form>
</section>

<style>
  .guide {
    justify-self: start;
  }

  .add {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(10rem, 1fr));
    align-items: end;
    gap: var(--s3);
  }
</style>
