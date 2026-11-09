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
  // Place in the default layout, where clearing a selection brings it back.
  homeX: number;
  homeY: number;
  // Width of the hub's label on screen, what framing and hit testing go by.
  textWidth: number;
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

const ISLAND_RING = 255;
const CENTER_RING = 92;
const GOLDEN_ANGLE = 2.399963;
const HUB_LABEL_HEIGHT = 30;
const HUB_LABEL_MIN_WIDTH = 118;

const STEPS: Record<NodeKind, { limits: number[]; radii: number[]; charge: number; pull: number }> = {
  machine: { limits: [], radii: [15], charge: 6, pull: 0.1 },
  model: { limits: [], radii: [13], charge: 6, pull: 0.1 },
  theme: { limits: [3, 8], radii: [8, 10, 12], charge: 2.5, pull: 0.2 },
  scenario: { limits: [3, 7], radii: [4.5, 5.5, 6.5], charge: 2.2, pull: 0.012 },
  question: { limits: [0, 2, 6], radii: [2.4, 3, 3.6, 4.2], charge: 1, pull: 0.006 },
  document: { limits: [1, 3], radii: [3.4, 4.2, 5], charge: 1.1, pull: 0.006 },
  benchmark: { limits: [], radii: [4], charge: 1.4, pull: 0.05 },
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
 * @param {Question[]} questions - questions citing a document
 * @returns {string} - the theme most of them belong to
 */
function mainTheme(questions: Question[]): string {
  const counts = new Map<string, number>();
  for (const { category } of questions) counts.set(category, (counts.get(category) ?? 0) + 1);
  return [...counts].sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]))[0][0];
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

  const add = (kind: NodeKind, key: string, label: string, community: string, count: number, x: number, y: number, anchorX: number, anchorY: number, question?: Question): GraphNode => {
    const id = nodeId(kind, key);
    const known = previous.get(id);
    const { limits, radii, charge, pull } = STEPS[kind];
    const step = limits.findIndex((limit) => count <= limit);
    const hub = kind === 'machine' || kind === 'model' || kind === 'theme';
    const textWidth = hub ? Math.max(measure(label), HUB_LABEL_MIN_WIDTH) + 8 : 0;
    // A theme hub sits amid its island: only the central hubs push others away from their labels.
    const roomy = kind === 'machine' || kind === 'model';
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
      homeX: known?.homeX ?? x,
      homeY: known?.homeY ?? y,
      seedX: x,
      seedY: y,
      anchorX,
      anchorY,
      pull,
      radius: radii[step < 0 ? radii.length - 1 : step],
      charge,
      textWidth,
      labelWidth: roomy ? textWidth : 0,
      labelHeight: roomy ? HUB_LABEL_HEIGHT : 0,
      pinned: false,
    };
    byId.set(id, node);
    return node;
  };
  const link = (kind: GraphLink['kind'], from: GraphNode, to: GraphNode, rest: number, strength: number): void => {
    links.push({ kind, from, to });
    // Without strength a link is only drawn: model to theme and shared sources must not deform the islands.
    if (strength > 0) springs.push({ a: from, b: to, rest, strength });
  };

  content.machines.forEach((key, position) => {
    const x = (position - (content.machines.length - 1) / 2) * 90;
    add('machine', key, key, '', 0, x, -CENTER_RING * 0.6, x, -CENTER_RING * 0.6);
  });
  content.models.forEach((key, position) => {
    const x = (position - (content.models.length - 1) / 2) * 110 * stretch;
    add('model', key, shortModel(key, content.models), '', 0, x, CENTER_RING * 0.35, x, CENTER_RING * 0.35);
  });

  const themeNodes = new Map<string, GraphNode>();
  const questionsOf = (theme: string): number => content.questions.filter((question) => question.category === theme).length;
  const weights = content.themes.map((theme) => 1 + 0.15 * Math.sqrt(questionsOf(theme)));
  const totalWeight = weights.reduce((total, weight) => total + weight, 0);
  let swept = 0;
  content.themes.forEach((theme, position) => {
    const angle = (2 * Math.PI * (swept + weights[position] / 2)) / totalWeight - Math.PI / 2;
    swept += weights[position];
    // A superellipse rather than a circle: islands reach into the corners of the free area.
    const x = Math.sign(Math.cos(angle)) * Math.abs(Math.cos(angle)) ** 0.6 * ISLAND_RING * stretch;
    const y = (Math.sign(Math.sin(angle)) * Math.abs(Math.sin(angle)) ** 0.6 * ISLAND_RING) / stretch;
    themeNodes.set(theme, add('theme', theme, categoryLabel(theme), theme, questionsOf(theme), x, y, x, y));
  });

  for (const scenario of content.scenarios) {
    const theme = themeNodes.get(scenario.category) as GraphNode;
    const angle = hash(scenario.id) * 2 * Math.PI;
    const hub = add('scenario', scenario.id, scenario.title, scenario.category, scenario.items.length, theme.x + Math.cos(angle) * 34, theme.y + Math.sin(angle) * 34, theme.anchorX, theme.anchorY);
    // Alone in its theme, a scenario stays on the hub: its questions then surround both.
    const alone = content.scenarios.filter((other) => other.category === scenario.category).length === 1;
    link('structure', theme, hub, alone ? 8 : 26 + 5 * Math.sqrt(scenario.items.length), alone ? 0.5 : 0.3);
    const rest = 11 + 3 * Math.sqrt(scenario.items.length);
    scenario.items.forEach((item, position) => {
      const key = `${scenario.id}/${item.id}`;
      const question = content.questions.find((candidate) => candidate.key === key) as Question;
      const runs = content.models.reduce((total, model) => total + (itemOf(items, '', model, scenario.id, item.id)?.runs ?? 0), 0);
      const spin = position * GOLDEN_ANGLE;
      const spread = 8 + 5 * Math.sqrt(position);
      const dot = add('question', key, item.id, scenario.category, runs, hub.x + Math.cos(spin) * spread, hub.y + Math.sin(spin) * spread, theme.anchorX, theme.anchorY, question);
      link('structure', hub, dot, rest, 0.3);
    });
  }

  for (const [file, questions] of content.documents) {
    const dots = questions.map((question) => byId.get(nodeId('question', question.key)) as GraphNode);
    const x = dots.reduce((total, dot) => total + dot.x, 0) / dots.length + (hash(file) - 0.5) * 30;
    const y = dots.reduce((total, dot) => total + dot.y, 0) / dots.length + (hash(`${file}y`) - 0.5) * 30;
    // A document cited across themes belongs to its main theme and pulls only on that one.
    const main = mainTheme(questions);
    const anchor = themeNodes.get(main) as GraphNode;
    const document = add('document', file, file, main, questions.length, x, y, anchor.anchorX, anchor.anchorY);
    dots.forEach((dot) => link('source', dot, document, 20, dot.community === main ? 0.05 : 0));
  }

  for (const model of content.models) {
    const modelNode = byId.get(nodeId('model', model)) as GraphNode;
    for (const theme of content.themes) {
      const cell = cellOf(cells, '', model, theme);
      if (cell?.avg_score != null) link('score', modelNode, themeNodes.get(theme) as GraphNode, 170 + (100 - cell.avg_score) * 1.4, 0.008);
    }
    for (const machine of content.machines) {
      if (cellOf(cells, machine, model, '')) link('machine', modelNode, byId.get(nodeId('machine', machine)) as GraphNode, 0, 0);
    }
  }

  for (const benchmark of content.benchmarks) {
    const angle = hash(benchmark.id) * 2 * Math.PI;
    const x = Math.cos(angle) * CENTER_RING * 1.3 * stretch;
    const y = Math.sin(angle) * CENTER_RING;
    const node = add('benchmark', benchmark.id, benchmarkName(benchmark.name), '', benchmark.done_items, x, y, x, y);
    for (const model of benchmark.models) {
      const modelNode = byId.get(nodeId('model', model));
      if (modelNode) link('benchmark', node, modelNode, 0, 0);
    }
  }

  const nodes = [...byId.values()].sort((a, b) => a.radius - b.radius);
  return { nodes, links, springs, byId };
}
