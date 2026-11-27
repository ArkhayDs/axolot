<script lang="ts">
  import ChatApproval from './ChatApproval.svelte';
  import type { ToolPart } from '../lib/chat';

  // Calls waiting for the user's click whose buttons are off screen, repeated above the composer.
  let { approvals }: { approvals: { model: string; step: ToolPart }[] } = $props();
</script>

{#if approvals.length > 0}
  <ul class="approvals" aria-label="Appels en attente de votre accord">
    {#each approvals as { model, step } (step.approval)}
      <li>
        <span><strong>{model}</strong> attend votre accord · <code>{step.name}</code></span>
        <button class="btn" type="button" onclick={() => document.getElementById(`approval-${step.approval}`)?.scrollIntoView({ block: 'center', behavior: 'smooth' })}>Voir</button>
        <ChatApproval {step} />
      </li>
    {/each}
  </ul>
{/if}

<style>
  .approvals {
    display: grid;
    gap: var(--s2);
    margin: 0;
    padding: 0 0 var(--s3);
    border-bottom: 1px solid var(--hairline);
    list-style: none;
    font-size: var(--text-sm);
  }

  li {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--s2) var(--s3);
  }

  span {
    flex: 1 1 16rem;
  }

  code {
    font-family: var(--mono);
    overflow-wrap: anywhere;
  }
</style>
