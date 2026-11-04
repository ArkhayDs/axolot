// Screen framing and pointer lookup of the map.

import type { GraphNode } from './graph';

const HUB_LABEL_HEIGHT = 30;

/**
 * Frames the default layout in the area the panels leave free, so a rearrangement never reframes; the zoom floor also bounds the labels, drawn at 1/zoom
 * @param {GraphNode[]} nodes - nodes, not empty
 * @param {{ top: number; right: number; bottom: number; left: number }} free - margins the panels cover
 * @param {number} width - canvas width
 * @param {number} height - canvas height
 * @param {number} minZoom - smallest zoom
 * @param {number} maxZoom - largest zoom when framing
 * @returns {{ x: number; y: number; k: number }} - view
 */
export function frameNodes(nodes: GraphNode[], free: { top: number; right: number; bottom: number; left: number }, width: number, height: number, minZoom: number, maxZoom: number): { x: number; y: number; k: number } {
  let [minX, maxX, minY, maxY] = [Infinity, -Infinity, Infinity, -Infinity];
  for (const node of nodes) {
    const halfHeight = Math.max(node.radius, node.textWidth > 0 ? HUB_LABEL_HEIGHT / 2 : 0);
    minX = Math.min(minX, node.homeX - node.radius);
    maxX = Math.max(maxX, node.homeX + node.radius + node.textWidth);
    minY = Math.min(minY, node.homeY - halfHeight);
    maxY = Math.max(maxY, node.homeY + halfHeight);
  }
  const freeWidth = Math.max(width - free.left - free.right, 1);
  const freeHeight = Math.max(height - free.top - free.bottom, 1);
  const k = Math.max(Math.min(freeWidth / (maxX - minX + 24), freeHeight / (maxY - minY + 24), maxZoom), minZoom) || 1;
  return { k, x: free.left + freeWidth / 2 - (k * (minX + maxX)) / 2, y: free.top + freeHeight / 2 - (k * (minY + maxY)) / 2 };
}

/**
 * @param {GraphNode[]} nodes - nodes
 * @param {number} x - pointer in graph coordinates
 * @param {number} y - pointer in graph coordinates
 * @param {number} zoom - current zoom
 * @param {number} grabPixels - grabbing distance on screen, small points stay easy to hit
 * @returns {GraphNode | null} - closest node within reach, a hub's label included
 */
export function closestNode(nodes: GraphNode[], x: number, y: number, zoom: number, grabPixels: number): GraphNode | null {
  const grab = grabPixels / zoom;
  let closest: GraphNode | null = null;
  let best = Infinity;
  for (const node of nodes) {
    const distance = Math.hypot(x - node.x, y - node.y);
    const onLabel = node.textWidth > 0 && x >= node.x && x <= node.x + node.radius + node.textWidth / zoom && Math.abs(y - node.y) <= HUB_LABEL_HEIGHT / 2 / zoom;
    if ((distance <= Math.max(node.radius, grab) || onLabel) && distance < best) {
      closest = node;
      best = distance;
    }
  }
  return closest;
}
