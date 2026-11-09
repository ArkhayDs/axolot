// Every node of the Atlas as a line of the keyboard paths: search field and counter lists, with the figure that helps choosing.

import { benchmarkName, categoryLabel, score } from '../format';
import type { MapContent } from './content';
import type { NodeKind } from './graph';
import { benchmarkScore, cellOf } from './ranking';
import type { CellIndex, MapFilter } from './ranking';
import { itemOf } from './results';
import type { ItemIndex } from './results';

export interface Findable {
  kind: NodeKind;
  key: string;
  label: string;
  type: string;
  note: string;
}

/**
 * @param {number} count - quantity
 * @param {string} unit - singular unit
 * @returns {string} - quantity with its unit
 */
function counted(count: number, unit: string): string {
  return `${count} ${unit}${count > 1 ? 's' : ''}`;
}

/**
 * @param {number | undefined} value - score
 * @returns {string} - score out of 100, a dash when never played
 */
function outOf100(value: number | null | undefined): string {
  return value === undefined || value === null ? '-' : `${score(value)}/100`;
}

/**
 * @param {MapContent} content - what the map holds
 * @param {CellIndex} cells - figures per theme
 * @param {ItemIndex} items - figures per question
 * @param {MapFilter} filter - current selection, the figures follow it
 * @returns {Findable[]} - every node, by type
 */
export function findableOf(content: MapContent, cells: CellIndex, items: ItemIndex, filter: MapFilter): Findable[] {
  const { machine, model, category } = filter;
  return [
    ...content.machines.map((key): Findable => ({
      kind: 'machine',
      key,
      label: key,
      type: 'Machine',
      note: model ? outOf100(cellOf(cells, key, model, category)?.avg_score) : counted(content.models.filter((candidate) => cellOf(cells, key, candidate, category)).length, 'modèle'),
    })),
    ...content.models.map((key): Findable => ({ kind: 'model', key, label: key, type: 'Modèle', note: outOf100(cellOf(cells, machine, key, category)?.avg_score) })),
    ...content.themes.map((key): Findable => ({
      kind: 'theme',
      key,
      label: categoryLabel(key),
      type: 'Thème',
      note: model ? outOf100(cellOf(cells, machine, model, key)?.avg_score) : counted(content.questions.filter((question) => question.category === key).length, 'question'),
    })),
    ...content.scenarios.map((scenario): Findable => ({ kind: 'scenario', key: scenario.id, label: scenario.title, type: 'Scénario', note: counted(scenario.items.length, 'question') })),
    ...content.questions.map((question): Findable => ({
      kind: 'question',
      key: question.key,
      label: question.item.id,
      type: 'Question',
      note: model ? outOf100(itemOf(items, machine, model, question.caseId, question.item.id)?.avg_score) : categoryLabel(question.category),
    })),
    ...[...content.documents].map(([key, questions]): Findable => ({ kind: 'document', key, label: key, type: 'Document', note: counted(questions.length, 'question') })),
    ...content.benchmarks.map((benchmark): Findable => ({
      kind: 'benchmark',
      key: benchmark.id,
      label: benchmarkName(benchmark.name),
      type: 'Benchmark',
      note: outOf100(benchmarkScore(benchmark, filter) ?? undefined),
    })),
  ];
}
