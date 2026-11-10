<script lang="ts" module>
  export interface BridgeConnection {
    name: string;
    kind: 'openai';
    base_url: string;
  }
</script>

<script lang="ts">
  import Icon from './Icon.svelte';
  import Notice from './Notice.svelte';

  let { open, onclose, onprefill }: { open: boolean; onclose: () => void; onprefill: (connection: BridgeConnection) => void } = $props();

  const COMMAND = 'make bridge';
  // Default AXOLOT_BRIDGE_PORT: the app does not see the host .env.
  const CONNECTION: BridgeConnection = { name: 'Claude (abonnement)', kind: 'openai', base_url: 'http://host.docker.internal:5092/v1' };

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
    <li>Préremplir la connexion, puis coller dans Clé API la valeur de <code>AXOLOT_BRIDGE_TOKEN</code> du fichier <code>.env</code>.</li>
  </ol>
  <Notice message={error} />
  <p class="warning">
    Le pont utilise le compte personnel connecté à <code>claude</code> sur ce Mac. Chaque question consomme son quota et part chez Anthropic, avec les fichiers joints et les extraits du corpus. À
    utiliser à la discrétion du titulaire du compte.
  </p>
  <button class="btn btn-primary" type="button" onclick={() => onprefill(CONNECTION)}><Icon name="plug" /> Préremplir la connexion</button>
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
