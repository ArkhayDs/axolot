<script lang="ts">
  import ModelName from './ModelName.svelte';
  import type { Case, Run } from '../lib/api';
  import { score } from '../lib/format';

  let {
    cases,
    models,
    runs,
    activeItem = '',
    activeModel = '',
  }: { cases: Case[]; models: string[]; runs: Run[]; activeItem?: string; activeModel?: string } = $props();

  const byCell = $derived(new Map(runs.map((run) => [`${run.model}|${run.case_id}|${run.item_id}`, run])));

  /**
   * @param {Run | undefined} run - answer, absent when not played yet
   * @returns {string} - cell state: the shape carries it as well as the color
   */
  function cellState(run: Run | undefined): string {
    if (!run) return 'pending';
    if (run.error || run.score === 0) return 'failed';
    return run.score >= 100 ? 'passed' : 'partial';
  }

  /**
   * @param {string} itemId - question id
   * @param {string} model - model tag
   * @param {Run | undefined} run - answer
   * @returns {string} - native tooltip
   */
  function cellTitle(itemId: string, model: string, run: Run | undefined): string {
    if (!run) return `${itemId}, ${model} : pas de réponse`;
    return `${itemId}, ${model} : ${run.error ? 'échec' : `${score(run.score)}/100`}`;
  }
</script>

<figure class="matrix panel">
  <figcaption>
    <h3>Matrice des réponses</h3>
    <ul class="legend" aria-label="Légende">
      <li><span class="cell passed"></span> Réussie</li>
      <li><span class="cell partial"></span> Partielle</li>
      <li><span class="cell failed"></span> Échouée</li>
      <li><span class="cell pending"></span> Pas de réponse</li>
    </ul>
  </figcaption>
  <div class="scroll">
    <table>
      <thead>
        <tr>
          <th scope="col">Scénario</th>
          {#each models as model (model)}
            <th scope="col"><ModelName name={model} /></th>
          {/each}
        </tr>
      </thead>
      <tbody>
        {#each cases as item (item.id)}
          <tr>
            <th scope="row"><a href="#/cas/{encodeURIComponent(item.id)}">{item.title}</a></th>
            {#each models as model (model)}
              <td>
                <span class="cells">
                  {#each item.items as question (question.id)}
                    {@const run = byCell.get(`${model}|${item.id}|${question.id}`)}
                    <span
                      class="cell {cellState(run)}"
                      class:active={activeModel === model && activeItem === question.id}
                      title={cellTitle(question.id, model, run)}
                    ></span>
                  {/each}
                </span>
              </td>
            {/each}
          </tr>
        {/each}
      </tbody>
    </table>
  </div>
</figure>

<style>
  .matrix {
    display: grid;
    gap: var(--s3);
    margin: 0;
    min-width: 0;
  }

  figcaption {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: var(--s2) var(--s4);
  }

  .legend {
    display: flex;
    flex-wrap: wrap;
    gap: var(--s1) var(--s3);
    margin: 0;
    padding: 0;
    color: var(--ink-2);
    font-size: var(--text-sm);
    list-style: none;
  }

  .legend li {
    display: inline-flex;
    align-items: center;
    gap: var(--s1);
  }

  .scroll {
    overflow-x: auto;
  }

  th,
  td {
    padding: 3px var(--s3) 3px 0;
    vertical-align: middle;
    white-space: nowrap;
  }

  tbody th {
    color: var(--ink-2);
    font-size: var(--text-sm);
    font-weight: 400;
  }

  tbody a {
    text-decoration: none;
  }

  tbody a:hover {
    color: var(--accent);
  }

  .cells {
    display: flex;
    gap: 2px;
  }

  .cell {
    flex: none;
    display: inline-block;
    width: 13px;
    height: 13px;
    border-radius: 3px;
    border: 1px solid transparent;
  }

  .passed {
    background: var(--good);
  }

  /* Half filled, so a partial score reads without color. */
  .partial {
    border-color: var(--warning);
    background: linear-gradient(to top, var(--warning) 50%, transparent 50%);
  }

  .failed {
    border: 2px solid var(--critical);
  }

  .pending {
    border-color: var(--axis);
  }

  .active {
    border-color: var(--accent);
    background: var(--accent);
    animation: cell-pulse 0.9s ease-in-out infinite alternate;
  }

  @keyframes cell-pulse {
    to {
      opacity: 0.35;
    }
  }
</style>
