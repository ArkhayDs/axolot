<script lang="ts">
  import ChatApproval from './ChatApproval.svelte';
  import { awaitsUser, prettyJson, shortArgs } from '../lib/chat';
  import type { ToolPart } from '../lib/chat';
  import { approvalFold } from '../lib/fold.svelte';
  import { duration } from '../lib/format';

  // onseen: whether the approval buttons are on screen, bottomInset left out (the composer covers it).
  let { step, live, bottomInset, onseen }: { step: ToolPart; live: boolean; bottomInset: number; onseen: (id: string, seen: boolean) => void } = $props();

  let buttons = $state<HTMLElement>();

  let waiting = $derived(awaitsUser(step, live));
  const fold = approvalFold(
    () => waiting,
    () => step.result !== null,
  );

  // Answered or stopped: reported as seen so the page drops its off-screen entry.
  $effect(() => {
    if (!waiting && step.approval !== null) onseen(step.approval, true);
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

<details class="step fold" bind:open={fold.open}>
  <summary>
    <code title={step.arguments}>{step.name}({shortArgs(step.arguments)})</code>
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
  }

  summary {
    display: flex;
    align-items: center;
    gap: var(--s2);
    color: var(--ink-2);
    white-space: nowrap;
    cursor: pointer;
  }

  /* One line per call: long arguments end in an ellipsis, the full text sits under the fold and in the tooltip. */
  code {
    min-width: 0;
    overflow: hidden;
    font-family: var(--mono);
    text-overflow: ellipsis;
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
</style>
