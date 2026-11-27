<script lang="ts">
  import ChatToolRun from './ChatToolRun.svelte';
  import Icon from './Icon.svelte';
  import ModelName from './ModelName.svelte';
  import { toolRuns, toolSummary } from '../lib/chat';
  import type { Answer } from '../lib/chat';
  import { BUFFERED_HINT, STARTUP_HINT, duration, number } from '../lib/format';

  // One model's answer card; searchMs is the turn's document search, shown with the metrics; bottomInset and onseen go to the tool runs.
  let {
    answer,
    color,
    imageSent,
    searchMs,
    bottomInset,
    onseen,
  }: { answer: Answer; color: string; imageSent: boolean; searchMs: number | null; bottomInset: number; onseen: (id: string, seen: boolean) => void } = $props();
</script>

{#snippet pending(text: string, tone: string)}
  <p class={tone}>{text}<span class="dots" aria-hidden="true"><span>.</span><span>.</span><span>.</span></span></p>
{/snippet}

<div class="answer">
  <ModelName name={answer.model} {color} />
  {#if imageSent && answer.vision === undefined}
    <p class="muted">Ce serveur n'indique pas si le modèle lit les images : réponse à vérifier.</p>
  {/if}
  {#each answer.notices as notice, index (index)}
    <p class="state state-warning"><Icon name="alert" size={16} /> {notice}</p>
  {/each}
  {#if answer.state === 'no-image'}
    <p class="state state-warning"><Icon name="alert" size={16} /> Ce modèle ne lit pas les images</p>
  {:else if answer.state === 'waiting'}
    <p class="muted">En attente de l'autre modèle...</p>
  {:else if answer.state === 'streaming' && answer.parts.length === 0}
    {@render pending('Chargement du modèle', 'muted')}
  {/if}
  {#each toolRuns(answer.parts) as block, index (index)}
    {#if Array.isArray(block)}
      <ChatToolRun steps={block} live={answer.state === 'streaming'} {bottomInset} {onseen} />
    {:else if block.kind === 'text'}
      <div class="text">{block.text}</div>
    {:else if block.ms !== null}
      <p class="musing muted">{block.word} pendant {duration(block.ms)}</p>
    {:else if answer.state === 'streaming'}
      {@render pending(block.word, 'musing live')}
    {/if}
  {/each}
  {#if answer.state === 'failed'}
    <p class="state state-critical"><Icon name="cross" size={16} /> {answer.error}</p>
  {/if}
  {#if answer.state === 'stopped'}
    <p class="muted">Arrêté.</p>
  {/if}
  {#if answer.metrics}
    {@const tools = toolSummary(answer.parts)}
    {@const startup = answer.metrics.load_ms ?? answer.metrics.cli_startup_ms}
    <dl class="metrics">
      {#if searchMs !== null}
        <div><dt>Recherche</dt><dd>{duration(searchMs)}</dd></div>
      {/if}
      <div><dt>1er token</dt><dd>{duration(answer.metrics.ttft_ms)}</dd></div>
      <div><dt>Total</dt><dd>{duration(answer.metrics.total_ms)}</dd></div>
      <div>
        <dt>t/s</dt>
        <dd>
          {number(answer.metrics.tok_per_sec)}
          {#if answer.metrics.tok_per_sec === null && answer.metrics.cli_startup_ms !== null && tools.calls > 0}
            <span class="hint" role="img" aria-label={BUFFERED_HINT} title={BUFFERED_HINT}><Icon name="info" size={14} /></span>
          {/if}
        </dd>
      </div>
      <!-- No threshold on purpose: a near-zero startup is a result being compared, not noise. -->
      {#if startup !== null}
        <div><dt title={STARTUP_HINT}>Démarrage</dt><dd>{duration(startup)}</dd></div>
      {/if}
      {#if tools.calls > 0}
        <div><dt>Outils</dt><dd>{tools.calls} {tools.calls > 1 ? 'appels' : 'appel'} · {duration(tools.ms)}</dd></div>
      {/if}
    </dl>
  {/if}
</div>

<style>
  .answer {
    display: flex;
    flex-direction: column;
    gap: var(--s3);
    min-width: 0;
    padding: var(--s4);
    border: 1px solid var(--hairline);
    border-radius: var(--radius);
    background: var(--panel);
  }

  .text {
    overflow-wrap: anywhere;
    white-space: pre-wrap;
  }

  .musing {
    font-size: var(--text-sm);
  }

  .live {
    color: var(--accent);
  }

  .dots span {
    animation: dot 1.2s ease-in-out infinite both;
  }

  .dots span:nth-child(2) {
    animation-delay: 0.2s;
  }

  .dots span:nth-child(3) {
    animation-delay: 0.4s;
  }

  @keyframes dot {
    0%,
    60%,
    100% {
      opacity: 0.2;
    }

    30% {
      opacity: 1;
    }
  }

  .metrics {
    display: flex;
    flex-wrap: wrap;
    gap: var(--s2) var(--s5);
    margin: auto 0 0;
    padding-top: var(--s3);
    border-top: 1px solid var(--hairline);
  }

  dt {
    color: var(--ink-3);
    font-size: var(--text-sm);
  }

  dd {
    margin: 0;
    font-weight: 600;
    font-variant-numeric: tabular-nums;
  }

  .hint {
    display: inline-flex;
    vertical-align: -2px;
    color: var(--ink-3);
    cursor: help;
  }
</style>
