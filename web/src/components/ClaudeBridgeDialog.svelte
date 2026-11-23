<script lang="ts">
  import Icon from './Icon.svelte';
  import Notice from './Notice.svelte';
  import { api, errorMessage } from '../lib/api';

  let { open, added, onclose, onadded }: { open: boolean; added: boolean; onclose: () => void; onadded: () => Promise<void> } = $props();

  const COMMAND = 'make bridge';

  let dialog: HTMLDialogElement;
  let copied = $state(false);
  let error = $state('');
  let adding = $state(false);

  /** Copies the bridge command, the page is on loopback so the clipboard API is allowed */
  async function copy(): Promise<void> {
    try {
      await navigator.clipboard.writeText(COMMAND);
      copied = true;
      error = '';
    } catch {
      error = 'Copie refusée par le navigateur, sélectionner la commande à la main.';
    }
  }

  /** Saves the bridge connection, locked until the reloaded list hides the button; reloads on refusal too, another tab may have added it */
  async function add(): Promise<void> {
    adding = true;
    try {
      await api.createBridge();
      error = '';
      await onadded();
    } catch (saveError) {
      error = errorMessage(saveError);
      await onadded();
    } finally {
      adding = false;
    }
  }

  $effect(() => {
    if (open) {
      copied = false;
      error = '';
      dialog.showModal();
    } else if (dialog.open) dialog.close();
  });
</script>

<dialog class="sheet glass" bind:this={dialog} onclose={onclose} aria-labelledby="bridge-title">
  <div class="sheet-head">
    <h2 id="bridge-title">Compte Claude Pro ou Max</h2>
    <button class="btn" type="button" onclick={onclose} aria-label="Fermer"><Icon name="cross" /></button>
  </div>
  <figure>
    <ol class="path" aria-label="Chemin d'une question">
      <li class="node"><strong>Axolot</strong></li>
      <li class="arrow" aria-hidden="true"><Icon name="exchange" size={16} /></li>
      <li class="node node-bridge"><strong>Pont</strong><small>lance <code>claude -p</code> sur le Mac</small></li>
      <li class="arrow" aria-hidden="true"><Icon name="exchange" size={16} /></li>
      <li class="node"><strong>Abonnement Claude</strong></li>
    </ol>
    <figcaption>Un abonnement n'a pas de clé API : le pont le présente à Axolot comme un service en ligne.</figcaption>
  </figure>
  <ol class="steps">
    <li>
      {#if added}
        Connexion ajoutée.
      {:else}
        Une fois :
        <button class="btn btn-primary" type="button" onclick={add} disabled={adding}><Icon name="plug" /> Ajouter la connexion</button>
      {/if}
    </li>
    <li>
      À chaque session, à la racine du projet, terminal ouvert :
      <figure class="terminal">
        <div class="terminal-head">
          <code><span class="prompt">$</span> {COMMAND}</code>
          <button class="btn" type="button" onclick={copy}><Icon name={copied ? 'check' : 'copy'} /> {copied ? 'Copié' : 'Copier'}</button>
        </div>
        <pre>INFO axolot-bridge <mark>listening</mark> addr=127.0.0.1:…</pre>
        <figcaption>Prêt quand <code>listening</code> s'affiche.</figcaption>
      </figure>
    </li>
  </ol>
  <Notice message={error} />
  <p class="warning">Consomme le quota du compte connecté à <code>claude</code>. Questions, fichiers joints et extraits du corpus partent chez Anthropic.</p>
</dialog>

<style>
  .sheet {
    width: min(600px, calc(100vw - 2 * var(--s4)));
    margin: auto;
  }

  figure {
    margin: 0;
  }

  .sheet[open] {
    display: grid;
    gap: var(--s4);
    overflow: auto;
  }

  .path {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--s2);
    list-style: none;
    padding: 0;
  }

  .node {
    display: grid;
    padding: var(--s2) var(--s3);
    border: 1px solid var(--hairline);
    border-radius: var(--radius);
    background: var(--raised);
  }

  .node-bridge {
    border-color: var(--accent);
  }

  .node small,
  .arrow,
  figcaption,
  .warning {
    color: var(--ink-3);
  }

  .node small {
    font-size: var(--text-sm);
  }

  figcaption {
    margin-top: var(--s2);
    font-size: var(--text-sm);
  }

  .steps {
    display: grid;
    gap: var(--s3);
    padding-left: var(--s4);
  }

  .terminal {
    display: grid;
    gap: var(--s2);
    margin-top: var(--s2);
    padding: var(--s3);
    border: 1px solid var(--hairline);
    border-radius: var(--radius);
    background: var(--ground);
  }

  .terminal-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--s3);
  }

  .prompt,
  pre {
    color: var(--ink-3);
  }

  pre {
    margin: 0;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }

  mark {
    background: none;
    color: var(--good);
  }

  .warning {
    font-size: var(--text-sm);
  }

  @media (max-width: 760px) {
    .path {
      flex-direction: column;
      align-items: stretch;
    }

    .arrow {
      align-self: center;
      rotate: 90deg;
    }
  }
</style>
