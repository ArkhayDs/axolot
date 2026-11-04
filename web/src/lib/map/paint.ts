// Canvas primitives of the map: tokens, shapes that tell node types and results apart without color, labels.

import type { GraphNode } from './graph';
import type { ResultState } from './results';

const MAX_ANSWER_DOTS = 8;

const tokens = new Map<string, string>();

/**
 * Reads a design token once: getComputedStyle forces a style recalculation when the page is dirty, far too costly per frame
 * @param {string} value - color, possibly var(--token)
 * @returns {string} - resolved value
 */
export function resolve(value: string): string {
  const known = tokens.get(value);
  if (known !== undefined) return known;
  const token = /^var\((--[\w-]+)\)$/.exec(value)?.[1];
  const resolved = token ? getComputedStyle(document.documentElement).getPropertyValue(token).trim() : value;
  tokens.set(value, resolved);
  return resolved;
}

/**
 * @param {string} community - theme
 * @param {string[]} themes - every theme, sorted
 * @returns {string} - community color
 */
export function communityColor(community: string, themes: string[]): string {
  return resolve(`var(--community-${(themes.indexOf(community) % 9) + 1})`);
}

/**
 * Traces a node: hexagon for a machine, diamond for a document, triangle for a benchmark, circle otherwise
 * @param {CanvasRenderingContext2D} context - canvas
 * @param {GraphNode} node - node
 * @param {number} radius - radius to trace
 */
export function trace(context: CanvasRenderingContext2D, node: GraphNode, radius: number): void {
  context.beginPath();
  const sides = node.kind === 'machine' ? 6 : node.kind === 'document' ? 4 : node.kind === 'benchmark' ? 3 : 0;
  if (sides === 0) {
    context.arc(node.x, node.y, radius, 0, Math.PI * 2);
    return;
  }
  const start = node.kind === 'machine' ? Math.PI / 6 : node.kind === 'benchmark' ? -Math.PI / 2 : 0;
  for (let side = 0; side < sides; side += 1) {
    const angle = (Math.PI * 2 * side) / sides + start;
    context.lineTo(node.x + Math.cos(angle) * radius, node.y + Math.sin(angle) * radius);
  }
  context.closePath();
}

/**
 * Paints a question in its result: filled, half filled, hollow or faint, like the answer matrix
 * @param {CanvasRenderingContext2D} context - canvas
 * @param {GraphNode} node - question
 * @param {ResultState} state - result
 * @param {number} radius - point radius
 * @param {number} pixel - one screen pixel in world units
 */
export function paintResult(context: CanvasRenderingContext2D, node: GraphNode, state: ResultState, radius: number, pixel: number): void {
  context.beginPath();
  context.arc(node.x, node.y, radius, 0, Math.PI * 2);
  if (state === 'passed') {
    context.fillStyle = resolve('var(--good)');
    context.fill();
  } else if (state === 'partial') {
    context.strokeStyle = resolve('var(--warning)');
    context.lineWidth = 1.25 * pixel;
    context.stroke();
    context.fillStyle = resolve('var(--warning)');
    context.beginPath();
    context.arc(node.x, node.y, radius, 0, Math.PI);
    context.closePath();
    context.fill();
  } else if (state === 'failed') {
    context.strokeStyle = resolve('var(--critical)');
    context.lineWidth = 2 * pixel;
    context.stroke();
  } else {
    context.strokeStyle = resolve('var(--axis)');
    context.lineWidth = pixel;
    context.stroke();
  }
}

/**
 * @param {CanvasRenderingContext2D} context - canvas
 * @param {GraphNode} node - hub node
 * @param {string} color - series color
 * @param {string} ground - ground color, the gap between the rings
 * @param {number} pixel - one screen pixel in world units
 */
export function paintModel(context: CanvasRenderingContext2D, node: GraphNode, color: string, ground: string, pixel: number): void {
  context.beginPath();
  context.arc(node.x, node.y, node.radius, 0, Math.PI * 2);
  context.fillStyle = ground;
  context.fill();
  context.strokeStyle = color;
  context.lineWidth = 1.5 * pixel;
  context.stroke();
  context.beginPath();
  context.arc(node.x, node.y, node.radius * 0.62, 0, Math.PI * 2);
  context.fillStyle = color;
  context.fill();
}

/**
 * @param {string} text - label
 * @param {number} length - longest length
 * @returns {string} - the text, cut short with an ellipsis
 */
export function shorten(text: string, length: number): string {
  return text.length > length ? `${text.slice(0, length - 1).trimEnd()}…` : text;
}
