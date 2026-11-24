<script lang="ts">
  import { cubicOut } from 'svelte/easing';
  import { fade, fly, slide } from 'svelte/transition';

  import Icon from './Icon.svelte';
  import Notice from './Notice.svelte';
  import { api, errorMessage } from '../lib/api';
  import type { McpProbe, McpServer } from '../lib/api';

  type IconName = 'check' | 'alert' | 'cross';

  let { open, onclose }: { open: boolean; onclose: () => void } = $props();

  // Svelte transitions run in JS, out of reach of the global reduced-motion CSS rule.
  const motion = { duration: window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 0 : 180, easing: cubicOut };

  let dialog: HTMLDialogElement;
  let servers = $state<McpServer[] | null>(null);
  let probes = $state<Record<number, McpProbe>>({});
  let selected = $state<number | null>(null);
  let adding = $state(false);
  let name = $state('');
  let url = $state('');
  let token = $state('');
  let error = $state('');
  let saving = $state(false);
  let lastLoad = 0;

  let detail = $derived(servers?.find((server) => server.id === selected));

  /**
   * Connection state shown beside the server
   * @param {McpProbe | undefined} probe - server's answer, undefined while it is asked
   * @returns {{ label: string; tone: string; icon?: IconName }} - label, its class and icon
   */
  function stateOf(probe: McpProbe | undefined): { label: string; tone: string; icon?: IconName } {
    if (!probe) return { label: 'Vérification...', tone: 'muted' };
    if (probe.login) return { label: 'Connexion requise', tone: 'state-warning', icon: 'alert' };
    if (probe.error) return { label: 'Injoignable', tone: 'state-critical', icon: 'cross' };
    return { label: 'Connecté', tone: 'state-good', icon: 'check' };
  }

  /** Lists the servers at once, then asks each one for its tools; only the latest call writes, a slower earlier one is dropped */
  async function load(): Promise<void> {
    const current = ++lastLoad;
    try {
      const list = await api.mcpServers();
      if (current !== lastLoad) return;
      servers = list;
      for (const server of list) {
        api.probeMcpServer(server.id).then(
          (probe) => {
            if (current === lastLoad) probes[server.id] = probe;
          },
          (probeError) => {
            if (current === lastLoad) probes[server.id] = { tools: [], error: errorMessage(probeError), login: false };
          },
        );
      }
    } catch (loadError) {
      if (current === lastLoad) error = errorMessage(loadError);
    }
  }

  /** Saves the server and folds the form, the token never stays in the page */
  async function add(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    saving = true;
    try {
      await api.createMcpServer({ name, url, token });
      name = url = token = '';
      error = '';
      adding = false;
      await load();
    } catch (saveError) {
      error = errorMessage(saveError);
    } finally {
      saving = false;
    }
  }

  /**
   * Saves the switch, turned back when the server refuses
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

  /**
   * Opens the server's sign-in page in a new tab; once signed in, that tab tells this panel to reload
   * @param {McpServer} server - server asking for a sign-in
   */
  async function signIn(server: McpServer): Promise<void> {
    // Opened before the request: browsers block a tab opened after an await.
    const tab = window.open('', '_blank');
    if (!tab) {
      error = 'Le navigateur a bloqué le nouvel onglet : autorisez les fenêtres pour cette page.';
      return;
    }
    tab.opener = null;
    try {
      const { url: pageURL } = await api.loginMcpServer(server.id);
      tab.location.href = pageURL;
      error = '';
    } catch (loginError) {
      tab.close();
      error = errorMessage(loginError);
    }
  }

  $effect(() => {
    if (!open) return;
    const channel = new BroadcastChannel('axolot-mcp');
    channel.onmessage = () => load();
    return () => channel.close();
  });

  $effect(() => {
    if (open) {
      error = '';
      selected = null;
      adding = false;
      dialog.showModal();
      load();
    } else if (dialog.open) dialog.close();
  });
</script>

<dialog class="sheet glass mcp" bind:this={dialog} onclose={onclose} aria-labelledby="mcp-title">
  <div class="head">
    <h2 id="mcp-title">Serveurs MCP</h2>
    <button class="ghost" type="button" onclick={onclose} aria-label="Fermer"><Icon name="cross" /></button>
  </div>
  <div class="body">
    <Notice message={error} />
    {#if detail}
      {@const probe = probes[detail.id]}
      {@const state = stateOf(probe)}
      {@const host = new URL(detail.url).host}
      <div class="view" in:fly={{ ...motion, x: 16 }}>
        <button class="back" type="button" onclick={() => (selected = null)}><Icon name="arrow" size={16} /> Serveurs MCP</button>
        <div class="detail-head">
          <span class="tile tile-lg"><Icon name="plug" size={26} /></span>
          <div class="title">
            <h3>{detail.name}</h3>
            <p class="muted">Serveur MCP · {host}</p>
          </div>
          {#if probe?.login}
            <button class="btn btn-primary" type="button" onclick={() => detail && signIn(detail)} in:fade={motion}><Icon name="plug" /> Se connecter</button>
          {:else}
            {#key state.label}
              <span class="pill {state.tone}" in:fade={motion}>{#if state.icon}<Icon name={state.icon} size={14} />{/if} {state.label}</span>
            {/key}
          {/if}
        </div>
        <section class="group">
          <h4 class="label">Connexion</h4>
          <dl class="card">
            <div class="fact">
              <dt>URL</dt>
              <dd>{detail.url}</dd>
            </div>
            <div class="fact">
              <dt>État</dt>
              <dd>
                <span class={state.tone}>{state.label}</span>
                {#if probe?.error && !probe.login}<span class="muted error">{probe.error}</span>{/if}
              </dd>
            </div>
            <div class="fact">
              <dt>Outils</dt>
              <dd>
                {#if probe && probe.tools.length > 0}
                  {probe.tools.join(', ')}
                {:else if probe}
                  <span class="muted">{probe.login ? 'Visibles une fois connecté' : 'Aucun outil'}</span>
                {:else}
                  <span class="muted">Vérification...</span>
                {/if}
              </dd>
            </div>
          </dl>
        </section>
        <div class="notes">
          <div>
            <h4 class="label">Sécurité et accès</h4>
            <p>Les modèles peuvent envoyer des données à {host}</p>
          </div>
          <div>
            <h4 class="label">Dans le Chat</h4>
            <p>{detail.enabled ? 'Outils prêtés aux modèles' : 'Désactivé'}</p>
          </div>
        </div>
      </div>
    {:else}
      <div class="view" in:fly={{ ...motion, x: -16 }}>
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
                  <button class="row" type="button" onclick={() => (selected = server.id)}>
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
                    <input type="checkbox" role="switch" bind:checked={server.enabled} onchange={() => toggle(server)} aria-label="Activer {server.name}" />
                    <span class="switch-track" aria-hidden="true"></span>
                  </label>
                  <button class="ghost danger" type="button" onclick={() => remove(server)} aria-label="Supprimer {server.name}"><Icon name="trash" size={18} /></button>
                </li>
              {:else}
                <li class="muted">Aucun serveur.</li>
              {/each}
            </ul>
          {/if}
        </section>
        {#if adding}
          <form class="card add" onsubmit={add} transition:slide={motion}>
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
      </div>
    {/if}
  </div>
</dialog>

<style>
  /* Compound selector: must beat the shared .sheet padding and width. */
  .sheet.mcp {
    width: min(760px, calc(100vw - 2 * var(--s4)));
    min-height: min(560px, calc(100dvh - 2 * var(--s5)));
    margin: auto;
    padding: 0;
    opacity: 0;
    transform: translateY(8px) scale(0.98);
    transition:
      opacity 0.2s var(--ease-out),
      transform 0.2s var(--ease-out),
      overlay 0.2s allow-discrete,
      display 0.2s allow-discrete;
  }

  .sheet.mcp::backdrop {
    opacity: 0;
    transition:
      opacity 0.2s var(--ease-out),
      overlay 0.2s allow-discrete,
      display 0.2s allow-discrete;
  }

  .sheet.mcp[open],
  .sheet.mcp[open]::backdrop {
    opacity: 1;
    transform: none;
  }

  @starting-style {
    .sheet.mcp[open] {
      opacity: 0;
      transform: translateY(8px) scale(0.98);
    }

    .sheet.mcp[open]::backdrop {
      opacity: 0;
    }
  }

  /* minmax(0, 1fr): an auto track grows to the unwrapped host line and pushes the switch out on mobile. */
  .sheet.mcp[open] {
    display: grid;
    grid-template-rows: auto 1fr;
    grid-template-columns: minmax(0, 1fr);
  }

  .head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--s3);
    padding: var(--s3) var(--s4) var(--s3) var(--s5);
    border-bottom: 1px solid var(--hairline);
  }

  .head h2 {
    margin: 0;
    font-size: var(--text-lg);
    font-weight: 600;
  }

  .body {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    align-content: start;
    gap: var(--s5);
    padding: var(--s5);
    overflow: auto;
  }

  .view {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    gap: var(--s5);
  }

  .intro,
  .detail-head p,
  .notes p {
    margin: 0;
  }

  .ghost {
    display: grid;
    flex: none;
    place-items: center;
    width: 36px;
    height: 36px;
    padding: 0;
    border: 0;
    border-radius: var(--radius-sm);
    background: none;
    color: var(--ink-2);
    cursor: pointer;
  }

  .ghost:hover {
    background: var(--raised);
    color: var(--ink);
  }

  .ghost.danger:hover {
    color: var(--critical);
  }

  .ghost:focus-visible,
  .row:focus-visible,
  .back:focus-visible,
  .link:focus-visible {
    outline: 2px solid var(--accent);
    outline-offset: 2px;
  }

  .group {
    display: grid;
    gap: var(--s2);
  }

  .label {
    margin: 0;
    color: var(--ink-2);
    font-size: var(--text-sm);
    font-weight: 600;
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

  .tile {
    display: grid;
    flex: none;
    place-items: center;
    width: 44px;
    height: 44px;
    border: 1px solid var(--hairline);
    border-radius: var(--radius);
    background: var(--panel);
    color: var(--ink-2);
  }

  .tile-lg {
    width: 64px;
    height: 64px;
    border-radius: var(--radius-lg);
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

  .link,
  .back {
    display: inline-flex;
    align-items: center;
    justify-self: start;
    gap: var(--s2);
    padding: 0;
    border: 0;
    border-radius: var(--radius-sm);
    background: none;
    font: inherit;
    cursor: pointer;
  }

  .link {
    color: var(--accent);
  }

  .link:hover {
    text-decoration: underline;
    text-underline-offset: 3px;
  }

  .back {
    color: var(--ink-2);
  }

  .back:hover {
    color: var(--ink);
  }

  .back :global(svg) {
    transform: scaleX(-1);
  }

  .detail-head {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--s4);
  }

  .title {
    flex: 1;
    min-width: 12rem;
  }

  .title h3 {
    margin: 0;
    font-size: 1.5rem;
    font-weight: 600;
  }

  .pill {
    display: inline-flex;
    align-items: center;
    gap: var(--s1);
    padding: var(--s1) var(--s3);
    border-radius: 999px;
    background: color-mix(in srgb, currentColor 14%, transparent);
    font-size: var(--text-sm);
  }

  .card {
    margin: 0;
    border: 1px solid var(--hairline);
    border-radius: var(--radius);
    background: color-mix(in srgb, var(--panel) 70%, transparent);
  }

  .fact {
    display: grid;
    grid-template-columns: 8rem 1fr;
    gap: var(--s4);
    padding: var(--s3) var(--s4);
  }

  .fact + .fact {
    border-top: 1px solid var(--hairline);
  }

  .fact dt {
    color: var(--ink-3);
  }

  .fact dd {
    display: grid;
    gap: var(--s1);
    margin: 0;
    overflow-wrap: anywhere;
  }

  .error {
    font-size: var(--text-sm);
  }

  .notes {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(14rem, 1fr));
    gap: var(--s4);
  }

  .notes > div {
    display: grid;
    gap: var(--s1);
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

    .fact {
      grid-template-columns: 1fr;
      gap: var(--s1);
    }
  }
</style>
