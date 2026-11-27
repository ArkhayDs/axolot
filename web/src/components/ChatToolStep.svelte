<script lang="ts">
  import ChatApproval from './ChatApproval.svelte';
  import Icon from './Icon.svelte';
  import { awaitsUser, prettyJson } from '../lib/chat';
  import type { ToolPart } from '../lib/chat';
  import { duration } from '../lib/format';

  // onseen: whether the approval buttons are on screen, bottomInset left out (the composer covers it).
  let { step, live, bottomInset, onseen }: { step: ToolPart; live: boolean; bottomInset: number; onseen: (id: string, seen: boolean) => void } = $props();

  let open = $state(false);
  let buttons = $state<HTMLElement>();
  let openedForApproval = false;

  let waiting = $derived(awaitsUser(step, live));

  // Opened for the user to read the arguments, folded again once the call is done; a step the user opened stays open.
  $effect(() => {
    if (waiting) {
      open = true;
      openedForApproval = true;
    } else if (openedForApproval && step.result !== null) {
      open = false;
      openedForApproval = false;
    }
  });

  $effect(() => {
    if (!buttons || step.approval === null) return;
    const id = step.approval;
    const observer = new IntersectionObserver(([entry]) => onseen(id, entry.isIntersecting), { rootMargin: `0px 0px -${bottomInset}px 0px` });
    observer.observe(buttons);
    // No "unseen" on cleanup: a rerun when the composer resizes would flash the bar and resize it again.
    return () => observer.disconnect();
  });
</script>

<details class="step" bind:open>
  <summary>
    <Icon name="plug" size={16} />
    <code>{step.name}</code>
    {#if waiting}
      <span class="state-warning">en attente de validation</span>
    {:else if step.result === null}
      <span class="muted">en cours...</span>
    {:else}
      <span class="muted">{duration(step.ms)}</span>
      {#if step.refused}
        <span class="muted">refusé</span>
      {:else if step.error}
        <span class="state-critical">échec</span>
      {/if}
    {/if}
  </summary>
  <p class="muted">Arguments</p>
  <pre>{prettyJson(step.arguments)}</pre>
  {#if step.result !== null}
    <p class="muted">Résultat</p>
    <pre>{step.result}</pre>
  {/if}
</details>
{#if waiting}
  <div id="approval-{step.approval}" bind:this={buttons}>
    <ChatApproval {step} />
  </div>
{/if}

<style>
  .step {
    display: grid;
    gap: var(--s1);
    font-size: var(--text-sm);
    interpolate-size: allow-keywords;
  }

  .step::details-content {
    block-size: 0;
    overflow: hidden;
    transition:
      block-size 220ms ease,
      content-visibility 220ms allow-discrete;
  }

  .step[open]::details-content {
    block-size: auto;
  }

  summary {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--s2);
    color: var(--ink-2);
    cursor: pointer;
  }

  code {
    font-family: var(--mono);
    overflow-wrap: anywhere;
  }

  p {
    margin: var(--s2) 0 0;
  }

  pre {
    max-height: 14rem;
    margin: var(--s1) 0 0;
    padding: var(--s3);
    overflow: auto;
    border: 1px solid var(--hairline);
    border-radius: var(--radius);
    background: var(--ground);
    font-family: var(--mono);
    overflow-wrap: anywhere;
    white-space: pre-wrap;
  }

  /* Own rule: ::details-content in the global reduced-motion list would void it where unsupported. */
  @media (prefers-reduced-motion: reduce) {
    .step::details-content {
      transition: none;
    }
  }
</style>
