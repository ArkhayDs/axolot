// Canvas painting of the map. Holds no state: the component owns the view, the particles and the ripples.

import { score as formatScore, seriesColor } from '../format';
import type { MapContent } from './content';
import { familyOf, linkScore, nodeMatches } from './figures';
import type { Graph, GraphNode } from './graph';
import { paintLabels } from './labels';
import { communityColor, paintModel, paintResult, resolve, trace } from './paint';
import type { CellIndex, MapFilter, RankSection } from './ranking';
import { itemOf, stateOfItem } from './results';
import type { ItemIndex, ResultState } from './results';

export interface View {
  x: number;
  y: number;
  k: number;
}

export interface Particle {
  // The real route of the answer, walked leg by leg.
  path: GraphNode[];
  progress: number;
  color: string;
}

export interface Ripple {
  node: GraphNode;
  age: number;
  color: string;
  text: string;
}

export interface Scene {
  width: number;
  height: number;
  view: View;
  graph: Graph;
  content: MapContent;
  cells: CellIndex;
  items: ItemIndex;
  filter: MapFilter;
  focus: GraphNode | null;
  sections: RankSection[];
  hover: GraphNode | null;
  activePath: GraphNode[];
  // Answers of the running benchmark: by "model|question", and by "|question" for the latest of any model.
  liveResults: Map<string, ResultState>;
  // Nodes of the running benchmark, the rest fades; null at rest.
  liveNodes: Set<string> | null;
  particles: Particle[];
  ripples: Ripple[];
}

const GRID_STEP = 48;

/**
 * Light grid in screen space, following pan and zoom, doubling its step when zoomed out
 * @param {CanvasRenderingContext2D} context - canvas without the view transform
 * @param {Scene} scene - what to paint
 */
function paintGrid(context: CanvasRenderingContext2D, scene: Scene): void {
  const { view, width, height } = scene;
  let step = GRID_STEP * view.k;
  while (step < 28) step *= 2;
  const left = ((view.x % step) + step) % step;
  const top = ((view.y % step) + step) % step;
  context.strokeStyle = resolve('var(--map-grid)');
  context.lineWidth = 1;
  context.beginPath();
  for (let x = left; x < width; x += step) {
    context.moveTo(Math.round(x) + 0.5, 0);
    context.lineTo(Math.round(x) + 0.5, height);
  }
  for (let y = top; y < height; y += step) {
    context.moveTo(0, Math.round(y) + 0.5);
    context.lineTo(width, Math.round(y) + 0.5);
  }
  context.stroke();
}

/**
 * Diffuse halo per community, from where its nodes really are
 * @param {CanvasRenderingContext2D} context - canvas
 * @param {Scene} scene - what to paint
 * @param {number} strength - halo opacity
 */
function paintHalos(context: CanvasRenderingContext2D, scene: Scene, strength: number): void {
  const { graph, content, filter } = scene;
  for (const theme of content.themes) {
    if (filter.category && filter.category !== theme) continue;
    const members = graph.nodes.filter((node) => node.community === theme);
    if (members.length === 0) continue;
    const x = members.reduce((total, node) => total + node.x, 0) / members.length;
    const y = members.reduce((total, node) => total + node.y, 0) / members.length;
    const spread = Math.max(...members.map((node) => Math.hypot(node.x - x, node.y - y)));
    const radius = Math.max(spread * 1.7, 70);
    const color = communityColor(theme, content.themes);
    const gradient = context.createRadialGradient(x, y, 0, x, y, radius);
    gradient.addColorStop(0, color);
    gradient.addColorStop(1, `${color}00`);
    context.globalAlpha = strength;
    context.fillStyle = gradient;
    context.beginPath();
    context.arc(x, y, radius, 0, Math.PI * 2);
    context.fill();
  }
  context.globalAlpha = 1;
}

/**
 * @param {GraphNode} node - node
 * @param {Scene} scene - what to paint
 * @returns {ResultState | null} - result the point takes, null to keep its community color
 */
function shownResult(node: GraphNode, scene: Scene): ResultState | null {
  if (!node.question) return null;
  const { filter, hover, items, liveResults } = scene;
  const model = filter.model || (hover?.kind === 'model' ? hover.key : '');
  const live = liveResults.get(`${model}|${node.question.key}`);
  if (live) return live;
  return model ? stateOfItem(itemOf(items, filter.machine, model, node.question.caseId, node.question.item.id)) : null;
}

/**
 * Links: hairlines for the structure, width and opacity following the score for model to theme
 * @param {CanvasRenderingContext2D} context - canvas
 * @param {Scene} scene - what to paint
 * @param {Set<string>} kept - ids the filters keep
 * @param {Set<string> | null} family - ids around the focus or hover, null without any
 * @param {number} pixel - one screen pixel in world units
 * @returns {{ x: number; y: number; text: string }[]} - score pills for the focused links
 */
function paintLinks(context: CanvasRenderingContext2D, scene: Scene, kept: Set<string>, family: Set<string> | null, pixel: number): { x: number; y: number; text: string }[] {
  const { graph, cells, filter, content } = scene;
  const pills: { x: number; y: number; text: string }[] = [];
  const hairline = resolve('var(--map-link)');
  for (const link of graph.links) {
    const bothKept = kept.has(link.from.id) && kept.has(link.to.id);
    const close = family !== null && family.has(link.from.id) && family.has(link.to.id);
    const faded = !bothKept || (family !== null && !close);
    context.setLineDash([]);
    if (link.kind === 'score' || link.kind === 'machine') {
      const value = linkScore(link, cells, filter);
      if (value === null) continue;
      const strength = (value / 100) ** 3;
      const picked = link.from.key === filter.model || link.to.key === filter.category || link.to.key === filter.machine;
      const focused = close || picked;
      context.globalAlpha = faded && !picked ? 0.05 : focused ? 0.35 + 0.6 * strength : 0.1 + 0.3 * strength;
      context.lineWidth = (0.6 + 2.2 * strength) * pixel;
      context.strokeStyle = link.kind === 'machine' && !focused ? resolve('var(--ink-3)') : resolve(seriesColor(link.from.key, content.models));
      if (link.kind === 'machine' && !focused) context.setLineDash([4 * pixel, 4 * pixel]);
      if (link.kind === 'machine' && focused) {
        context.globalAlpha = 1;
        context.lineWidth = 2 * pixel;
      }
      // Under a selected model each hub already reads its score: a pill per link would only cover the islands.
      if (focused && !faded && link.kind === 'score' && !filter.model) {
        pills.push({ x: link.from.x + (link.to.x - link.from.x) * 0.6, y: link.from.y + (link.to.y - link.from.y) * 0.6, text: formatScore(value) });
      }
    } else {
      context.globalAlpha = faded ? 0.1 : close ? 0.9 : link.kind === 'source' ? 0.35 : 0.55;
      context.lineWidth = (close ? 1 : 0.6) * pixel;
      context.strokeStyle = close && link.to.community ? communityColor(link.to.community, content.themes) : hairline;
    }
    context.beginPath();
    context.moveTo(link.from.x, link.from.y);
    context.lineTo(link.to.x, link.to.y);
    context.stroke();
  }
  context.setLineDash([]);
  context.globalAlpha = 1;
  return pills;
}

/**
 * Model to question links, only under a selected model or question: otherwise the graph turns into a ball of wool
 * @param {CanvasRenderingContext2D} context - canvas
 * @param {Scene} scene - what to paint
 * @param {Set<string>} kept - ids the filters keep
 * @param {number} pixel - one screen pixel in world units
 */
function paintAnswerLinks(context: CanvasRenderingContext2D, scene: Scene, kept: Set<string>, pixel: number): void {
  const { graph, filter, focus, items, content } = scene;
  const focusQuestion = focus?.kind === 'question' ? focus : null;
  const models = focusQuestion ? (filter.model ? [filter.model] : content.models) : filter.model ? [filter.model] : [];
  context.lineWidth = 0.7 * pixel;
  for (const model of models) {
    const from = graph.byId.get(`model:${model}`);
    if (!from) continue;
    for (const dot of focusQuestion ? [focusQuestion] : graph.nodes) {
      if (!dot.question || !kept.has(dot.id)) continue;
      const item = itemOf(items, filter.machine, model, dot.question.caseId, dot.question.item.id);
      if (!item) continue;
      const state = stateOfItem(item);
      context.globalAlpha = focusQuestion ? 0.8 : 0.16;
      context.strokeStyle = resolve(state === 'passed' ? 'var(--good)' : state === 'partial' ? 'var(--warning)' : 'var(--critical)');
      context.beginPath();
      context.moveTo(from.x, from.y);
      context.lineTo(dot.x, dot.y);
      context.stroke();
    }
  }
  context.globalAlpha = 1;
}

/**
 * @param {CanvasRenderingContext2D} context - canvas
 * @param {Scene} scene - what to paint
 * @param {GraphNode} node - node
 * @param {number} pixel - one screen pixel in world units
 */
function paintNode(context: CanvasRenderingContext2D, scene: Scene, node: GraphNode, pixel: number): void {
  const { content } = scene;
  const ground = resolve('var(--ground)');
  const result = shownResult(node, scene);
  if (result) {
    paintResult(context, node, result, Math.max(node.radius + 0.8, 4.2 * pixel), pixel);
    return;
  }
  if (node.kind === 'model') {
    paintModel(context, node, resolve(seriesColor(node.key, content.models)), ground, pixel);
    return;
  }
  trace(context, node, node.radius);
  if (node.kind === 'machine' || node.kind === 'benchmark') {
    context.fillStyle = resolve('var(--raised)');
    context.strokeStyle = resolve('var(--ink-2)');
    context.lineWidth = 1.25 * pixel;
    context.fill();
    context.stroke();
    return;
  }
  const color = communityColor(node.community, content.themes);
  context.fillStyle = color;
  context.fill();
  if (node.kind === 'theme' || node.kind === 'scenario') {
    context.strokeStyle = resolve('var(--ink)');
    context.lineWidth = (node.kind === 'theme' ? 1.5 : 0.8) * pixel;
    context.stroke();
  }
}

/**
 * @param {CanvasRenderingContext2D} context - canvas
 * @param {Particle} particle - impulse
 * @param {number} pixel - one screen pixel in world units
 */
function paintParticle(context: CanvasRenderingContext2D, particle: Particle, pixel: number): void {
  const legs = particle.path.length - 1;
  const at = Math.min(particle.progress * legs, legs - 1e-6);
  const leg = Math.floor(at);
  const from = particle.path[leg];
  const to = particle.path[leg + 1];
  context.fillStyle = particle.color;
  context.beginPath();
  context.arc(from.x + (to.x - from.x) * (at - leg), from.y + (to.y - from.y) * (at - leg), 2.4 * pixel, 0, Math.PI * 2);
  context.fill();
}

/**
 * @param {CanvasRenderingContext2D} context - canvas
 * @param {Scene} scene - what to paint
 */
export function drawScene(context: CanvasRenderingContext2D, scene: Scene): void {
  const { width, height, view, graph, content, cells, filter, focus, hover, activePath, liveNodes } = scene;
  const ratio = window.devicePixelRatio || 1;
  const pixel = 1 / view.k;
  const ink = resolve('var(--ink)');
  const accent = resolve('var(--accent)');
  const ground = resolve('var(--ground)');
  const font = resolve('var(--font)');
  context.setTransform(ratio, 0, 0, ratio, 0, 0);
  context.clearRect(0, 0, width, height);
  paintGrid(context, scene);
  context.setTransform(ratio * view.k, 0, 0, ratio * view.k, ratio * view.x, ratio * view.y);

  const kept = new Set(graph.nodes.filter((node) => nodeMatches(node, cells, content, filter) && (!liveNodes || liveNodes.has(node.id))).map((node) => node.id));
  const faded = liveNodes ? 0.08 : 0.2;
  const family = hover ? familyOf(graph, hover) : focus ? familyOf(graph, focus) : null;
  const previewing = Boolean(filter.model) || hover?.kind === 'model';
  paintHalos(context, scene, Number(resolve('var(--map-halo-opacity)')) * (previewing ? 0.45 : 1));
  const pills = paintLinks(context, scene, kept, family, pixel);
  paintAnswerLinks(context, scene, kept, pixel);

  context.strokeStyle = accent;
  context.lineWidth = 1.5 * pixel;
  context.beginPath();
  activePath.forEach((node, position) => (position === 0 ? context.moveTo(node.x, node.y) : context.lineTo(node.x, node.y)));
  context.stroke();
  for (const particle of scene.particles) paintParticle(context, particle, pixel);

  for (const node of graph.nodes) {
    const dimmed = !kept.has(node.id) || (family !== null && !family.has(node.id) && !shownResult(node, scene));
    context.globalAlpha = dimmed && !activePath.includes(node) ? faded : 1;
    paintNode(context, scene, node, pixel);
    if (node === focus || node === hover || node === activePath.at(-1) || node.key === filter.model || node.key === filter.machine || node.key === filter.category) {
      context.globalAlpha = 1;
      context.strokeStyle = node === hover ? ink : accent;
      context.lineWidth = 1.5 * pixel;
      trace(context, node, node.radius + 3.5 * pixel);
      context.stroke();
    }
  }
  context.globalAlpha = 1;
  paintLabels(context, scene, kept, family, pixel);

  context.font = `600 ${11 * pixel}px ${font}`;
  context.textAlign = 'center';
  context.textBaseline = 'middle';
  for (const pill of pills) {
    const pillWidth = context.measureText(pill.text).width + 8 * pixel;
    context.fillStyle = ground;
    context.strokeStyle = resolve('var(--axis)');
    context.lineWidth = pixel;
    context.beginPath();
    context.roundRect(pill.x - pillWidth / 2, pill.y - 8 * pixel, pillWidth, 16 * pixel, 4 * pixel);
    context.fill();
    context.stroke();
    context.fillStyle = ink;
    context.fillText(pill.text, pill.x, pill.y);
  }

  for (const ripple of scene.ripples) {
    context.globalAlpha = 1 - ripple.age;
    context.strokeStyle = ripple.color;
    context.lineWidth = 1.5 * pixel;
    context.beginPath();
    context.arc(ripple.node.x, ripple.node.y, ripple.node.radius + (3 + ripple.age * 24) * pixel, 0, Math.PI * 2);
    context.stroke();
    context.fillStyle = ripple.color;
    context.font = `700 ${13 * pixel}px ${font}`;
    context.fillText(ripple.text, ripple.node.x, ripple.node.y - ripple.node.radius - (10 + ripple.age * 16) * pixel);
  }
  context.globalAlpha = 1;
  context.textAlign = 'start';
  context.textBaseline = 'alphabetic';
}
