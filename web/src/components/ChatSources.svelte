<script lang="ts">
  import Icon from './Icon.svelte';
  import type { ChatDocument, ChatPassage } from '../lib/api';
  import { number } from '../lib/format';

  let { passages, document }: { passages: ChatPassage[]; document: ChatDocument | null } = $props();
</script>

{#if document}
  <p class="attached muted">
    <Icon name="doc" size={16} />
    <span>{document.name}, {number(document.chars)} caractères lus</span>
    {#if document.truncated}
      <span class="state-warning">(tronqué : {number(document.total_chars - document.chars)} caractères non transmis)</span>
    {/if}
  </p>
{/if}

{#if passages.length > 0}
  <details class="sources">
    <summary>Passages retrouvés ({passages.length})</summary>
    <ol>
      {#each passages as passage (passage.file + passage.position)}
        <li>
          <p class="muted">{passage.file}, similarité {number(passage.similarity * 100)} %</p>
          <pre>{passage.content}</pre>
        </li>
      {/each}
    </ol>
  </details>
{/if}

<style>
  .attached {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--s2);
    margin: 0;
  }

  .sources {
    max-width: 75ch;
  }

  summary {
    color: var(--ink-2);
    cursor: pointer;
  }

  ol {
    display: grid;
    gap: var(--s3);
    margin: var(--s3) 0 0;
    padding: 0;
    list-style: none;
  }

  li {
    display: grid;
    gap: var(--s1);
  }

  pre {
    max-height: 14rem;
    margin: 0;
    padding: var(--s3);
    overflow: auto;
    border: 1px solid var(--hairline);
    border-radius: var(--radius);
    background: var(--panel);
    font: inherit;
    font-size: var(--text-sm);
    overflow-wrap: anywhere;
    white-space: pre-wrap;
  }
</style>
