// What each node and link says under the current selection: kept or dimmed, score, label, result.

import type { MapBenchmark } from '../api';
import { duration, score as formatScore } from '../format';
import { questionKept } from './content';
import type { MapContent } from './content';
import type { Graph, GraphLink, GraphNode } from './graph';
import { benchmarkScore, cellOf, sameScore, shortModel } from './ranking';
import type { CellIndex, MapFilter, RankSection } from './ranking';
import { itemOf, stateOfItem } from './results';
import type { ItemIndex, ResultState } from './results';

/**
 * @param {GraphNode} node - node
 * @param {CellIndex} cells - figures per theme
 * @param {MapContent} content - what the map holds
 * @param {MapFilter} filter - current selection
 * @returns {boolean} - whether the selection keeps this node
 */
export function nodeMatches(node: GraphNode, cells: CellIndex, content: MapContent, filter: MapFilter): boolean {
  const models = filter.model ? [filter.model] : content.models;
  switch (node.kind) {
    case 'model':
      return (!filter.model || node.key === filter.model) && Boolean(cellOf(cells, filter.machine, node.key, filter.category));
    case 'theme':
      return (!filter.category || node.key === filter.category) && models.some((model) => cellOf(cells, filter.machine, model, node.key));
    case 'machine':
      return (!filter.machine || node.key === filter.machine) && models.some((model) => cellOf(cells, node.key, model, filter.category));
    case 'benchmark': {
      const benchmark = content.benchmarks.find((candidate) => candidate.id === node.key) as MapBenchmark;
      return benchmarkScore(benchmark, filter) !== null;
    }
    default:
      // An unanswered question stays visible: under a model, that is exactly what the point says.
      return !filter.category || node.community === filter.category;
  }
}

/**
 * @param {GraphLink} link - model to theme or machine
 * @param {CellIndex} cells - figures per theme
 * @param {MapFilter} filter - current selection
 * @returns {number | null} - score the link stands for, null when never played under this selection
 */
export function linkScore(link: GraphLink, cells: CellIndex, filter: MapFilter): number | null {
  const cell = link.kind === 'score' ? cellOf(cells, filter.machine, link.from.key, link.to.key) : cellOf(cells, link.to.key, link.from.key, filter.category);
  return cell ? cell.avg_score : null;
}

/**
 * @param {GraphNode} node - node
 * @param {CellIndex} cells - figures per theme
 * @param {MapContent} content - what the map holds
 * @param {MapFilter} filter - current selection
 * @param {RankSection[]} sections - current rankings
 * @returns {string} - second label line of a hub: its counter, then its figure under this selection
 */
export function nodeFigure(node: GraphNode, cells: CellIndex, content: MapContent, filter: MapFilter, sections: RankSection[]): string {
  const rank = sections.find((section) => section.kind === node.kind)?.rows.find((row) => row.key === node.key)?.rank;
  const prefix = rank ? `n°${rank} · ` : '';
  const figures = (machine: string, model: string, category: string): string => {
    const cell = cellOf(cells, machine, model, category);
    return cell ? `${prefix}${formatScore(cell.avg_score)} · ${duration(cell.p50_total_ms)}` : 'Sans donnée';
  };
  if (node.kind === 'model') return figures(filter.machine, node.key, filter.category);
  if (node.kind === 'machine') {
    if (filter.model) return figures(node.key, filter.model, filter.category);
    const count = content.models.filter((model) => cellOf(cells, node.key, model, filter.category)).length;
    return count === 0 ? 'Sans donnée' : `${count} modèle${count > 1 ? 's' : ''}`;
  }
  const counter = `${node.count} question${node.count > 1 ? 's' : ''}`;
  if (filter.model) return `${counter} · ${figures(filter.machine, filter.model, node.key)}`;
  const scored = content.models.flatMap((model) => {
    const cell = cellOf(cells, filter.machine, model, node.key);
    return cell ? [{ model, value: cell.avg_score }] : [];
  });
  if (scored.length === 0) return counter;
  const best = Math.max(...scored.map((entry) => entry.value));
  const leaders = scored.filter((entry) => sameScore(entry.value, best));
  return `${counter} · ${formatScore(best)} ${leaders.length > 1 ? 'ex aequo' : shortModel(leaders[0].model, content.models)}`;
}

/**
 * @param {Graph} graph - graph
 * @param {GraphNode} node - starting node
 * @returns {Set<string>} - ids of the node, its ancestors, its descendants and its direct neighbours
 */
export function familyOf(graph: Graph, node: GraphNode): Set<string> {
  const family = new Set<string>([node.id]);
  const structural = (link: GraphLink): boolean => link.kind === 'structure' || link.kind === 'source';
  const walk = (start: GraphNode, step: (link: GraphLink, at: GraphNode) => GraphNode | null): void => {
    const pending = [start];
    const seen = new Set<string>([start.id]);
    while (pending.length > 0) {
      const at = pending.pop() as GraphNode;
      for (const link of graph.links) {
        const next = structural(link) ? step(link, at) : null;
        if (!next || seen.has(next.id)) continue;
        seen.add(next.id);
        family.add(next.id);
        pending.push(next);
      }
    }
  };
  walk(node, (link, at) => (link.from === at ? link.to : null));
  walk(node, (link, at) => (link.to === at ? link.from : null));
  for (const link of graph.links) {
    if (structural(link)) continue;
    if (link.from === node) family.add(link.to.id);
    if (link.to === node) family.add(link.from.id);
  }
  return family;
}

/**
 * @param {MapContent} content - what the map holds
 * @param {ItemIndex} items - cumulated figures
 * @param {MapFilter} filter - selected model, machine and theme
 * @returns {Record<ResultState, number>} - questions of the selection per state
 */
export function resultCounts(content: MapContent, items: ItemIndex, filter: MapFilter): Record<ResultState, number> {
  const counts: Record<ResultState, number> = { passed: 0, partial: 0, failed: 0, none: 0 };
  for (const question of content.questions) {
    if (questionKept(question, filter)) counts[stateOfItem(itemOf(items, filter.machine, filter.model, question.caseId, question.item.id))] += 1;
  }
  return counts;
}
