<script lang="ts">
  import type { ModelStats } from '../lib/api';
  import { categoryLabel, score, seriesColor } from '../lib/format';

  let { stats, models }: { stats: ModelStats[]; models: string[] } = $props();

  const ROW_HEIGHT = 44;
  const LEFT_MARGIN = 78;
  const RIGHT_MARGIN = 164;
  const MAX_STROKE = 7;

  let width = $state(0);
  let focus = $state('');

  const tasks = $derived([...new Set(stats.map((stat) => stat.category).filter(Boolean))]);
  const height = $derived(Math.max(280, tasks.length * ROW_HEIGHT + 24));
  const leftX = LEFT_MARGIN;
  const rightX = $derived(Math.max(leftX + 80, width - RIGHT_MARGIN));

  /**
   * @param {number} index - node position
   * @param {number} count - nodes in the column
   * @returns {number} - vertical center
   */
  function nodeY(index: number, count: number): number {
    return (height * (index + 0.5)) / Math.max(count, 1);
  }

  const edges = $derived(
    stats
      .filter((stat) => stat.category !== '')
      .map((stat) => {
        const y1 = nodeY(models.indexOf(stat.model), models.length);
        const y2 = nodeY(tasks.indexOf(stat.category), tasks.length);
        const middle = (leftX + rightX) / 2;
        return {
          key: `${stat.model}|${stat.category}`,
          model: stat.model,
          category: stat.category,
          value: stat.avg_score,
          path: `M${leftX} ${y1} C${middle} ${y1} ${middle} ${y2} ${rightX} ${y2}`,
        };
      }),
  );
  const focusEdge = $derived(edges.find((edge) => edge.key === focus));

  /**
   * @param {string} model - edge's model
   * @param {string} category - edge's task
   * @returns {boolean} - whether the edge is dimmed by the current hover
   */
  function dimmed(model: string, category: string): boolean {
    return focus !== '' && focus !== model && focus !== category && focus !== `${model}|${category}`;
  }
</script>

<figure class="network panel" bind:clientWidth={width}>
  <figcaption>
    <h2>Réseau des scores</h2>
    <p class="muted">Épaisseur du lien : score du modèle sur la tâche.</p>
  </figcaption>
  {#if width > 0}
    <svg {width} {height} role="img" aria-label="Score de chaque modèle par tâche, repris dans le tableau de résultats">
      {#each edges as edge, index (edge.key)}
        <path
          class="edge"
          class:dim={dimmed(edge.model, edge.category)}
          d={edge.path}
          pathLength="1"
          stroke={seriesColor(edge.model, models)}
          stroke-width={Math.max(1, (edge.value / 100) * MAX_STROKE)}
          style:animation-delay="{index * 30}ms"
        />
        <!-- Wider invisible stroke: a thin low-score edge must stay easy to hover. -->
        <path
          class="hit"
          role="presentation"
          d={edge.path}
          onpointerenter={() => (focus = edge.key)}
          onpointerleave={() => (focus = '')}
        />
      {/each}

      {#each models as model, index (model)}
        {@const y = nodeY(index, models.length)}
        <g role="presentation" onpointerenter={() => (focus = model)} onpointerleave={() => (focus = '')}>
          <circle class="halo" cx={leftX} cy={y} r="15" fill={seriesColor(model, models)} />
          <circle cx={leftX} cy={y} r="9" fill={seriesColor(model, models)} />
          <text class="model-label" x={leftX - 22} y={y + 4} text-anchor="end">{model.split(':')[0]}</text>
        </g>
      {/each}

      {#each tasks as task, index (task)}
        {@const y = nodeY(index, tasks.length)}
        <g role="presentation" onpointerenter={() => (focus = task)} onpointerleave={() => (focus = '')}>
          <circle class="task-node" cx={rightX} cy={y} r="6" />
          <text class="task-label" x={rightX + 14} y={y + 4}>{categoryLabel(task)}</text>
        </g>
      {/each}
    </svg>
    <p class="readout" aria-live="polite">
      {#if focusEdge}
        {focusEdge.model.split(':')[0]} sur {categoryLabel(focusEdge.category).toLowerCase()} : <strong>{score(focusEdge.value)}/100</strong>
      {:else}
        <span class="muted">Survolez un lien pour lire son score.</span>
      {/if}
    </p>
  {/if}
</figure>

<style>
  .network {
    display: grid;
    align-content: start;
    gap: var(--s3);
    margin: 0;
    min-width: 0;
    padding: var(--s4);
  }

  figcaption p,
  .readout {
    margin-top: var(--s1);
    font-size: var(--text-sm);
  }

  svg {
    display: block;
    overflow: visible;
  }

  .edge {
    fill: none;
    stroke-linecap: round;
    stroke-opacity: 0.7;
    stroke-dasharray: 1;
    transition: stroke-opacity 0.15s;
    animation: edge-draw 1.1s var(--ease-out) backwards;
  }

  .edge.dim {
    stroke-opacity: 0.08;
  }

  .hit {
    fill: none;
    stroke: transparent;
    stroke-width: 14;
  }

  .halo {
    fill-opacity: 0.22;
  }

  .task-node {
    fill: var(--ground);
    stroke: var(--ink-3);
    stroke-width: 2;
  }

  .model-label {
    fill: var(--ink);
    font-family: var(--mono);
    font-size: 12px;
    font-weight: 600;
  }

  .task-label {
    fill: var(--ink-2);
    font-size: 12px;
  }

  @keyframes edge-draw {
    from {
      stroke-dashoffset: 1;
    }
    to {
      stroke-dashoffset: 0;
    }
  }
</style>
