// Canvas painting of the map. Holds no state: the component owns the view, the particles and the ripples.

import { score as formatScore, seriesColor } from '../format';
import type { Graph, GraphLink, GraphNode } from './graph';
import { linkScore, nodeFigure, nodeId, nodeMatches } from './graph';
import type { CellIndex, MapFilter, MapLists, RankSection } from './ranking';

export interface View {
  x: number;
  y: number;
  k: number;
}

export interface Particle {
  from: GraphNode;
  to: GraphNode;
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
  index: CellIndex;
  lists: MapLists;
  filter: MapFilter;
  sections: RankSection[];
  hover: GraphNode | null;
  activeModel: string;
  activeCategory: string;
  particles: Particle[];
  ripples: Ripple[];
}

const DIMMED = 0.16;

/**
 * Reads a design token, the canvas cannot use CSS variables directly
 * @param {string} value - color, possibly var(--token)
 * @returns {string} - resolved value
 */
export function resolve(value: string): string {
  const token = /^var\((--[\w-]+)\)$/.exec(value)?.[1];
  return token ? getComputedStyle(document.documentElement).getPropertyValue(token).trim() : value;
}

/**
 * @param {GraphNode} node - node
 * @param {MapFilter} filter - current selection
 * @returns {boolean} - whether the node itself is selected
 */
function isSelected(node: GraphNode, filter: MapFilter): boolean {
  if (node.kind === 'machine') return node.key === filter.machine;
  return node.kind === 'model' ? node.key === filter.model : node.key === filter.category;
}

/**
 * Traces the shape that tells node types apart without color: hexagon, circle, diamond
 * @param {CanvasRenderingContext2D} context - canvas
 * @param {GraphNode} node - node
 * @param {number} grow - extra radius
 */
function shape(context: CanvasRenderingContext2D, node: GraphNode, grow = 0): void {
  const radius = node.radius + grow;
  context.beginPath();
  if (node.kind === 'model') {
    context.arc(node.x, node.y, radius, 0, Math.PI * 2);
    return;
  }
  const sides = node.kind === 'machine' ? 6 : 4;
  for (let side = 0; side < sides; side += 1) {
    const angle = (Math.PI * 2 * side) / sides + (node.kind === 'machine' ? Math.PI / 6 : 0);
    context.lineTo(node.x + Math.cos(angle) * radius, node.y + Math.sin(angle) * radius);
  }
  context.closePath();
}

/**
 * @param {CanvasRenderingContext2D} context - canvas
 * @param {string} text - label
 * @param {number} x - center
 * @param {number} y - baseline
 * @param {string} color - text color
 * @param {string} halo - plate behind the text, keeps it readable over a link
 */
function label(context: CanvasRenderingContext2D, text: string, x: number, y: number, color: string, halo: string): void {
  const size = context.measureText(text);
  const height = size.actualBoundingBoxAscent + size.actualBoundingBoxDescent;
  context.fillStyle = halo;
  context.beginPath();
  context.roundRect(x - size.width / 2 - 3, y - size.actualBoundingBoxAscent - 2, size.width + 6, height + 4, 3);
  context.fill();
  context.fillStyle = color;
  context.fillText(text, x, y);
}

/**
 * Paints the links: width and opacity follow the score, machine links stay neutral and dashed
 * @param {CanvasRenderingContext2D} context - canvas
 * @param {Scene} scene - what to paint
 * @param {GraphLink} link - link
 * @param {number} value - score the link stands for
 * @param {boolean} focused - whether the hovered or selected node touches it
 * @param {boolean} kept - whether the current selection keeps it
 */
function paintLink(context: CanvasRenderingContext2D, scene: Scene, link: GraphLink, value: number, focused: boolean, kept: boolean): void {
  const strength = (value / 100) ** 3;
  const dimmed = !kept || (scene.hover !== null && !focused);
  context.globalAlpha = dimmed ? 0.05 : focused ? 0.35 + 0.65 * strength : 0.12 + 0.6 * strength;
  context.lineWidth = 0.75 + 4.5 * strength;
  context.strokeStyle = link.kind === 'machine' ? resolve('var(--ink-3)') : resolve(seriesColor(link.model.key, scene.lists.models));
  context.setLineDash(link.kind === 'machine' ? [5, 5] : []);
  context.beginPath();
  context.moveTo(link.model.x, link.model.y);
  context.lineTo(link.other.x, link.other.y);
  context.stroke();
  context.setLineDash([]);
  context.globalAlpha = 1;
}

/**
 * @param {CanvasRenderingContext2D} context - canvas
 * @param {Scene} scene - what to paint
 */
export function drawScene(context: CanvasRenderingContext2D, scene: Scene): void {
  const { width, height, view, graph, index, lists, filter, sections, hover } = scene;
  const ground = resolve('var(--ground)');
  const ink = resolve('var(--ink)');
  const ink2 = resolve('var(--ink-2)');
  const accent = resolve('var(--accent)');
  const font = resolve('var(--font)');
  const ratio = window.devicePixelRatio || 1;
  context.setTransform(ratio, 0, 0, ratio, 0, 0);
  context.clearRect(0, 0, width, height);
  context.setTransform(ratio * view.k, 0, 0, ratio * view.k, ratio * view.x, ratio * view.y);

  const kept = new Set(graph.nodes.filter((node) => nodeMatches(node, index, lists, filter)).map((node) => node.id));
  const neighbours = new Set<string>(hover ? [hover.id] : []);
  for (const link of graph.links) {
    if (link.model === hover) neighbours.add(link.other.id);
    if (link.other === hover) neighbours.add(link.model.id);
  }

  const pills: { x: number; y: number; text: string }[] = [];
  for (const link of graph.links) {
    const value = linkScore(link, index, filter);
    if (value === null) continue;
    const touchesSelection = isSelected(link.model, filter) || isSelected(link.other, filter);
    const focused = hover ? link.model === hover || link.other === hover : touchesSelection;
    const linkKept = kept.has(link.model.id) && kept.has(link.other.id);
    paintLink(context, scene, link, value, focused, linkKept);
    if (focused && linkKept) {
      pills.push({ x: link.model.x + (link.other.x - link.model.x) * 0.62, y: link.model.y + (link.other.y - link.model.y) * 0.62, text: formatScore(value) });
    }
  }

  const activeModelNode = graph.byId.get(nodeId('model', scene.activeModel));
  const activeThemeNode = graph.byId.get(nodeId('theme', scene.activeCategory));
  if (activeModelNode && activeThemeNode) {
    context.strokeStyle = accent;
    context.lineWidth = 2.5;
    context.beginPath();
    context.moveTo(activeModelNode.x, activeModelNode.y);
    context.lineTo(activeThemeNode.x, activeThemeNode.y);
    context.stroke();
  }

  for (const particle of scene.particles) {
    context.fillStyle = particle.color;
    context.beginPath();
    context.arc(particle.from.x + (particle.to.x - particle.from.x) * particle.progress, particle.from.y + (particle.to.y - particle.from.y) * particle.progress, 3, 0, Math.PI * 2);
    context.fill();
  }

  // Labels keep a readable size when zoomed out, the collision boxes leave room for that.
  const nameSize = Math.max(13, 11 / view.k);
  const figureSize = Math.max(12, 10.5 / view.k);
  context.textAlign = 'center';
  for (const node of graph.nodes) {
    const active = node === activeModelNode || node === activeThemeNode;
    const selected = isSelected(node, filter);
    const shown = hover ? neighbours.has(node.id) : kept.has(node.id) || active;
    const alpha = shown || active ? 1 : DIMMED;
    context.globalAlpha = alpha;
    context.fillStyle = node.kind === 'model' ? resolve(seriesColor(node.key, lists.models)) : resolve('var(--raised)');
    context.strokeStyle = node.kind === 'model' ? ground : resolve('var(--ink-3)');
    context.lineWidth = 1.5;
    shape(context, node);
    context.fill();
    context.stroke();
    if (selected || active || node === hover) {
      context.strokeStyle = active || selected ? accent : ink;
      context.lineWidth = 2.5;
      shape(context, node, 5);
      context.stroke();
    }
    const nameY = node.y + node.radius + nameSize + 4;
    context.font = `650 ${nameSize}px ${font}`;
    label(context, node.label, node.x, nameY, selected || active ? accent : ink, ground);
    context.font = `500 ${figureSize}px ${font}`;
    label(context, nodeFigure(node, index, lists, filter, sections), node.x, nameY + figureSize + 3, ink2, ground);
  }
  context.globalAlpha = 1;

  context.font = `600 ${figureSize}px ${font}`;
  context.textBaseline = 'middle';
  for (const pill of pills) {
    const pillWidth = context.measureText(pill.text).width + 10;
    context.fillStyle = ground;
    context.strokeStyle = resolve('var(--axis)');
    context.lineWidth = 1;
    context.beginPath();
    context.roundRect(pill.x - pillWidth / 2, pill.y - figureSize * 0.8, pillWidth, figureSize * 1.6, 5);
    context.fill();
    context.stroke();
    context.fillStyle = ink;
    context.fillText(pill.text, pill.x, pill.y + 0.5);
  }
  context.textBaseline = 'alphabetic';

  for (const ripple of scene.ripples) {
    context.globalAlpha = 1 - ripple.age;
    context.strokeStyle = ripple.color;
    context.lineWidth = 2.5;
    context.beginPath();
    context.arc(ripple.node.x, ripple.node.y, ripple.node.radius + 4 + ripple.age * 46, 0, Math.PI * 2);
    context.stroke();
    context.fillStyle = ripple.color;
    context.font = `700 ${nameSize + 4}px ${font}`;
    context.fillText(ripple.text, ripple.node.x, ripple.node.y - ripple.node.radius - 10 - ripple.age * 22);
  }
  context.globalAlpha = 1;
}
