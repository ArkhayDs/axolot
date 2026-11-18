<script lang="ts">
  import { onMount, untrack } from 'svelte';

  import type { ConnectionType } from '../../lib/api';
  import { score as formatScore, seriesColor } from '../../lib/format';
  import { applyMoves, planMoves } from '../../lib/map/arrange';
  import type { Move } from '../../lib/map/arrange';
  import type { MapContent } from '../../lib/map/content';
  import { drawScene } from '../../lib/map/draw';
  import type { Particle, Ripple, View } from '../../lib/map/draw';
  import { activeRoute, selectionOf } from '../../lib/map/figures';
  import { settle, solve } from '../../lib/map/force';
  import { buildGraph, nodeId } from '../../lib/map/graph';
  import type { Graph, GraphNode, NodeKind } from '../../lib/map/graph';
  import { resolve } from '../../lib/map/paint';
  import type { CellIndex, MapFilter, RankSection } from '../../lib/map/ranking';
  import { resultsOfRuns } from '../../lib/map/results';
  import type { ItemIndex } from '../../lib/map/results';
  import { closestNode, frameNodes } from '../../lib/map/view';
  import { live, onPulse } from '../../lib/live.svelte';

  let {
    content,
    cells,
    items,
    filter,
    connection,
    focus,
    sections,
    reserve,
    onpick,
    onclear,
  }: {
    content: MapContent;
    cells: CellIndex;
    items: ItemIndex;
    filter: MapFilter;
    connection: ConnectionType;
    focus: { kind: NodeKind; key: string } | null;
    sections: RankSection[];
    reserve: () => { top: number; right: number; bottom: number; left: number };
    onpick: (node: GraphNode) => void;
    onclear: () => void;
  } = $props();

  const MAX_PARTICLES = 140;
  const PARTICLE_SPEED = 0.022;
  const RIPPLE_SPEED = 0.018;
  const MIN_ZOOM = 0.35;
  const MAX_ZOOM = 6;
  const FIT_ZOOM = 1.5;
  const GRAB_PIXELS = 7;
  const ARRANGE_MS = 500;

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
  let particles: Particle[] = [];
  let ripples: Ripple[] = [];
  let raf = 0;
  let moves: Move[] = [];
  let movesStart = 0;
  let arranged = false;
  const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)');

  // The live run of another connection type stays off the graph, its figures are not on it either.
  const liveShown = $derived(!connection || live.connection === connection);
  const newLiveModel = $derived(liveShown && !content.models.includes(live.model) ? live.model : '');
  const liveResults = $derived(resultsOfRuns(connection ? live.runs.filter((run) => run.connection_type === connection) : live.runs));

  /** Frames the whole graph in the area the panels leave free */
  function fit(): void {
    if (graph.nodes.length === 0 || width === 0) return;
    view = frameNodes(graph.nodes, reserve(), width, height, MIN_ZOOM, FIT_ZOOM);
    viewMoved = false;
  }

  /** Back to the framed view */
  export function recenter(): void {
    fit();
    schedule();
  }

  /** Rebuilds the graph from the data; the layout runs once, a node that appears only pushes its neighbours aside */
  function rebuild(): void {
    const context = canvas.getContext('2d');
    if (!context || width === 0) return;
    context.font = `650 12.5px ${resolve('var(--font)')}`;
    const known = graph.byId;
    const models = newLiveModel ? [...content.models, newLiveModel].sort() : content.models;
    const free = reserve();
    const aspect = Math.max(width - free.left - free.right, 240) / Math.max(height - free.top - free.bottom, 240);
    graph = buildGraph({ ...content, models }, cells, items, known, aspect, (text) => context.measureText(text).width);
    hover = null;
    dragged = null;
    if (graph.nodes.some((node) => !known.has(node.id))) {
      if (known.size === 0) {
        solve(graph.nodes, graph.springs);
        for (const node of graph.nodes) {
          node.homeX = node.x;
          node.homeY = node.y;
        }
      } else {
        settle(graph.nodes);
      }
      if (!viewMoved) fit();
    }
    schedule();
  }

  /** Follows the selection: distance says the score, no selection means the default layout */
  function arrange(): void {
    const selective = Boolean(filter.model || filter.category);
    if (graph.nodes.length === 0 || (!selective && !arranged)) return;
    arranged = selective;
    moves = planMoves(graph.nodes, selectionOf(filter, cells));
    movesStart = performance.now();
    if (reducedMotion.matches) {
      applyMoves(moves, 1);
      moves = [];
    }
    schedule();
  }

  /** Paints one frame, then keeps going only while something really moves */
  function frame(): void {
    raf = 0;
    const context = canvas.getContext('2d');
    if (!context || width === 0) return;
    if (moves.length > 0) {
      const progress = Math.min((performance.now() - movesStart) / ARRANGE_MS, 1);
      applyMoves(moves, progress);
      if (progress === 1) moves = [];
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
      content,
      cells,
      items,
      filter,
      focus: focus ? (graph.byId.get(nodeId(focus.kind, focus.key)) ?? null) : null,
      sections,
      hover,
      activePath: liveShown ? activeRoute(graph, content, live) : [],
      liveResults,
      particles,
      ripples,
    });
    if (moves.length > 0 || particles.length > 0 || ripples.length > 0) schedule();
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
   * @returns {GraphNode | null} - node under the pointer
   */
  function nodeAt(event: PointerEvent): GraphNode | null {
    const { x, y } = toWorld(event);
    return closestNode(graph.nodes, x, y, view.k, GRAB_PIXELS);
  }

  /** @param {PointerEvent} event - starts dragging a node, or panning on the background */
  function onPointerDown(event: PointerEvent): void {
    canvas.setPointerCapture(event.pointerId);
    travelled = 0;
    last = { x: event.clientX, y: event.clientY };
    dragged = nodeAt(event);
    if (dragged) moves = [];
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
    if (dragged && travelled >= 5) {
      const { x, y } = toWorld(event);
      dragged.x = x;
      dragged.y = y;
    } else if (panning) {
      view.x += event.clientX - last.x;
      view.y += event.clientY - last.y;
      viewMoved = travelled >= 5 || viewMoved;
    }
    last = { x: event.clientX, y: event.clientY };
    schedule();
  }

  /** @param {PointerEvent} event - a press that barely moved counts as a click */
  function onPointerUp(event: PointerEvent): void {
    if (dragged) {
      dragged.pinned = false;
      if (travelled < 5) onpick(dragged);
    } else if (panning && travelled < 5) {
      onclear();
    }
    dragged = null;
    panning = false;
    schedule();
    if (canvas.hasPointerCapture(event.pointerId)) canvas.releasePointerCapture(event.pointerId);
  }

  /**
   * Two-finger scroll pans; a trackpad pinch, sent by browsers as a wheel event with ctrlKey, zooms around the pointer
   * @param {WheelEvent} event - wheel or trackpad gesture
   */
  function onWheel(event: WheelEvent): void {
    event.preventDefault();
    if (!event.ctrlKey) {
      view = { ...view, x: view.x - event.deltaX, y: view.y - event.deltaY };
      viewMoved = true;
      schedule();
      return;
    }
    const { x, y } = toWorld(event);
    const k = Math.min(Math.max(view.k * Math.exp(-event.deltaY * 0.01), MIN_ZOOM), MAX_ZOOM);
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
      if (!liveShown) return;
      if (reducedMotion.matches) {
        schedule();
        return;
      }
      if (pulse.type === 'token') {
        const path = activeRoute(graph, content, live);
        if (path.length > 0 && particles.length < MAX_PARTICLES) particles.push({ path, progress: 0, color: resolve(seriesColor(live.model, content.models)) });
      } else {
        const node = graph.byId.get(nodeId('question', `${pulse.caseId}/${pulse.itemId}`));
        const tone = pulse.unmeasured ? 'var(--axis)' : pulse.score < 60 ? 'var(--critical)' : pulse.score >= 90 ? 'var(--good)' : 'var(--warning)';
        if (node) ripples.push({ node, age: 0, color: resolve(tone), text: pulse.unmeasured ? 'Non mesurée' : formatScore(pulse.score) });
      }
      schedule();
    });
    return () => {
      observer.disconnect();
      unsubscribe();
      cancelAnimationFrame(raf);
      raf = 0;
      moves = [];
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

  // Only what changes the set of nodes: a live event never reaches this.
  $effect(() => {
    void [content, cells, items, newLiveModel];
    untrack(rebuild);
  });

  $effect(() => {
    void [filter.machine, filter.model, filter.category, cells];
    untrack(arrange);
  });

  $effect(() => {
    void [filter.machine, filter.model, filter.category, focus, sections, hover, liveResults, liveShown, live.model, live.item];
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
    onpointerleave={() => (hover = null)}
    ondblclick={recenter}
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
