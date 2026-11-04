<script lang="ts">
  import Icon from './Icon.svelte';
  import ModelName from './ModelName.svelte';
  import type { Run } from '../lib/api';
  import { checkDetail, checkLabel, duration, number, score } from '../lib/format';

  let { run, color }: { run: Run; color: string } = $props();
</script>

<article class="run">
  <ModelName name={run.model} {color} />
  {#if run.error}
    <p class="state state-critical"><Icon name="cross" size={16} /> Échec</p>
    <pre class="output">{run.error}</pre>
  {:else}
    <p class="figures">
      <strong>{score(run.score)}/100</strong>
      <span class="muted">{duration(run.total_ms)}, {number(run.tok_per_sec)} jetons/s</span>
    </p>
    <ul>
      {#each run.checks ?? [] as check, index (index)}
        <li class="state" class:state-good={check.passed} class:state-critical={!check.passed}>
          <Icon name={check.passed ? 'check' : 'cross'} size={16} />
          <span class="check-text">{checkLabel(check)}{#if check.detail && !check.passed}<span class="muted"> : {checkDetail(check.detail)}</span>{/if}</span>
        </li>
      {/each}
    </ul>
    <details open>
      <summary>Sortie</summary>
      <pre class="output">{run.output}</pre>
    </details>
  {/if}
</article>

<style>
  .run {
    display: grid;
    align-content: start;
    gap: var(--s2);
    min-width: 0;
    padding: var(--s4);
    border: 1px solid var(--hairline);
    border-radius: var(--radius);
    background: var(--panel);
  }

  .figures {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: var(--s1) var(--s3);
    font-variant-numeric: tabular-nums;
  }

  .figures strong {
    font-size: var(--text-lg);
  }

  .figures .muted {
    font-size: var(--text-sm);
  }

  ul {
    display: grid;
    gap: var(--s1);
    margin: 0;
    padding: 0;
    list-style: none;
  }

  li {
    align-items: flex-start;
  }

  .check-text {
    color: var(--ink);
  }

  summary {
    color: var(--ink-2);
    font-size: var(--text-sm);
    cursor: pointer;
  }

  details .output {
    margin-top: var(--s2);
  }
</style>
