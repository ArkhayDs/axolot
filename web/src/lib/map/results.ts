// Results per question: the same four states as the answer matrix, from the cumulated figures or a live answer.

import type { MapItem } from '../api';

export type ResultState = 'passed' | 'partial' | 'failed' | 'none';

export type ItemIndex = Map<string, MapItem>;

export const RESULT_LABELS: Record<ResultState, string> = { passed: 'Réussie', partial: 'Partielle', failed: 'Échouée', none: 'Pas de réponse' };

/**
 * @param {string} machine - machine label, empty for all
 * @param {string} model - model tag
 * @param {string} caseId - scenario id
 * @param {string} itemId - question id
 * @returns {string} - lookup key
 */
function itemKey(machine: string, model: string, caseId: string, itemId: string): string {
  return `${machine}|${model}|${caseId}|${itemId}`;
}

/**
 * @param {MapItem[]} items - figures sent by the API
 * @returns {ItemIndex} - figures by machine, model and question
 */
export function indexItems(items: MapItem[]): ItemIndex {
  return new Map(items.map((item) => [itemKey(item.machine, item.model, item.case_id, item.item_id), item]));
}

/**
 * @param {ItemIndex} index - indexed figures
 * @param {string} machine - machine label, empty for all
 * @param {string} model - model tag
 * @param {string} caseId - scenario id
 * @param {string} itemId - question id
 * @returns {MapItem | undefined} - figures, absent when never answered
 */
export function itemOf(index: ItemIndex, machine: string, model: string, caseId: string, itemId: string): MapItem | undefined {
  return index.get(itemKey(machine, model, caseId, itemId));
}

/**
 * Same thresholds as the matrix; 99.95 reads 100 once displayed and absorbs the rounding of the weighted mean
 * @param {number} score - score out of 100
 * @param {boolean} failed - whether nothing usable came back
 * @returns {ResultState} - state of the answer
 */
export function stateOfScore(score: number, failed: boolean): ResultState {
  if (failed || score <= 0) return 'failed';
  return score >= 99.95 ? 'passed' : 'partial';
}

/**
 * @param {MapItem | undefined} item - cumulated figures
 * @returns {ResultState} - state of the question for that model
 */
export function stateOfItem(item: MapItem | undefined): ResultState {
  return item ? stateOfScore(item.avg_score, item.errors === item.runs) : 'none';
}
