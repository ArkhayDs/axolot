<script lang="ts">
  import { fade } from 'svelte/transition';

  import Icon from './Icon.svelte';
  import type { McpProbe, McpServer } from '../lib/api';
  import { motion, stateOf } from '../lib/mcp';

  let {
    server,
    probe,
    onback,
    onsignin,
  }: { server: McpServer; probe: McpProbe | undefined; onback: () => void; onsignin: (server: McpServer) => Promise<void> } = $props();

  let state = $derived(stateOf(probe));
  let host = $derived(new URL(server.url).host);
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
      <dd>{server.url}</dd>
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
          {probe.tools.map((tool) => tool.name).join(', ')}
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
    <p>{server.enabled ? 'Outils prêtés aux modèles' : 'Désactivé'}</p>
  </div>
</div>

<style>
  .detail-head p,
  .notes p {
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

  @media (max-width: 480px) {
    .fact {
      grid-template-columns: 1fr;
      gap: var(--s1);
    }
  }
</style>
