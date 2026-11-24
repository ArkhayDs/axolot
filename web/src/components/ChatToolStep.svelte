<script lang="ts">
  import Icon from './Icon.svelte';
  import { prettyJson } from '../lib/chat';
  import type { AnswerPart } from '../lib/chat';
  import { duration } from '../lib/format';

  let { step }: { step: Extract<AnswerPart, { kind: 'tool' }> } = $props();
</script>

<details class="step">
  <summary>
    <Icon name="plug" size={16} />
    <code>{step.name}</code>
    {#if step.result === null}
      <span class="muted">en cours...</span>
    {:else}
      <span class="muted">{duration(step.ms)}</span>
      {#if step.error}
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

<style>
  .step {
    display: grid;
    gap: var(--s1);
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
</style>
