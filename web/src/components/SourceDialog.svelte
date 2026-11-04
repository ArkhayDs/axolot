<script lang="ts">
  import { tick } from 'svelte';

  import Icon from './Icon.svelte';
  import Notice from './Notice.svelte';
  import { api, errorMessage } from '../lib/api';
  import type { Source } from '../lib/api';

  let { corpus, source, onclose }: { corpus: string; source: Source | null; onclose: () => void } = $props();

  let dialog: HTMLDialogElement;
  let text = $state('');
  let error = $state('');
  let quoteMark = $state<HTMLElement | undefined>();

  const index = $derived(source ? text.indexOf(source.quote) : -1);

  /**
   * Opens the document and scrolls to the expected passage
   * @param {Source} target - file and quote to show
   */
  async function open(target: Source): Promise<void> {
    text = '';
    error = '';
    dialog.showModal();
    try {
      text = await api.corpusFile(corpus, target.file);
      await tick();
      quoteMark?.scrollIntoView({ block: 'center' });
    } catch (loadError) {
      error = errorMessage(loadError);
    }
  }

  $effect(() => {
    if (source) void open(source);
    else if (dialog.open) dialog.close();
  });
</script>

<dialog class="sheet glass" bind:this={dialog} onclose={onclose} aria-labelledby="source-title">
  <div class="sheet-head">
    <h2 id="source-title"><Icon name="doc" /> {source?.file}</h2>
    <button class="btn" type="button" onclick={onclose} aria-label="Fermer"><Icon name="cross" /></button>
  </div>
  <Notice message={error} />
  {#if text && source}
    {#if index < 0}
      <Notice message="Citation introuvable dans ce document." />
      <div class="document">{text}</div>
    {:else}
      <div class="document">{text.slice(0, index)}<mark bind:this={quoteMark}>{source.quote}</mark>{text.slice(index + source.quote.length)}</div>
    {/if}
  {/if}
</dialog>

<style>
  .sheet {
    width: min(720px, calc(100vw - 2 * var(--s4)));
    max-height: calc(100dvh - 2 * var(--s5));
    margin: auto var(--s4) auto auto;
    padding: var(--s4);
    border-radius: var(--radius-lg);
    color: var(--ink);
  }

  .sheet[open] {
    display: grid;
    grid-template-rows: auto auto minmax(0, 1fr);
    gap: var(--s3);
  }

  .sheet::backdrop {
    background: color-mix(in srgb, var(--ground) 60%, transparent);
  }

  .sheet-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--s3);
  }

  h2 {
    display: flex;
    align-items: center;
    gap: var(--s2);
    overflow-wrap: anywhere;
  }

  .document {
    overflow: auto;
    padding: var(--s4);
    border-radius: var(--radius);
    background: var(--ground);
    color: var(--ink-2);
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }

  mark {
    padding: 1px 2px;
    border-radius: 3px;
    background: var(--accent);
    color: var(--on-accent);
  }
</style>
