<script lang="ts">
  import ChatToolStep from './ChatToolStep.svelte';
  import Icon from './Icon.svelte';
  import { awaitsUser, toolSummary } from '../lib/chat';
  import type { ToolPart } from '../lib/chat';
  import { approvalFold } from '../lib/fold.svelte';
  import { duration } from '../lib/format';

  // Consecutive tool calls folded behind one line; props other than steps go through to each step.
  let { steps, live, bottomInset, onseen }: { steps: ToolPart[]; live: boolean; bottomInset: number; onseen: (id: string, seen: boolean) => void } = $props();

  let waiting = $derived(steps.some((step) => awaitsUser(step, live)));
  let running = $derived(steps.some((step) => step.result === null));
  let summary = $derived(toolSummary(steps));
  const fold = approvalFold(
    () => waiting,
    () => !running,
  );
</script>

<details class="run fold" bind:open={fold.open}>
  <summary>
    <Icon name="plug" size={16} />
    <span>{summary.calls} {summary.calls > 1 ? 'appels' : 'appel'} · {duration(summary.ms)}</span>
    {#if waiting}
      <span class="state-warning">en attente de validation</span>
    {:else if running && live}
      <span class="muted">en cours...</span>
    {/if}
  </summary>
  <div class="steps">
    {#each steps as step, index (index)}
      <ChatToolStep {step} {live} {bottomInset} {onseen} />
    {/each}
  </div>
</details>

<style>
  .run {
    font-size: var(--text-sm);
  }

  summary {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--s2);
    color: var(--ink-2);
    cursor: pointer;
  }

  .steps {
    display: grid;
    gap: var(--s2);
    margin-top: var(--s2);
    padding-left: var(--s3);
    border-left: 1px solid var(--hairline);
  }
</style>
