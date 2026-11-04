// Graph of the map: which nodes and links exist, their seeds, sizes and springs. Grain goes from machine down to source document.

import { benchmarkName, categoryLabel } from '../format';
import type { MapContent, Question } from './content';
import type { Body, Spring } from './force';
import { cellOf, shortModel } from './ranking';
import type { CellIndex, RankKind } from './ranking';
import { itemOf } from './results';
import type { ItemIndex } from './results';

export type NodeKind = RankKind | 'scenario' | 'question' | 'document' | 'benchmark';

export interface GraphNode extends Body {
  id: string;
  kind: NodeKind;
  key: string;
  label: string;
  // Theme whose color the node takes, empty outside communities.
  community: string;
  // Questions under the node, or answers for a question: what its size and counter say.
  count: number;
  question?: Question;
}

export interface GraphLink {
  kind: 'score' | 'machine' | 'structure' | 'source' | 'benchmark';
  from: GraphNode;
  to: GraphNode;
}

export interface Graph {
  // Smallest first: the paint order keeps hubs on top.
  nodes: GraphNode[];
  links: GraphLink[];
  springs: Spring[];
  byId: Map<string, GraphNode>;
}

const THEME_RING = 310;
const GOLDEN_ANGLE = 2.399963;
const HUB_LABEL_HEIGHT = 30;
const HUB_LABEL_MIN_WIDTH = 74;

const STEPS: Record<NodeKind, { limits: number[]; radii: number[]; charge: number }> = {
  machine: { limits: [], radii: [15], charge: 8 },
  model: { limits: [], radii: [13], charge: 8 },
  theme: { limits: [3, 8], radii: [8, 10, 12], charge: 12 },
  scenario: { limits: [3, 7], radii: [4.5, 5.5, 6.5], charge: 2.5 },
  question: { limits: [0, 2, 6], radii: [2.4, 3, 3.6, 4.2], charge: 1 },
  document: { limits: [1, 3], radii: [3.4, 4.2, 5], charge: 1.2 },
  benchmark: { limits: [], radii: [4], charge: 1.5 },
};

/**
 * @param {NodeKind} kind - node type
 * @param {string} key - identity within the type
 * @returns {string} - node id
 */
export function nodeId(kind: NodeKind, key: string): string {
  return `${kind}:${key}`;
}

/**
 * @param {string} text - stable identity
 * @returns {number} - FNV-1a hash in 0-1: same data, same seed, same layout
 */
function hash(text: string): number {
  let value = 2166136261;
  for (let position = 0; position < text.length; position += 1) {
    value = Math.imul(value ^ text.charCodeAt(position), 16777619);
  }
  return (value >>> 0) / 4294967296;
}

/**
 * Builds the graph, keeping the place of nodes that already exist
 * @param {MapContent} content - what the map holds
 * @param {CellIndex} cells - figures per theme
 * @param {ItemIndex} items - figures per question
 * @param {Map<string, GraphNode>} previous - nodes of the graph being replaced
 * @param {number} aspect - free area width over height, the seeds follow it
 * @param {(text: string) => number} measure - hub label width in pixels
 * @returns {Graph} - nodes, links and springs
 */
export function buildGraph(content: MapContent, cells: CellIndex, items: ItemIndex, previous: Map<string, GraphNode>, aspect: number, measure: (text: string) => number): Graph {
  const byId = new Map<string, GraphNode>();
  const links: GraphLink[] = [];
  const springs: Spring[] = [];
  const stretch = Math.sqrt(Math.max(aspect, 0.5));

  const add = (kind: NodeKind, key: string, label: string, community: string, count: number, x: number, y: number, question?: Question): GraphNode => {
    const id = nodeId(kind, key);
    const known = previous.get(id);
    const { limits, radii, charge } = STEPS[kind];
    const step = limits.findIndex((limit) => count <= limit);
    const hub = kind === 'machine' || kind === 'model' || kind === 'theme';
    const node: GraphNode = {
      id,
      kind,
      key,
      label,
      community,
      count,
      question,
      x: known?.x ?? x,
      y: known?.y ?? y,
      vx: 0,
      vy: 0,
      seedX: x,
      seedY: y,
      radius: radii[step < 0 ? radii.length - 1 : step],
      charge,
      labelWidth: hub ? Math.max(measure(label), HUB_LABEL_MIN_WIDTH) + 8 : 0,
      labelHeight: hub ? HUB_LABEL_HEIGHT : 0,
      pinned: false,
    };
    byId.set(id, node);
    return node;
  };
  const link = (kind: GraphLink['kind'], from: GraphNode, to: GraphNode, rest: number, strength: number): void => {
    links.push({ kind, from, to });
    springs.push({ a: from, b: to, rest, strength });
  };

  content.machines.forEach((key, position) => add('machine', key, key, '', 0, position * 34 - 12, -24));
  content.models.forEach((key, position) => {
    const angle = (2 * Math.PI * position) / Math.max(content.models.length, 1);
    add('model', key, shortModel(key, content.models), '', 0, Math.cos(angle) * 80 * stretch, Math.sin(angle) * 60);
  });

  const themeNodes = new Map<string, GraphNode>();
  content.themes.forEach((theme, position) => {
    const angle = (2 * Math.PI * position) / Math.max(content.themes.length, 1) + 0.3;
    const count = content.questions.filter((question) => question.category === theme).length;
    themeNodes.set(theme, add('theme', theme, categoryLabel(theme), theme, count, Math.cos(angle) * THEME_RING * stretch, (Math.sin(angle) * THEME_RING) / stretch));
  });

  for (const scenario of content.scenarios) {
    const theme = themeNodes.get(scenario.category) as GraphNode;
    const angle = hash(scenario.id) * 2 * Math.PI;
    const hub = add('scenario', scenario.id, scenario.title, scenario.category, scenario.items.length, theme.x + Math.cos(angle) * 50, theme.y + Math.sin(angle) * 50);
    link('structure', theme, hub, 26 + 3 * Math.sqrt(scenario.items.length), 0.22);
    const rest = 10 + 2.4 * Math.sqrt(scenario.items.length);
    scenario.items.forEach((item, position) => {
      const key = `${scenario.id}/${item.id}`;
      const question = content.questions.find((candidate) => candidate.key === key) as Question;
      const runs = content.models.reduce((total, model) => total + (itemOf(items, '', model, scenario.id, item.id)?.runs ?? 0), 0);
      const spin = position * GOLDEN_ANGLE;
      const spread = 8 + 6 * Math.sqrt(position);
      const dot = add('question', key, item.id, scenario.category, runs, hub.x + Math.cos(spin) * spread, hub.y + Math.sin(spin) * spread, question);
      link('structure', hub, dot, rest, 0.22);
    });
  }

  for (const [file, questions] of content.documents) {
    const dots = questions.map((question) => byId.get(nodeId('question', question.key)) as GraphNode);
    const x = dots.reduce((total, dot) => total + dot.x, 0) / dots.length + (hash(file) - 0.5) * 30;
    const y = dots.reduce((total, dot) => total + dot.y, 0) / dots.length + (hash(`${file}y`) - 0.5) * 30;
    const document = add('document', file, file, questions[0].category, questions.length, x, y);
    for (const dot of dots) link('source', dot, document, 26, 0.03);
  }

  for (const model of content.models) {
    const modelNode = byId.get(nodeId('model', model)) as GraphNode;
    for (const theme of content.themes) {
      const cell = cellOf(cells, '', model, theme);
      // A strong result draws the theme toward its model, so the layout itself shows who is good at what.
      if (cell) link('score', modelNode, themeNodes.get(theme) as GraphNode, 170 + (100 - cell.avg_score) * 1.4, 0.008);
    }
    for (const machine of content.machines) {
      if (cellOf(cells, machine, model, '')) link('machine', modelNode, byId.get(nodeId('machine', machine)) as GraphNode, 58, 0.06);
    }
  }

  for (const benchmark of content.benchmarks) {
    const angle = hash(benchmark.id) * 2 * Math.PI;
    const node = add('benchmark', benchmark.id, benchmarkName(benchmark.name), '', benchmark.done_items, Math.cos(angle) * 120 * stretch, Math.sin(angle) * 100);
    for (const model of benchmark.models) {
      const modelNode = byId.get(nodeId('model', model));
      if (modelNode) link('benchmark', node, modelNode, 70, 0.02);
    }
  }

  const nodes = [...byId.values()].sort((a, b) => a.radius - b.radius);
  return { nodes, links, springs, byId };
}
