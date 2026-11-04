<script lang="ts">
  import Icon from './Icon.svelte';
  import { api, errorMessage } from '../lib/api';
  import type { ChatDocument } from '../lib/api';
  import { number } from '../lib/format';

  let {
    useDocuments = $bindable(),
    document = $bindable(),
    attaching = $bindable(),
    blocked,
  }: {
    useDocuments: boolean;
    document: ChatDocument | null;
    attaching: boolean;
    blocked: boolean;
  } = $props();

  let error = $state('');
  let documentInput: HTMLInputElement | undefined = $state();

  /**
   * Reads the picked document on the server, keeping only its text for this turn
   * @param {Event & { currentTarget: HTMLInputElement }} event - file input change
   */
  async function attachDocument(event: Event & { currentTarget: HTMLInputElement }): Promise<void> {
    const input = event.currentTarget;
    const file = input.files?.[0];
    if (!file) return;
    error = '';
    attaching = true;
    try {
      document = await api.chatDocument(file);
    } catch (attachError) {
      document = null;
      error = errorMessage(attachError);
    }
    attaching = false;
    input.value = '';
  }
</script>

<div class="options">
  <label class="switch">
    <input type="checkbox" role="switch" bind:checked={useDocuments} disabled={blocked} />
    <span class="track" aria-hidden="true"></span>
    Documents (RAG)
  </label>
  <button class="btn attach" type="button" disabled={blocked || attaching} onclick={() => documentInput?.click()}>
    <Icon name="doc" size={16} /> {attaching ? 'Lecture...' : 'Joindre un document'}
  </button>
  <input bind:this={documentInput} type="file" hidden accept=".md,.txt,.docx,.pdf,.pptx,.xlsx,.eml" onchange={attachDocument} />
  {#if document}
    <span class="pill">
      <Icon name="doc" size={16} />
      <span class="name">{document.name}</span>
      <span class="muted">{number(document.chars)} caractères</span>
      <button class="remove" type="button" aria-label="Retirer {document.name}" onclick={() => (document = null)}><Icon name="cross" size={14} /></button>
    </span>
    {#if document.truncated}
      <span class="note state-warning">Tronqué : {number(document.chars)} caractères transmis sur {number(document.total_chars)}, le contexte du modèle ne peut pas contenir la suite.</span>
    {/if}
  {/if}
  {#if error}
    <span class="note state-critical" role="alert">{error}</span>
  {/if}
</div>

<style>
  .options {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--s2) var(--s4);
  }

  .switch {
    position: relative;
    display: inline-flex;
    align-items: center;
    gap: var(--s2);
    min-height: 32px;
    font-size: var(--text-sm);
    cursor: pointer;
  }

  .switch input {
    position: absolute;
    opacity: 0;
  }

  .track {
    position: relative;
    width: 32px;
    height: 18px;
    border: 1px solid var(--axis);
    border-radius: 9px;
    transition: background 120ms ease-out;
  }

  .track::after {
    position: absolute;
    top: 2px;
    left: 2px;
    width: 12px;
    height: 12px;
    border-radius: 50%;
    background: var(--ink-2);
    content: '';
    transition: transform 120ms ease-out;
  }

  .switch input:checked + .track {
    border-color: var(--accent);
    background: var(--accent);
  }

  .switch input:checked + .track::after {
    background: var(--on-accent);
    transform: translateX(14px);
  }

  .switch input:focus-visible + .track {
    outline: 2px solid var(--accent);
    outline-offset: 2px;
  }

  .switch input:disabled + .track {
    opacity: 0.5;
  }

  .attach {
    min-height: 32px;
    padding-block: 0;
    font-size: var(--text-sm);
  }

  .pill {
    display: inline-flex;
    align-items: center;
    gap: var(--s2);
    min-height: 32px;
    padding-inline: var(--s3) var(--s1);
    border: 1px solid var(--axis);
    border-radius: var(--radius);
    font-size: var(--text-sm);
  }

  .name {
    max-width: 24ch;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .remove {
    display: inline-grid;
    place-items: center;
    width: 28px;
    height: 28px;
    border: 0;
    border-radius: var(--radius);
    background: transparent;
    color: var(--ink-2);
    cursor: pointer;
  }

  .remove:hover {
    color: var(--ink);
  }

  .note {
    flex-basis: 100%;
    font-size: var(--text-sm);
  }
</style>
