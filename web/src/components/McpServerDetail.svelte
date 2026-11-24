<script lang="ts">
  import { fade } from 'svelte/transition';

  import Icon from './Icon.svelte';
  import type { McpProbe, McpServer, McpTool, McpToolMode } from '../lib/api';
  import { motion, stateOf } from '../lib/mcp';

  let {
    server,
    probe,
    onback,
    onsignin,
    onmode,
  }: {
    server: McpServer;
    probe: McpProbe | undefined;
    onback: () => void;
    onsignin: (server: McpServer) => Promise<void>;
    onmode: (server: McpServer, tool: McpTool, mode: McpToolMode) => Promise<void>;
  } = $props();

  const TABS = [
    { key: 'connection', label: 'Connexion' },
    { key: 'permissions', label: 'Permissions' },
  ] as const;

  const MODES: { value: McpToolMode; label: string }[] = [
    { value: 'allow', label: 'Autorisé' },
    { value: 'ask', label: 'Validation requise' },
    { value: 'block', label: 'Bloqué' },
  ];

  let tab = $state<(typeof TABS)[number]['key']>('connection');
  let status = $derived(stateOf(probe));
  let host = $derived(new URL(server.url).host);
  let noTools = $derived(!probe ? 'Vérification...' : probe.login ? 'Visibles une fois connecté' : 'Aucun outil');

  /**
   * Arrow keys move between tabs, as the tabs pattern expects
   * @param {KeyboardEvent} event - key press on a tab
   * @param {number} position - index of the focused tab
   */
  function onTabKey(event: KeyboardEvent, position: number): void {
    const moves: Record<string, number> = { ArrowRight: position + 1, ArrowLeft: position - 1, Home: 0, End: TABS.length - 1 };
    const target = moves[event.key];
    if (target === undefined) return;
    event.preventDefault();
    const next = (target + TABS.length) % TABS.length;
    tab = TABS[next].key;
    document.getElementById(`mcp-tab-${TABS[next].key}`)?.focus();
  }
</script>

<button class="back" type="button" onclick={onback}><Icon name="arrow" size={16} /> Serveurs MCP</button>
<div class="detail-head">
  <span class="tile tile-lg"><Icon name="plug" size={26} /></span>
  <div class="title">
    <h3>{server.name}</h3>
    <p class="muted">Serveur MCP · {host}</p>
  </div>
  {#if probe?.login}
    <button class="btn btn-primary" type="button" onclick={() => onsignin(server)} in:fade={motion}><Icon name="plug" /> Se connecter</button>
  {:else}
    {#key status.label}
      <span class="pill {status.tone}" in:fade={motion}>{#if status.icon}<Icon name={status.icon} size={14} />{/if} {status.label}</span>
    {/key}
  {/if}
</div>
<div class="tabs" role="tablist" aria-label="Vues du serveur">
  {#each TABS as item, position (item.key)}
    <button
      id="mcp-tab-{item.key}"
      type="button"
      role="tab"
      aria-selected={tab === item.key}
      aria-controls="mcp-tabpanel"
      tabindex={tab === item.key ? 0 : -1}
      onclick={() => (tab = item.key)}
      onkeydown={(event) => onTabKey(event, position)}
    >
      {item.label}
    </button>
  {/each}
</div>
{#key tab}
  <div id="mcp-tabpanel" class="tabpanel" role="tabpanel" aria-labelledby="mcp-tab-{tab}" tabindex="0" in:fade={motion}>
    {#if tab === 'connection'}
      <dl class="card">
        <div class="fact">
          <dt>URL</dt>
          <dd>{server.url}</dd>
        </div>
        <div class="fact">
          <dt>État</dt>
          <dd>
            <span class={status.tone}>{status.label}</span>
            {#if probe?.error && !probe.login}<span class="muted error">{probe.error}</span>{/if}
          </dd>
        </div>
        <div class="fact">
          <dt>Outils</dt>
          <dd>
            {#if probe && probe.tools.length > 0}
              {probe.tools.map((tool) => tool.name).join(', ')}
            {:else}
              <span class="muted">{noTools}</span>
            {/if}
          </dd>
        </div>
      </dl>
      <div class="notes">
        <div>
          <h4 class="label">Sécurité et accès</h4>
          <p>Les modèles peuvent envoyer des données à {host}</p>
        </div>
        <div>
          <h4 class="label">Dans le Chat</h4>
          <p>{server.enabled ? 'Outils prêtés aux modèles' : 'Désactivé'}</p>
        </div>
      </div>
    {:else}
      <p class="muted">
        Autorisé : appelé sans demander. Validation requise : le Chat attend votre accord à chaque appel. Bloqué : jamais proposé aux modèles. Vaut pour tous les modèles.
      </p>
      {#if probe && probe.tools.length > 0}
        <ul class="card tools">
          {#each probe.tools as tool (tool.name)}
            <li class="tool">
              <div class="tool-text">
                <span class="tool-name">{tool.name}</span>
                {#if tool.description}<p class="muted description" title={tool.description}>{tool.description}</p>{/if}
              </div>
              <select
                class="input mode"
                value={tool.mode}
                aria-label="Permission de {tool.name}"
                onchange={(event) => onmode(server, tool, event.currentTarget.value as McpToolMode)}
              >
                {#each MODES as mode (mode.value)}
                  <option value={mode.value}>{mode.label}</option>
                {/each}
              </select>
            </li>
          {/each}
        </ul>
      {:else}
        <p class="muted">{noTools}</p>
      {/if}
    {/if}
  </div>
{/key}

<style>
  .detail-head p,
  .notes p,
  .tabpanel p {
    margin: 0;
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

  .tabpanel {
    display: grid;
    gap: var(--s5);
  }

  .fact,
  .tool {
    padding: var(--s3) var(--s4);
  }

  .fact {
    display: grid;
    grid-template-columns: 8rem 1fr;
    gap: var(--s4);
  }

  .fact + .fact,
  .tool + .tool {
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

  .error,
  .description {
    font-size: var(--text-sm);
  }

  .notes {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(14rem, 1fr));
    gap: var(--s4);
  }

  .notes > div,
  .tool-text {
    display: grid;
    gap: var(--s1);
  }

  .tools {
    padding: 0;
    list-style: none;
  }

  .tool {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--s3) var(--s4);
  }

  .tool-text {
    flex: 1;
    min-width: 14rem;
  }

  .tool-name {
    font-weight: 600;
    overflow-wrap: anywhere;
  }

  .description {
    display: -webkit-box;
    overflow: hidden;
    -webkit-box-orient: vertical;
    -webkit-line-clamp: 3;
    line-clamp: 3;
  }

  .mode {
    width: auto;
  }

  @media (max-width: 480px) {
    .fact {
      grid-template-columns: 1fr;
      gap: var(--s1);
    }

    .mode {
      width: 100%;
    }
  }
</style>
