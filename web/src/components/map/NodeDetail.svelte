<script lang="ts">
  import Icon from '../Icon.svelte';
  import ModelName from '../ModelName.svelte';
  import ResultMark from './ResultMark.svelte';
  import type { MapBenchmark } from '../../lib/api';
  import { UNMEASURED_HINT, benchmarkName, categoryLabel, duration, score, statusLabel, trapLabel, unmeasuredLabel, when } from '../../lib/format';
  import type { MapContent } from '../../lib/map/content';
  import type { NodeKind } from '../../lib/map/graph';
  import { benchmarkScore } from '../../lib/map/ranking';
  import type { MapFilter } from '../../lib/map/ranking';
  import { RESULT_LABELS, itemOf, stateOfItem } from '../../lib/map/results';
  import type { ItemIndex } from '../../lib/map/results';

  let {
    focus,
    content,
    items,
    filter,
    onclose,
  }: { focus: { kind: NodeKind; key: string }; content: MapContent; items: ItemIndex; filter: MapFilter; onclose: () => void } = $props();

  const question = $derived(focus.kind === 'question' ? content.questions.find((candidate) => candidate.key === focus.key) : undefined);
  const scenario = $derived(content.scenarios.find((candidate) => candidate.id === (question?.caseId ?? focus.key)));
  const benchmark = $derived<MapBenchmark | undefined>(focus.kind === 'benchmark' ? content.benchmarks.find((candidate) => candidate.id === focus.key) : undefined);
  const citing = $derived(focus.kind === 'document' ? (content.documents.get(focus.key) ?? []) : []);
  const answers = $derived(
    question
      ? content.models
          .map((model) => ({ model, item: itemOf(items, filter.machine, model, question.caseId, question.item.id) }))
          .sort((a, b) => (b.item?.avg_score ?? -1) - (a.item?.avg_score ?? -1))
      : [],
  );
  const sources = $derived([...new Set((question?.item.rag?.sources ?? []).map((source) => source.file))]);
  const title = $derived(question ? question.item.id : benchmark ? benchmarkName(benchmark.name) : focus.kind === 'scenario' ? (scenario?.title ?? focus.key) : focus.key);
</script>

<aside class="detail glass" aria-label="Détail de la sélection">
  <div class="head">
    <h2>{title}</h2>
    <button class="close" type="button" aria-label="Fermer le détail" onclick={onclose}><Icon name="cross" size={16} /></button>
  </div>

  {#if question}
    <p class="meta"><a href="#/cas/{encodeURIComponent(question.caseId)}">{scenario?.title}</a>, {categoryLabel(question.category)}</p>
    <p class="prompt">{question.item.prompt}</p>
    <ul class="answers" aria-label="Réponses par modèle">
      {#each answers as { model, item } (model)}
        {@const state = stateOfItem(item)}
        <li>
          <span class="who"><ResultMark {state} /><ModelName name={model} /></span>
          <span class="figures">
            <strong>{item ? `${score(item.avg_score)}/100` : '-'}</strong>
            <span>{RESULT_LABELS[state]}</span>
            {#if item}<span>{duration(item.p50_total_ms)} médian, {item.runs} réponse{item.runs > 1 ? 's' : ''}</span>{/if}
            {#if item && item.errors > 0}<span title={UNMEASURED_HINT}>{unmeasuredLabel(item.errors)}</span>{/if}
          </span>
        </li>
      {/each}
    </ul>
    {#if question.item.rag}
      <p class="meta">{trapLabel(question.item.rag.trap)}{sources.length > 0 ? ` : ${sources.join(', ')}` : ''}</p>
    {/if}
    <a class="more" href="#/cas/{encodeURIComponent(question.caseId)}"><Icon name="arrow" size={15} /> Ouvrir le scénario</a>
  {:else if focus.kind === 'scenario' && scenario}
    <p class="meta">{categoryLabel(scenario.category)}, {scenario.items.length} question{scenario.items.length > 1 ? 's' : ''}</p>
    <a class="more" href="#/cas/{encodeURIComponent(scenario.id)}"><Icon name="arrow" size={15} /> Ouvrir le scénario</a>
  {:else if focus.kind === 'document'}
    <p class="meta">Source de {citing.length} question{citing.length > 1 ? 's' : ''}</p>
  {:else if benchmark}
    {@const value = benchmarkScore(benchmark, filter)}
    <p class="meta">{when(benchmark.created_at)}, {benchmark.machine}, {statusLabel(benchmark.status)}</p>
    <p class="meta"><strong>{value === null ? '-' : `${score(value)}/100`}</strong>, {benchmark.done_items} réponses</p>
    <a class="more" href="#/benchmarks/{benchmark.id}"><Icon name="arrow" size={15} /> Ouvrir le Benchmark</a>
  {/if}
</aside>

<style>
  .detail {
    display: grid;
    flex: none;
    gap: var(--s2);
    max-height: 70%;
    margin-bottom: var(--s2);
    overflow-y: auto;
    padding: var(--s3) var(--s4);
    border-radius: var(--radius-lg);
    scrollbar-color: var(--axis) transparent;
  }

  .head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--s2);
  }

  h2 {
    font-size: var(--text-md);
    overflow-wrap: anywhere;
  }

  .close {
    flex: none;
    display: inline-flex;
    padding: 2px;
    border: 0;
    background: transparent;
    color: var(--ink-2);
    cursor: pointer;
  }

  .meta,
  .prompt {
    font-size: var(--text-sm);
  }

  .meta {
    color: var(--ink-2);
    overflow-wrap: anywhere;
  }

  .prompt {
    display: -webkit-box;
    overflow: hidden;
    -webkit-box-orient: vertical;
    -webkit-line-clamp: 4;
    line-clamp: 4;
  }

  /* Figures sit on a solid ground: glass stays around them, never behind them. */
  .answers {
    display: grid;
    margin: 0;
    padding: 0;
    border: 1px solid var(--hairline);
    border-radius: var(--radius);
    background: var(--panel);
    list-style: none;
  }

  .answers li {
    display: grid;
    gap: 2px;
    padding: var(--s2) var(--s3);
  }

  .answers li + li {
    border-top: 1px solid var(--hairline);
  }

  .who {
    display: flex;
    align-items: center;
    gap: var(--s2);
    min-width: 0;
  }

  .figures {
    display: flex;
    flex-wrap: wrap;
    gap: 0 var(--s3);
    color: var(--ink-2);
    font-size: var(--text-sm);
    font-variant-numeric: tabular-nums;
  }

  .figures strong {
    color: var(--ink);
    font-weight: 650;
  }

  .more {
    display: inline-flex;
    align-items: center;
    gap: var(--s1);
    font-size: var(--text-sm);
  }
</style>
