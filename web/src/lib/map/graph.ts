// Graph of the map: which nodes and links exist, what each one says under the current selection.

import { categoryLabel, duration, score as formatScore } from '../format';
import type { Body, Spring } from './force';
import { cellOf, sameScore, shortModel } from './ranking';
import type { CellIndex, MapFilter, MapLists, RankKind, RankSection } from './ranking';

export interface GraphNode extends Body {
  id: string;
  kind: RankKind;
  key: string;
  label: string;
}

export interface GraphLink {
  kind: 'theme' | 'machine';
  model: GraphNode;
  other: GraphNode;
}

export interface Graph {
  nodes: GraphNode[];
  links: GraphLink[];
  springs: Spring[];
  byId: Map<string, GraphNode>;
}

const RADIUS: Record<RankKind, number> = { machine: 20, model: 17, theme: 11 };
const LABEL_HEIGHT = 36;
const FIGURE_WIDTH = 96;

/**
 * @param {RankKind} kind - node type
 * @param {string} key - machine label, model tag or category
 * @returns {string} - node id
 */
export function nodeId(kind: RankKind, key: string): string {
  return `${kind}:${key}`;
}

/**
 * Builds the graph, keeping the place of nodes that already exist
 * @param {MapLists} lists - axes
 * @param {CellIndex} index - indexed cells
 * @param {Map<string, GraphNode>} previous - nodes of the graph being replaced
 * @param {(text: string) => number} measure - label width in pixels
 * @param {number} labelScale - room labels need beyond their size, a small screen zooms out but keeps text readable
 * @returns {Graph} - nodes, links and springs
 */
export function buildGraph(lists: MapLists, index: CellIndex, previous: Map<string, GraphNode>, measure: (text: string) => number, labelScale = 1): Graph {
  const nodes: GraphNode[] = [];
  const add = (kind: RankKind, key: string, label: string, x: number, y: number): void => {
    const id = nodeId(kind, key);
    const known = previous.get(id);
    const labelWidth = Math.max(measure(label), FIGURE_WIDTH) * labelScale;
    nodes.push({ id, kind, key, label, x: known?.x ?? x, y: known?.y ?? y, vx: 0, vy: 0, radius: RADIUS[kind], labelWidth, labelHeight: LABEL_HEIGHT * labelScale, pinned: false });
  };
  lists.machines.forEach((key, position) => add('machine', key, key, position * 40, -60));
  lists.models.forEach((key, position) => {
    const angle = (2 * Math.PI * position) / Math.max(lists.models.length, 1);
    add('model', key, shortModel(key, lists.models), Math.cos(angle) * 240, Math.sin(angle) * 120);
  });
  lists.themes.forEach((key, position) => {
    const angle = (2 * Math.PI * position) / Math.max(lists.themes.length, 1) + 0.3;
    add('theme', key, categoryLabel(key), Math.cos(angle) * 420, Math.sin(angle) * 260);
  });

  const byId = new Map(nodes.map((node) => [node.id, node]));
  const links: GraphLink[] = [];
  const springs: Spring[] = [];
  for (const model of lists.models) {
    const modelNode = byId.get(nodeId('model', model)) as GraphNode;
    for (const theme of lists.themes) {
      const cell = cellOf(index, '', model, theme);
      if (!cell) continue;
      const other = byId.get(nodeId('theme', theme)) as GraphNode;
      links.push({ kind: 'theme', model: modelNode, other });
      // A strong result pulls the theme toward its model, so the layout itself shows who is good at what.
      springs.push({ a: modelNode, b: other, rest: 150 + (100 - cell.avg_score) * 5, strength: 0.05 });
    }
    for (const machine of lists.machines) {
      if (!cellOf(index, machine, model, '')) continue;
      const other = byId.get(nodeId('machine', machine)) as GraphNode;
      links.push({ kind: 'machine', model: modelNode, other });
      springs.push({ a: modelNode, b: other, rest: 170, strength: 0.05 });
    }
  }
  return { nodes, links, springs, byId };
}

/**
 * @param {GraphNode} node - node
 * @param {CellIndex} index - indexed cells
 * @param {MapLists} lists - axes
 * @param {MapFilter} filter - current selection
 * @returns {boolean} - whether the selection keeps this node
 */
export function nodeMatches(node: GraphNode, index: CellIndex, lists: MapLists, filter: MapFilter): boolean {
  const models = filter.model ? [filter.model] : lists.models;
  if (node.kind === 'model') return (!filter.model || node.key === filter.model) && Boolean(cellOf(index, filter.machine, node.key, filter.category));
  if (node.kind === 'theme') {
    return (!filter.category || node.key === filter.category) && models.some((model) => cellOf(index, filter.machine, model, node.key));
  }
  return (!filter.machine || node.key === filter.machine) && models.some((model) => cellOf(index, node.key, model, filter.category));
}

/**
 * @param {GraphLink} link - model to theme or machine
 * @param {CellIndex} index - indexed cells
 * @param {MapFilter} filter - current selection
 * @returns {number | null} - score the link stands for, null when never played under this selection
 */
export function linkScore(link: GraphLink, index: CellIndex, filter: MapFilter): number | null {
  const cell =
    link.kind === 'theme' ? cellOf(index, filter.machine, link.model.key, link.other.key) : cellOf(index, link.other.key, link.model.key, filter.category);
  return cell ? cell.avg_score : null;
}

/**
 * @param {GraphNode} node - node
 * @param {CellIndex} index - indexed cells
 * @param {MapLists} lists - axes
 * @param {MapFilter} filter - current selection
 * @param {RankSection[]} sections - current rankings
 * @returns {string} - second label line: the node's figure under this selection
 */
export function nodeFigure(node: GraphNode, index: CellIndex, lists: MapLists, filter: MapFilter, sections: RankSection[]): string {
  const rank = sections.find((section) => section.kind === node.kind)?.rows.find((row) => row.key === node.key)?.rank;
  const prefix = rank ? `n°${rank} · ` : '';
  const figures = (machine: string, model: string, category: string): string => {
    const cell = cellOf(index, machine, model, category);
    return cell ? `${prefix}${formatScore(cell.avg_score)} · ${duration(cell.p50_total_ms)}` : 'Sans donnée';
  };
  if (node.kind === 'model') return figures(filter.machine, node.key, filter.category);
  if (filter.model) return node.kind === 'theme' ? figures(filter.machine, filter.model, node.key) : figures(node.key, filter.model, filter.category);
  if (node.kind === 'machine') {
    const count = lists.models.filter((model) => cellOf(index, node.key, model, filter.category)).length;
    return count === 0 ? 'Sans donnée' : `${count} modèle${count > 1 ? 's' : ''}`;
  }
  const scored = lists.models.flatMap((model) => {
    const cell = cellOf(index, filter.machine, model, node.key);
    return cell ? [{ model, value: cell.avg_score }] : [];
  });
  if (scored.length === 0) return 'Sans donnée';
  const best = Math.max(...scored.map((entry) => entry.value));
  const leaders = scored.filter((entry) => sameScore(entry.value, best));
  return `${formatScore(best)} ${leaders.length > 1 ? 'ex aequo' : shortModel(leaders[0].model, lists.models)}`;
}
