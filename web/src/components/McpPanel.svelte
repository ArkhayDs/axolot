<script lang="ts">
  import { fly } from 'svelte/transition';

  import Icon from './Icon.svelte';
  import McpServerDetail from './McpServerDetail.svelte';
  import McpServerList from './McpServerList.svelte';
  import Notice from './Notice.svelte';
  import { api, errorMessage } from '../lib/api';
  import type { McpProbe, McpServer, McpTool, McpToolMode } from '../lib/api';
  import { motion } from '../lib/mcp';

  let { open, onclose }: { open: boolean; onclose: () => void } = $props();

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
   * Saves a tool's permission, turned back when the server refuses
   * @param {McpServer} server - server offering the tool
   * @param {McpTool} tool - tool changed
   * @param {McpToolMode} mode - permission picked
   */
  async function setMode(server: McpServer, tool: McpTool, mode: McpToolMode): Promise<void> {
    const previous = tool.mode;
    tool.mode = mode;
    try {
      await api.setMcpToolMode(server.id, tool.name, mode);
      error = '';
    } catch (saveError) {
      tool.mode = previous;
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
      <div class="view" in:fly={{ ...motion, x: 16 }}>
        <McpServerDetail server={detail} probe={probes[detail.id]} onback={() => (selected = null)} onsignin={signIn} onmode={setMode} />
      </div>
    {:else}
      <div class="view" in:fly={{ ...motion, x: -16 }}>
        <McpServerList
          bind:servers
          {probes}
          bind:adding
          bind:name
          bind:url
          bind:token
          {saving}
          onselect={(id) => (selected = id)}
          onadd={add}
          ontoggle={toggle}
          onremove={remove}
        />
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

  /* Shared with the list and detail views: :global reaches their markup, .mcp keeps it inside this dialog.
     The .mcp prefix adds one class, so .ghost.danger and .tile-lg stay here to still beat .ghost:hover and .tile. */
  .mcp :global(.ghost) {
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

  .mcp :global(.ghost:hover) {
    background: var(--raised);
    color: var(--ink);
  }

  .mcp :global(.ghost.danger:hover) {
    color: var(--critical);
  }

  .mcp :global(.ghost:focus-visible),
  .mcp :global(.row:focus-visible),
  .mcp :global(.back:focus-visible),
  .mcp :global(.link:focus-visible) {
    outline: 2px solid var(--accent);
    outline-offset: 2px;
  }

  .mcp :global(.group) {
    display: grid;
    gap: var(--s2);
  }

  .mcp :global(.label) {
    margin: 0;
    color: var(--ink-2);
    font-size: var(--text-sm);
    font-weight: 600;
  }

  .mcp :global(.tile) {
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

  .mcp :global(.tile-lg) {
    width: 64px;
    height: 64px;
    border-radius: var(--radius-lg);
  }

  .mcp :global(.link),
  .mcp :global(.back) {
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

  .mcp :global(.card) {
    margin: 0;
    border: 1px solid var(--hairline);
    border-radius: var(--radius);
    background: color-mix(in srgb, var(--panel) 70%, transparent);
  }
</style>
