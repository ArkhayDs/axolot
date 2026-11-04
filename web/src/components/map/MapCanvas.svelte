<script lang="ts">
  import { onMount, untrack } from 'svelte';

  import { score as formatScore, seriesColor } from '../../lib/format';
  import { drawScene, resolve } from '../../lib/map/draw';
  import type { Particle, Ripple, View } from '../../lib/map/draw';
  import { settle, solve, tick } from '../../lib/map/force';
  import { buildGraph, nodeId } from '../../lib/map/graph';
  import type { Graph, GraphNode } from '../../lib/map/graph';
  import type { CellIndex, MapFilter, MapLists, RankSection } from '../../lib/map/ranking';
  import { live, onPulse } from '../../lib/live.svelte';

  let {
    lists,
    index,
    filter,
    sections,
    activeCategory,
    categoryOf,
    reserve,
    onpick,
  }: {
    lists: MapLists;
    index: CellIndex;
    filter: MapFilter;
    sections: RankSection[];
    activeCategory: string;
    categoryOf: (caseId: string) => string;
    reserve: () => { top: number; right: number; bottom: number; left: number };
    onpick: (node: GraphNode) => void;
  } = $props();

  const REFERENCE_AREA = 900 * 620;
  const MAX_PARTICLES = 140;
  const PARTICLE_SPEED = 0.03;
  const RIPPLE_SPEED = 0.018;
  const COOLING = 0.97;

  let wrap: HTMLDivElement;
  let canvas: HTMLCanvasElement;
  let width = $state(0);
  let height = $state(0);
  let graph: Graph = { nodes: [], links: [], springs: [], byId: new Map() };
  let view: View = { x: 0, y: 0, k: 1 };
  let viewMoved = false;
  // Raw: the draw compares nodes by identity, a deep proxy would never match them.
  let hover = $state.raw<GraphNode | null>(null);
  let dragged: GraphNode | null = null;
  let panning = false;
  let travelled = 0;
  let last = { x: 0, y: 0 };
  let alpha = 0;
  let particles: Particle[] = [];
  let ripples: Ripple[] = [];
  let raf = 0;
  const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)');

  /** @returns {{ scale: number; aspect: number }} - free area relative to the reference one */
  function freeArea(): { scale: number; aspect: number } {
    const free = reserve();
    const freeWidth = Math.max(width - free.left - free.right, 240);
    const freeHeight = Math.max(height - free.top - free.bottom, 240);
    return { scale: Math.min(Math.sqrt((freeWidth * freeHeight) / REFERENCE_AREA), 1.25), aspect: freeWidth / freeHeight };
  }

  /** Frames the whole graph in the area the panels leave free */
  function fit(): void {
    if (graph.nodes.length === 0 || width === 0) return;
    const free = reserve();
    const xs = graph.nodes.flatMap((node) => [node.x - Math.max(node.radius, node.labelWidth / 2), node.x + Math.max(node.radius, node.labelWidth / 2)]);
    const ys = graph.nodes.flatMap((node) => [node.y - node.radius, node.y + node.radius + node.labelHeight]);
    const [minX, maxX, minY, maxY] = [Math.min(...xs), Math.max(...xs), Math.min(...ys), Math.max(...ys)];
    const freeWidth = Math.max(width - free.left - free.right, 1);
    const freeHeight = Math.max(height - free.top - free.bottom, 1);
    const k = Math.min(freeWidth / (maxX - minX + 32), freeHeight / (maxY - minY + 32), 1.3);
    view = { k, x: free.left + freeWidth / 2 - (k * (minX + maxX)) / 2, y: free.top + freeHeight / 2 - (k * (minY + maxY)) / 2 };
    viewMoved = false;
  }

  /** Rebuilds the graph from the data; a node that appears is the only thing allowed to move the others */
  function rebuild(): void {
    const context = canvas.getContext('2d');
    if (!context || width === 0) return;
    context.font = `650 13px ${resolve('var(--font)')}`;
    const known = graph.byId;
    const extended: MapLists = {
      machines: lists.machines,
      models: [...new Set([...lists.models, ...(live.model ? [live.model] : [])])].sort(),
      themes: [...new Set([...lists.themes, ...(activeCategory ? [activeCategory] : [])])].sort(),
    };
    graph = buildGraph(extended, index, known, (text) => context.measureText(text).width, width < 640 ? 1.4 : 1);
    const { scale, aspect } = freeArea();
    if (graph.nodes.some((node) => !known.has(node.id))) {
      if (known.size === 0) {
        solve(graph.nodes, graph.springs, scale, aspect);
      } else {
        settle(graph.nodes);
      }
      if (!viewMoved) fit();
    }
    schedule();
  }

  /** Paints one frame, then keeps going only while something really moves */
  function frame(): void {
    raf = 0;
    const context = canvas.getContext('2d');
    if (!context || width === 0) return;
    if (alpha > 0.01) {
      const { scale, aspect } = freeArea();
      tick(graph.nodes, graph.springs, alpha, scale, aspect);
      alpha *= COOLING;
    }
    for (const particle of particles) particle.progress += PARTICLE_SPEED;
    particles = particles.filter((particle) => particle.progress < 1);
    for (const ripple of ripples) ripple.age += RIPPLE_SPEED;
    ripples = ripples.filter((ripple) => ripple.age < 1);
    drawScene(context, {
      width,
      height,
      view,
      graph,
      index,
      lists,
      filter,
      sections,
      hover,
      activeModel: live.model,
      activeCategory,
      particles,
      ripples,
    });
    if (alpha > 0.01 || particles.length > 0 || ripples.length > 0) schedule();
  }

  /** Queues a frame unless one is already pending */
  function schedule(): void {
    if (!raf) raf = requestAnimationFrame(frame);
  }

  /**
   * @param {PointerEvent | WheelEvent} event - pointer event
   * @returns {{ x: number; y: number }} - position in graph coordinates
   */
  function toWorld(event: PointerEvent | WheelEvent): { x: number; y: number } {
    const box = canvas.getBoundingClientRect();
    return { x: (event.clientX - box.left - view.x) / view.k, y: (event.clientY - box.top - view.y) / view.k };
  }

  /**
   * @param {PointerEvent} event - pointer event
   * @returns {GraphNode | null} - node under the pointer, its label included
   */
  function nodeAt(event: PointerEvent): GraphNode | null {
    const { x, y } = toWorld(event);
    return (
      graph.nodes.find(
        (node) => Math.abs(x - node.x) <= Math.max(node.radius, node.labelWidth / 2) && y >= node.y - node.radius && y <= node.y + node.radius + node.labelHeight,
      ) ?? null
    );
  }

  /** @param {PointerEvent} event - starts dragging a node, or panning on the background */
  function onPointerDown(event: PointerEvent): void {
    canvas.setPointerCapture(event.pointerId);
    travelled = 0;
    last = { x: event.clientX, y: event.clientY };
    dragged = nodeAt(event);
    panning = dragged === null;
    if (dragged) dragged.pinned = true;
  }

  /** @param {PointerEvent} event - moves what is held, otherwise updates the hover */
  function onPointerMove(event: PointerEvent): void {
    if (!dragged && !panning) {
      const under = nodeAt(event);
      if (under !== hover) {
        hover = under;
        schedule();
      }
      return;
    }
    travelled += Math.abs(event.clientX - last.x) + Math.abs(event.clientY - last.y);
    if (dragged) {
      const { x, y } = toWorld(event);
      dragged.x = x;
      dragged.y = y;
      if (!reducedMotion.matches) alpha = Math.max(alpha, 0.22);
    } else {
      view.x += event.clientX - last.x;
      view.y += event.clientY - last.y;
      viewMoved = true;
    }
    last = { x: event.clientX, y: event.clientY };
    schedule();
  }

  /** @param {PointerEvent} event - a press that barely moved counts as a click */
  function onPointerUp(event: PointerEvent): void {
    if (dragged) {
      dragged.pinned = false;
      if (travelled < 5) onpick(dragged);
      else settle(graph.nodes);
    }
    dragged = null;
    panning = false;
    schedule();
    canvas.releasePointerCapture(event.pointerId);
  }

  /** @param {WheelEvent} event - zooms around the pointer */
  function onWheel(event: WheelEvent): void {
    event.preventDefault();
    const { x, y } = toWorld(event);
    const k = Math.min(Math.max(view.k * Math.exp(-event.deltaY * 0.0015), 0.35), 3);
    const box = canvas.getBoundingClientRect();
    view = { k, x: event.clientX - box.left - x * k, y: event.clientY - box.top - y * k };
    viewMoved = true;
    schedule();
  }

  onMount(() => {
    const observer = new ResizeObserver(([entry]) => {
      width = entry.contentRect.width;
      height = entry.contentRect.height;
    });
    observer.observe(wrap);
    canvas.addEventListener('wheel', onWheel, { passive: false });
    const unsubscribe = onPulse((pulse) => {
      if (reducedMotion.matches) return;
      if (pulse.type === 'token') {
        const from = graph.byId.get(nodeId('model', live.model));
        const to = graph.byId.get(nodeId('theme', activeCategory));
        if (from && to && particles.length < MAX_PARTICLES) {
          particles.push({ from, to, progress: 0, color: resolve(seriesColor(live.model, lists.models)) });
        }
      } else {
        const node = graph.byId.get(nodeId('theme', categoryOf(pulse.caseId)));
        const tone = pulse.failed || pulse.score < 60 ? 'var(--critical)' : pulse.score >= 90 ? 'var(--good)' : 'var(--warning)';
        if (node) ripples.push({ node, age: 0, color: resolve(tone), text: pulse.failed ? 'Échec' : formatScore(pulse.score) });
      }
      schedule();
    });
    return () => {
      observer.disconnect();
      unsubscribe();
      cancelAnimationFrame(raf);
    };
  });

  // Resizing a canvas clears it, so its size, the framing and the repaint follow the layout together.
  $effect(() => {
    const ratio = window.devicePixelRatio || 1;
    canvas.width = Math.round(width * ratio);
    canvas.height = Math.round(height * ratio);
    untrack(() => {
      if (graph.nodes.length === 0) rebuild();
      else if (!viewMoved) fit();
      schedule();
    });
  });

  $effect(() => {
    void [lists, index, live.model, activeCategory];
    untrack(rebuild);
  });

  $effect(() => {
    void [filter.machine, filter.model, filter.category, sections, hover];
    schedule();
  });
</script>

<div class="stage" bind:this={wrap}>
  <canvas
    bind:this={canvas}
    aria-hidden="true"
    class:pointing={hover !== null}
    onpointerdown={onPointerDown}
    onpointermove={onPointerMove}
    onpointerup={onPointerUp}
    onpointercancel={onPointerUp}
    onpointerleave={() => {
      if (hover) hover = null;
    }}
    ondblclick={() => {
      fit();
      schedule();
    }}
  ></canvas>
</div>

<style>
  .stage {
    position: absolute;
    inset: 0;
  }

  canvas {
    display: block;
    width: 100%;
    height: 100%;
    cursor: grab;
    touch-action: none;
  }

  canvas.pointing {
    cursor: pointer;
  }

  /* The phone page scrolls past the map: only horizontal gestures belong to it. */
  @media (max-width: 760px) {
    .stage {
      position: relative;
      inset: auto;
      height: 62vh;
    }

    canvas {
      touch-action: pan-y;
    }
  }
</style>
