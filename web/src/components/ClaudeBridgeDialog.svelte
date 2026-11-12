<script lang="ts">
  import Icon from './Icon.svelte';
  import Notice from './Notice.svelte';
  import { api, errorMessage } from '../lib/api';

  let { open, added, onclose, onadded }: { open: boolean; added: boolean; onclose: () => void; onadded: () => Promise<void> } = $props();

  const COMMAND = 'make bridge';

  let dialog: HTMLDialogElement;
  let copied = $state(false);
  let error = $state('');

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

  /** Saves the bridge connection, the server adds the token */
  async function add(): Promise<void> {
    try {
      await api.createBridge();
      error = '';
      await onadded();
    } catch (saveError) {
      error = errorMessage(saveError);
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
  <p>
    Un abonnement Claude n'a pas de clé API. La connexion passe par la commande <code>claude</code> du Mac, dont l'identification reste dans le trousseau, hors de portée du conteneur. Le pont lance cette
    commande sur le Mac et la présente à l'app comme un service compatible OpenAI.
  </p>
  <ol>
    <li>
      Dans un terminal, à la racine du projet :
      <span class="command">
        <code>{COMMAND}</code>
        <button class="btn" type="button" onclick={copy}><Icon name={copied ? 'check' : 'copy'} /> {copied ? 'Copié' : 'Copier'}</button>
      </span>
      Le pont reste au premier plan, fermer le terminal l'arrête.
    </li>
    <li>{added ? 'La connexion est déjà dans la liste.' : "Ajouter la connexion, l'app y met le jeton du pont."}</li>
  </ol>
  <Notice message={error} />
  <p class="warning">
    Le pont utilise le compte personnel connecté à <code>claude</code> sur ce Mac. Chaque question consomme son quota et part chez Anthropic, avec les fichiers joints et les extraits du corpus. À
    utiliser à la discrétion du titulaire du compte.
  </p>
  {#if !added}
    <button class="btn btn-primary" type="button" onclick={add}><Icon name="plug" /> Ajouter la connexion</button>
  {/if}
</dialog>

<style>
  .sheet[open] {
    display: grid;
    justify-items: start;
    gap: var(--s3);
    overflow: auto;
  }

  .sheet-head {
    justify-self: stretch;
  }

  ol {
    display: grid;
    gap: var(--s3);
    padding-left: var(--s4);
  }

  .command {
    display: flex;
    align-items: center;
    gap: var(--s3);
    margin-block: var(--s2);
  }

  .warning {
    color: var(--ink-2);
  }
</style>
