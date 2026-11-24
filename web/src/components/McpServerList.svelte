<script lang="ts">
  import { fade, slide } from 'svelte/transition';

  import Icon from './Icon.svelte';
  import type { McpProbe, McpServer } from '../lib/api';
  import { motion, stateOf } from '../lib/mcp';

  let {
    servers = $bindable(),
    probes,
    adding = $bindable(),
    name = $bindable(),
    url = $bindable(),
    token = $bindable(),
    saving,
    onselect,
    onadd,
    ontoggle,
    onremove,
  }: {
    servers: McpServer[] | null;
    probes: Record<number, McpProbe>;
    adding: boolean;
    name: string;
    url: string;
    token: string;
    saving: boolean;
    onselect: (id: number) => void;
    onadd: (event: SubmitEvent) => Promise<void>;
    ontoggle: (server: McpServer) => Promise<void>;
    onremove: (server: McpServer) => Promise<void>;
  } = $props();
</script>

<p class="muted intro">Les serveurs activés prêtent leurs outils aux modèles du Chat. Ce que renvoient les outils part chez les modèles cloud cochés.</p>
<section class="group">
  <h4 class="label">Serveurs</h4>
  {#if servers === null}
    <p class="muted">Chargement...</p>
  {:else}
    <ul class="servers">
      {#each servers as server (server.id)}
        {@const state = stateOf(probes[server.id])}
        <li class="server">
          <button class="row" type="button" onclick={() => onselect(server.id)}>
            <span class="tile"><Icon name="plug" size={20} /></span>
            <span class="text">
              <span class="name">{server.name}</span>
              <span class="meta muted">
                <span class="host">{new URL(server.url).host}</span> ·
                {#key state.label}<span class={state.tone} in:fade={motion}>{state.label}</span>{/key}
              </span>
            </span>
          </button>
          <label class="switch">
            <input type="checkbox" role="switch" bind:checked={server.enabled} onchange={() => ontoggle(server)} aria-label="Activer {server.name}" />
            <span class="switch-track" aria-hidden="true"></span>
          </label>
          <button class="ghost danger" type="button" onclick={() => onremove(server)} aria-label="Supprimer {server.name}"><Icon name="trash" size={18} /></button>
        </li>
      {:else}
        <li class="muted">Aucun serveur.</li>
      {/each}
    </ul>
  {/if}
</section>
{#if adding}
  <form class="card add" onsubmit={onadd} transition:slide={motion}>
    <h4 class="label">Nouveau serveur</h4>
    <label class="field">Nom<input class="input" bind:value={name} required pattern={'[a-z0-9_\\-]{1,32}'} title="Minuscules, chiffres, - et _" placeholder="sentinelle" /></label>
    <label class="field">URL<input class="input" type="url" bind:value={url} required placeholder="https://exemple.fr/mcp" /></label>
    <label class="field">Jeton (facultatif)<input class="input" type="password" bind:value={token} autocomplete="off" /></label>
    <p class="muted warning">Tous les outils du serveur sont transmis aux modèles, y compris ceux qui modifient des données : n'ajoutez qu'un serveur de confiance. Sans jeton, le serveur peut demander de se connecter depuis le navigateur.</p>
    <div class="add-actions">
      <button class="btn" type="button" onclick={() => (adding = false)}>Annuler</button>
      <button class="btn btn-primary" type="submit" disabled={saving}>{saving ? 'Ajout...' : 'Ajouter'}</button>
    </div>
  </form>
{:else}
  <button class="link" type="button" onclick={() => (adding = true)} in:fade={motion}><Icon name="plus" size={16} /> Ajouter un serveur</button>
{/if}

<style>
  .intro {
    margin: 0;
  }

  .servers {
    margin: 0 calc(-1 * var(--s3));
    padding: 0;
    list-style: none;
  }

  .server {
    position: relative;
    display: flex;
    align-items: center;
    gap: var(--s3);
    padding: var(--s3);
    border-radius: var(--radius);
    transition: background-color 0.15s;
  }

  .server:hover {
    background: color-mix(in srgb, var(--raised) 70%, transparent);
  }

  /* Hairline starts at the text, under the icon tile's right edge. */
  .server + .server::before {
    position: absolute;
    top: 0;
    right: var(--s3);
    left: calc(var(--s3) + 44px + var(--s4));
    border-top: 1px solid var(--hairline);
    content: '';
  }

  .row {
    display: flex;
    flex: 1;
    align-items: center;
    gap: var(--s4);
    min-width: 0;
    padding: 0;
    border: 0;
    border-radius: var(--radius-sm);
    background: none;
    color: inherit;
    font: inherit;
    text-align: start;
    cursor: pointer;
  }

  .text {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    flex: 1;
    gap: 2px;
    min-width: 0;
  }

  .name {
    font-size: var(--text-md);
    font-weight: 600;
  }

  .meta {
    display: flex;
    gap: 0.3em;
    font-size: var(--text-sm);
    white-space: nowrap;
  }

  .host {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .link {
    color: var(--accent);
  }

  .link:hover {
    text-decoration: underline;
    text-underline-offset: 3px;
  }

  .add {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    align-items: end;
    gap: var(--s3);
    padding: var(--s4);
  }

  .add .label,
  .warning,
  .add-actions {
    grid-column: 1 / -1;
  }

  .warning {
    margin: 0;
    font-size: var(--text-sm);
  }

  .add-actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--s2);
  }

  @media (max-width: 480px) {
    .add {
      grid-template-columns: minmax(0, 1fr);
    }
  }
</style>
