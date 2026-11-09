// Pure figures of the map: cell lookups, rankings with explicit ties, and gaps to the leader.

import type { MapBenchmark, MapCell, MapData } from '../api';
import { categoryLabel, number, score as formatScore } from '../format';

export interface MapFilter {
  machine: string;
  model: string;
  category: string;
}

export type RankKind = 'model' | 'theme' | 'machine';

export type RankFigures = Pick<MapCell, 'avg_score' | 'runs' | 'questions' | 'errors' | 'p50_total_ms' | 'p95_total_ms' | 'p50_tok_per_sec'>;

export interface RankRow {
  kind: RankKind;
  key: string;
  label: string;
  rank: number;
  tied: boolean;
  score: number | null;
  runs: number;
  questions: number;
  // Most questions any row of the ranking covers: below it, the scores do not compare the same work.
  coverage: number;
  errors: number;
  ms: number | null;
  p95: number | null;
  tokPerSec: number | null;
  gap: string;
  picked: boolean;
}

export interface RankSection {
  kind: RankKind;
  title: string;
  rows: RankRow[];
}

export interface MapLists {
  machines: string[];
  models: string[];
  themes: string[];
}

export type CellIndex = Map<string, MapCell>;

/**
 * @param {string} machine - machine label, empty for all
 * @param {string} model - model tag
 * @param {string} category - theme, empty for all
 * @returns {string} - cell lookup key
 */
function cellKey(machine: string, model: string, category: string): string {
  return `${machine}|${model}|${category}`;
}

/**
 * @param {MapCell[]} cells - cells and rollups sent by the API
 * @returns {CellIndex} - cells by machine, model and theme
 */
export function indexCells(cells: MapCell[]): CellIndex {
  return new Map(cells.map((cell) => [cellKey(cell.machine, cell.model, cell.category), cell]));
}

/**
 * @param {CellIndex} index - indexed cells
 * @param {string} machine - machine label, empty for all
 * @param {string} model - model tag
 * @param {string} category - theme, empty for all
 * @returns {MapCell | undefined} - figures, absent when never played
 */
export function cellOf(index: CellIndex, machine: string, model: string, category: string): MapCell | undefined {
  return index.get(cellKey(machine, model, category));
}

/**
 * @param {MapData} data - API answer
 * @returns {MapLists} - sorted axes; the current machine is listed even without data
 */
export function listsOf(data: MapData): MapLists {
  const axis = (pick: (cell: MapCell) => string): string[] => [...new Set(data.cells.map(pick).filter(Boolean))].sort();
  return {
    machines: [...new Set([...axis((cell) => cell.machine), data.current_machine])].filter(Boolean).sort(),
    models: axis((cell) => cell.model),
    themes: axis((cell) => cell.category),
  };
}

/**
 * @param {string} model - model tag
 * @param {string[]} models - every model of the view
 * @returns {string} - family alone while it still identifies the model
 */
export function shortModel(model: string, models: string[]): string {
  const family = model.split(':')[0];
  return models.filter((other) => other.split(':')[0] === family).length > 1 ? model : family;
}

/**
 * Two scores shown with the same figure tie: a rank must never rest on a digit nobody can see
 * @param {number | null} a - score
 * @param {number | null} b - score
 * @returns {boolean} - same displayed score, never for a missing one
 */
export function sameScore(a: number | null, b: number | null): boolean {
  return a !== null && b !== null && formatScore(a) === formatScore(b);
}

/**
 * @param {RankRow} row - ranked row
 * @param {RankRow} leader - first row
 * @returns {string} - gap to the leader, in points and speed; none without score, whose timings are errors
 */
function gapLabel(row: RankRow, leader: RankRow): string {
  if (row.score === null) return '';
  if (row === leader) return row.tied ? '' : 'En tête';
  const parts: string[] = [];
  if (leader.score !== null && !sameScore(row.score, leader.score)) {
    const gap = leader.score - row.score;
    parts.push(`${formatScore(gap)} point${gap >= 2 ? 's' : ''} de moins`);
  }
  if (row.ms && leader.ms) {
    const ratio = leader.ms / row.ms;
    if (ratio >= 1.05) parts.push(`${number(ratio)} fois plus rapide`);
    else if (ratio <= 0.95) parts.push(`${number(1 / ratio)} fois plus lent`);
  }
  return parts.join(', ');
}

/**
 * Best score first, rows without score last; on a tie the faster row is listed first but shares the rank
 * @param {RankKind} kind - what is ranked
 * @param {{ key: string; label: string; cell: RankFigures | undefined }[]} entries - candidates, absent figures dropped
 * @param {string} picked - key of the selected row
 * @returns {RankRow[]} - ranked rows
 */
export function rankRows(kind: RankKind, entries: { key: string; label: string; cell: RankFigures | undefined }[], picked: string): RankRow[] {
  const rows = entries.flatMap(({ key, label, cell }): RankRow[] =>
    cell
      ? [{
          kind,
          key,
          label,
          rank: 0,
          tied: false,
          score: cell.avg_score,
          runs: cell.runs,
          questions: cell.questions,
          coverage: 0,
          errors: cell.errors,
          ms: cell.p50_total_ms,
          p95: cell.p95_total_ms,
          tokPerSec: cell.p50_tok_per_sec,
          gap: '',
          picked: key === picked,
        }]
      : [],
  );
  rows.sort((a, b) => (sameScore(a.score, b.score) ? (a.ms ?? Infinity) - (b.ms ?? Infinity) : (b.score ?? -1) - (a.score ?? -1)));
  rows.forEach((row, position) => {
    const previous = rows[position - 1];
    const next = rows[position + 1];
    row.rank = row.score === null ? 0 : previous && sameScore(previous.score, row.score) ? previous.rank : position + 1;
    row.tied = Boolean((previous && sameScore(previous.score, row.score)) || (next && sameScore(next.score, row.score)));
  });
  // Themes hold different question counts by nature: only models and machines answer the same list.
  const coverage = kind === 'theme' ? 0 : Math.max(0, ...rows.map((row) => row.questions));
  rows.forEach((row) => {
    row.gap = gapLabel(row, rows[0]);
    row.coverage = coverage;
  });
  return rows;
}

/**
 * Picks what the filters ask to compare: models by default, a model's themes and machines once it is chosen
 * @param {CellIndex} index - indexed cells
 * @param {MapLists} lists - axes
 * @param {MapFilter} filter - current selection
 * @returns {RankSection[]} - rankings to display
 */
export function rankSections(index: CellIndex, lists: MapLists, filter: MapFilter): RankSection[] {
  const { machine, model, category } = filter;
  const sections: RankSection[] = [];
  if (!model || category) {
    const entries = lists.models.map((key) => ({ key, label: key, cell: cellOf(index, machine, key, category) }));
    sections.push({ kind: 'model', title: 'Modèles', rows: rankRows('model', entries, model) });
  }
  if (model && !category) {
    const entries = lists.themes.map((key) => ({ key, label: categoryLabel(key), cell: cellOf(index, machine, model, key) }));
    sections.push({ kind: 'theme', title: 'Thèmes', rows: rankRows('theme', entries, '') });
  }
  if (model && !machine) {
    const entries = lists.machines.map((key) => ({ key, label: key, cell: cellOf(index, key, model, category) }));
    sections.push({ kind: 'machine', title: 'Machines', rows: rankRows('machine', entries, '') });
  }
  return sections;
}

/**
 * @param {MapBenchmark} benchmark - past benchmark
 * @param {MapFilter} filter - current selection
 * @returns {number | null} - score over the measured answers the selection covers, null when there are none
 */
export function benchmarkScore(benchmark: MapBenchmark, filter: MapFilter): number | null {
  if (filter.machine && benchmark.machine !== filter.machine) return null;
  const cells = benchmark.cells.filter(
    (cell) => (!filter.model || cell.model === filter.model) && (!filter.category || cell.category === filter.category),
  );
  const measured = cells.reduce((total, cell) => total + cell.measured, 0);
  return measured === 0 ? null : cells.reduce((total, cell) => total + (cell.avg_score ?? 0) * cell.measured, 0) / measured;
}
