// Labels of the map: hubs always, the rest while they find room, small nodes past a zoom or under the pointer.

import type { Scene } from './draw';
import { nodeFigure } from './figures';
import type { GraphNode } from './graph';
import { communityColor, resolve, shorten } from './paint';

const LABEL_ZOOM: Partial<Record<GraphNode['kind'], number>> = { document: 1.25, benchmark: 1.25, question: 1.6 };
const HUBS: GraphNode['kind'][] = ['machine', 'model', 'theme'];
const LABEL_LENGTH = 26;

/**
 * Labels by priority: hubs always, the rest while they find room, small nodes past a zoom or under the pointer
 * @param {CanvasRenderingContext2D} context - canvas
 * @param {Scene} scene - what to paint
 * @param {Set<string>} kept - ids the filters keep
 * @param {Set<string> | null} family - ids around the focus or hover
 * @param {number} pixel - one screen pixel in world units
 */
export function paintLabels(context: CanvasRenderingContext2D, scene: Scene, kept: Set<string>, family: Set<string> | null, pixel: number): void {
  const { graph, view, content, cells, filter, sections, hover, focus, activePath } = scene;
  const font = resolve('var(--font)');
  const ink = resolve('var(--ink)');
  const ink2 = resolve('var(--ink-2)');
  const accent = resolve('var(--accent)');
  const ground = resolve('var(--ground)');
  const placed: { left: number; right: number; top: number; bottom: number }[] = [];
  const priority = (node: GraphNode): number => (node === hover || node === focus ? 0 : HUBS.includes(node.kind) ? 1 : node.kind === 'scenario' ? 2 : 3);
  const ordered = [...graph.nodes].sort((a, b) => priority(a) - priority(b));
  context.textBaseline = 'middle';
  context.textAlign = 'left';
  for (const node of ordered) {
    const named = node === hover || node === focus || activePath.includes(node);
    const threshold = LABEL_ZOOM[node.kind];
    if (!named && threshold && view.k < threshold) continue;
    const hub = HUBS.includes(node.kind);
    const text = hub || named ? node.label : shorten(node.label, LABEL_LENGTH);
    const figure = hub ? nodeFigure(node, cells, content, filter, sections) : node.kind === 'scenario' ? `${node.count}` : '';
    const nameSize = (hub ? 12.5 : node.kind === 'scenario' ? 11.5 : 11) * pixel;
    const figureSize = 11 * pixel;
    context.font = `${hub ? 650 : 500} ${nameSize}px ${font}`;
    const nameWidth = context.measureText(text).width;
    context.font = `500 ${figureSize}px ${font}`;
    const inline = node.kind === 'scenario';
    const figureWidth = figure ? context.measureText(figure).width : 0;
    const left = node.x + node.radius + 5 * pixel;
    const totalWidth = inline ? nameWidth + 5 * pixel + figureWidth : Math.max(nameWidth, figureWidth);
    const lines = hub && figure ? 2 : 1;
    const top = node.y - (lines * 8 + 1) * pixel;
    const box = { left, right: left + totalWidth, top, bottom: top + (lines * 16 + 2) * pixel };
    const hides = placed.some((other) => box.left < other.right && box.right > other.left && box.top < other.bottom && box.bottom > other.top);
    if (hides && !hub && !named) continue;
    placed.push(box);
    const dimmed = !kept.has(node.id) || (family !== null && !family.has(node.id));
    context.globalAlpha = dimmed ? 0.2 : 0.5;
    context.fillStyle = ground;
    context.fillRect(box.left - 2 * pixel, box.top, totalWidth + 4 * pixel, box.bottom - box.top);
    context.globalAlpha = dimmed ? 0.3 : 1;
    const picked = node === focus || activePath.includes(node) || node.key === filter.model || node.key === filter.machine || node.key === filter.category;
    context.font = `${hub ? 650 : 500} ${nameSize}px ${font}`;
    context.fillStyle = picked ? accent : hub ? ink : ink2;
    context.fillText(text, left, lines === 2 ? node.y - 8 * pixel : node.y);
    if (!figure) continue;
    context.font = `500 ${figureSize}px ${font}`;
    context.fillStyle = node.community ? communityColor(node.community, content.themes) : ink2;
    if (inline) context.fillText(figure, left + nameWidth + 5 * pixel, node.y);
    else context.fillText(figure, left, node.y + 8 * pixel);
  }
  context.globalAlpha = 1;
  context.textBaseline = 'alphabetic';
}
