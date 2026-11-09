<script lang="ts">
  import Icon from '../Icon.svelte';
  import ModelName from '../ModelName.svelte';
  import { UNMEASURED_HINT, duration, number, score, seriesColor, unmeasuredLabel } from '../../lib/format';
  import type { RankKind, RankRow, RankSection } from '../../lib/map/ranking';

  let {
    sections,
    heading,
    models,
    onpick,
  }: { sections: RankSection[]; heading: string; models: string[]; onpick: (kind: RankKind, key: string) => void } = $props();

  /**
   * @param {RankRow} row - ranked row
   * @returns {string} - gauge color: a model keeps its series color everywhere
   */
  function gaugeColor(row: RankRow): string {
    return row.kind === 'model' ? seriesColor(row.key, models) : 'var(--accent)';
  }
</script>

<aside class="ranking glass" aria-label="Classement">
  <h2>{heading}</h2>
  {#each sections as section (section.kind)}
    <section aria-label={section.title}>
      {#if sections.length > 1}<h3>{section.title}</h3>{/if}
      <ol>
        {#each section.rows as row (row.key)}
          <li>
            <button type="button" class="row" class:picked={row.picked} onclick={() => onpick(row.kind, row.key)}>
              <span class="top">
                <span class="rank">{row.rank}</span>
                <span class="name">
                  {#if row.kind === 'model'}<ModelName name={row.key} />{:else}{row.label}{/if}
                </span>
                <strong class="value">{score(row.score)}{#if row.score !== null}<small>/100</small>{/if}</strong>
              </span>
              <span class="gauge" role="meter" aria-valuemin="0" aria-valuemax="100" aria-valuenow={row.score} aria-label="Score">
                <span style:width="{row.score ?? 0}%" style:background={gaugeColor(row)}></span>
              </span>
              <span class="gap">
                {#if row.tied}<span class="tie">ex aequo</span>{/if}
                {#if row.gap}<span>{row.gap}</span>{/if}
              </span>
              <span class="tiles">
                {#if row.ms !== null}<span class="tile"><small>Temps médian</small>{duration(row.ms)}</span>{/if}
                {#if row.p95 !== null}<span class="tile"><small>95 % des réponses sous</small>{duration(row.p95)}</span>{/if}
                {#if row.tokPerSec !== null}<span class="tile"><small>Vitesse</small>{number(row.tokPerSec)} t/s</span>{/if}
                <span class="tile"><small>Réponses</small>{row.runs}</span>
                <span class="tile"><small>Questions</small>{row.questions}</span>
              </span>
              {#if row.questions < row.coverage}
                <span class="state state-warning"><Icon name="alert" size={14} /> {row.questions}/{row.coverage} questions couvertes</span>
              {/if}
              {#if row.errors > 0}
                <span class="state muted" title={UNMEASURED_HINT}>{unmeasuredLabel(row.errors)}</span>
              {/if}
            </button>
          </li>
        {:else}
          <li class="muted none">Aucune réponse pour cette sélection.</li>
        {/each}
      </ol>
    </section>
  {/each}
  <p class="muted note">Score moyen par question, sur toute la base. Écart mesuré au premier.</p>
</aside>

<style>
  .ranking {
    display: flex;
    flex-direction: column;
    gap: var(--s3);
    min-height: 0;
    overflow-y: auto;
    padding: var(--s4);
    border-radius: var(--radius-lg);
    pointer-events: auto;
    scrollbar-color: var(--axis) transparent;
  }

  h3 {
    margin-bottom: var(--s2);
    color: var(--ink-2);
    font-size: var(--text-sm);
    font-weight: 600;
  }

  /* Figures sit on a solid ground: glass stays around them, never behind them. */
  ol {
    display: grid;
    margin: 0;
    padding: 0;
    overflow: hidden;
    border: 1px solid var(--hairline);
    border-radius: var(--radius);
    background: var(--panel);
    list-style: none;
  }

  li + li {
    border-top: 1px solid var(--hairline);
  }

  .row {
    display: grid;
    gap: var(--s1);
    width: 100%;
    padding: var(--s2) var(--s3);
    border: 0;
    background: transparent;
    text-align: left;
    cursor: pointer;
    transition: background-color 0.15s;
  }

  .row:hover {
    background: var(--raised);
  }

  .row:focus-visible {
    outline-offset: -2px;
  }

  .row.picked {
    background: color-mix(in srgb, var(--accent) 16%, var(--panel));
  }

  .top {
    display: flex;
    align-items: baseline;
    gap: var(--s2);
  }

  .rank {
    color: var(--ink-3);
    font-size: var(--text-sm);
    font-variant-numeric: tabular-nums;
  }

  .name {
    flex: 1;
    min-width: 0;
    overflow-wrap: anywhere;
  }

  .value {
    font-size: var(--text-lg);
    font-variant-numeric: tabular-nums;
    font-weight: 650;
  }

  .value small {
    color: var(--ink-3);
    font-size: var(--text-sm);
    font-weight: 500;
  }

  /* Segments read as a scale of ten without any extra label. */
  .gauge {
    display: block;
    height: 6px;
    overflow: hidden;
    background: var(--hairline);
    mask: repeating-linear-gradient(to right, #000 0 calc(10% - 2px), transparent calc(10% - 2px) 10%);
  }

  .gauge > span {
    display: block;
    height: 100%;
    transition: width 0.45s var(--ease-out);
  }

  .gap {
    display: flex;
    flex-wrap: wrap;
    gap: 0 var(--s2);
    min-height: 0;
    color: var(--ink);
    font-size: var(--text-sm);
    font-weight: 550;
  }

  .tie {
    color: var(--accent);
  }

  /* Times and speed take the room their units need, the plain counts share the rest. */
  .tiles {
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: var(--s1);
    font-size: var(--text-sm);
    font-variant-numeric: tabular-nums;
    font-weight: 550;
  }

  .tile {
    display: grid;
    grid-column: span 2;
    gap: 1px;
    padding: 3px var(--s2);
    border-radius: var(--radius-sm);
    background: color-mix(in srgb, var(--ground) 45%, transparent);
  }

  .tile:nth-last-child(-n + 2) {
    grid-column: span 1;
  }

  .tile small {
    color: var(--ink-3);
    font-size: 0.6875rem;
    font-weight: 400;
    line-height: 1.25;
  }

  .none {
    padding: var(--s3);
    font-size: var(--text-sm);
  }

  .note {
    font-size: var(--text-sm);
  }
</style>
