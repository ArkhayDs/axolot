<script lang="ts">
  import { onMount } from 'svelte';

  import ModelName from './ModelName.svelte';
  import { score, seriesColor } from '../lib/format';
  import { live, onPulse } from '../lib/live.svelte';

  interface Point {
    x: number;
    y: number;
  }

  interface Particle {
    path: Point[];
    progress: number;
    color: string;
  }

  interface Ripple extends Point {
    age: number;
    color: string;
  }

  let { models, cases }: { models: string[]; cases: { id: string; title: string }[] } = $props();

  const HIDDEN_LAYERS = [5, 6];
  const MAX_PARTICLES = 140;
  const PARTICLE_SPEED = 0.028;
  const RIPPLE_SPEED = 0.03;

  let wrap: HTMLDivElement;
  let canvas: HTMLCanvasElement;
  let width = $state(0);
  let height = $state(0);
  const compact = $derived(width < 640);

  const caseAverages = $derived.by(() => {
    const totals = new Map<string, { sum: number; count: number }>();
    for (const run of live.runs) {
      const entry = totals.get(run.case_id) ?? { sum: 0, count: 0 };
      totals.set(run.case_id, { sum: entry.sum + run.score, count: entry.count + 1 });
    }
    return new Map([...totals].map(([id, entry]) => [id, entry.sum / entry.count]));
  });

  /**
   * Spreads nodes evenly over the height
   * @param {number} count - nodes in the column
   * @param {number} x - column position
   * @returns {Point[]} - node centers
   */
  function column(count: number, x: number): Point[] {
    const margin = 28;
    const step = (height - 2 * margin) / Math.max(count, 1);
    return Array.from({ length: count }, (_, index) => ({ x, y: margin + step * (index + 0.5) }));
  }

  const leftX = $derived(compact ? 96 : 190);
  const rightX = $derived(compact ? width - 24 : width - 270);
  const modelNodes = $derived(column(models.length, leftX));
  const caseNodes = $derived(column(cases.length, rightX));
  const hiddenNodes = $derived(
    HIDDEN_LAYERS.map((count, index) => column(count, leftX + ((rightX - leftX) * (index + 1)) / (HIDDEN_LAYERS.length + 1))),
  );

  let particles: Particle[] = [];
  let ripples: Ripple[] = [];
  let frame = 0;
  const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)');

  /**
   * Reads a design token, the canvas cannot use CSS variables directly
   * @param {string} value - color, possibly var(--token)
   * @returns {string} - resolved color
   */
  function resolve(value: string): string {
    const token = /^var\((--[\w-]+)\)$/.exec(value)?.[1];
    return token ? getComputedStyle(document.documentElement).getPropertyValue(token).trim() : value;
  }

  /**
   * @param {number} value - case score
   * @returns {string} - status token: the score is also written in the label
   */
  function scoreColor(value: number): string {
    if (value >= 90) return 'var(--good)';
    return value >= 60 ? 'var(--warning)' : 'var(--critical)';
  }

  /**
   * @param {Point} from - segment start
   * @param {Point} to - segment end
   * @param {string} color - stroke color
   * @param {number} alpha - stroke opacity
   */
  function line(context: CanvasRenderingContext2D, from: Point, to: Point, color: string, alpha: number): void {
    context.globalAlpha = alpha;
    context.strokeStyle = color;
    context.beginPath();
    context.moveTo(from.x, from.y);
    context.lineTo(to.x, to.y);
    context.stroke();
  }

  /**
   * @param {Point} point - node center
   * @param {number} radius - node radius
   * @param {string} color - fill color
   */
  function dot(context: CanvasRenderingContext2D, point: Point, radius: number, color: string): void {
    context.fillStyle = color;
    context.beginPath();
    context.arc(point.x, point.y, radius, 0, Math.PI * 2);
    context.fill();
  }

  /** Paints one frame, then keeps going only while something moves */
  function draw(): void {
    frame = 0;
    const context = canvas.getContext('2d');
    if (!context || width === 0) return;
    const ratio = window.devicePixelRatio || 1;
    context.setTransform(ratio, 0, 0, ratio, 0, 0);
    context.clearRect(0, 0, width, height);
    context.lineWidth = 1;

    const edge = resolve('var(--hairline)');
    const accent = resolve('var(--accent)');
    const activeModel = models.indexOf(live.model);
    const activeCase = cases.findIndex((item) => item.id === live.caseId);
    const layers = [modelNodes, ...hiddenNodes, caseNodes];
    for (let layer = 0; layer < layers.length - 1; layer += 1) {
      for (const [fromIndex, from] of layers[layer].entries()) {
        for (const [toIndex, to] of layers[layer + 1].entries()) {
          const lit = (layer === 0 && fromIndex === activeModel) || (layer === layers.length - 2 && toIndex === activeCase);
          line(context, from, to, lit ? accent : edge, lit ? 0.55 : 0.8);
        }
      }
    }

    context.globalAlpha = 1;
    for (const layer of hiddenNodes) for (const node of layer) dot(context, node, 4, resolve('var(--axis)'));
    for (const [index, node] of modelNodes.entries()) {
      if (index === activeModel) dot(context, node, 13, accent);
      dot(context, node, 9, resolve(seriesColor(models[index], [...models].sort())));
    }
    for (const [index, node] of caseNodes.entries()) {
      if (index === activeCase) dot(context, node, 11, accent);
      const average = caseAverages.get(cases[index].id);
      dot(context, node, 7, average === undefined ? resolve('var(--axis)') : resolve(scoreColor(average)));
    }

    for (const particle of particles) {
      const scaled = particle.progress * (particle.path.length - 1);
      const segment = Math.min(Math.floor(scaled), particle.path.length - 2);
      const from = particle.path[segment];
      const to = particle.path[segment + 1];
      const local = scaled - segment;
      dot(context, { x: from.x + (to.x - from.x) * local, y: from.y + (to.y - from.y) * local }, 2.5, particle.color);
      particle.progress += PARTICLE_SPEED;
    }
    particles = particles.filter((particle) => particle.progress < 1);

    for (const ripple of ripples) {
      context.globalAlpha = 1 - ripple.age;
      context.strokeStyle = ripple.color;
      context.lineWidth = 2;
      context.beginPath();
      context.arc(ripple.x, ripple.y, 8 + ripple.age * 34, 0, Math.PI * 2);
      context.stroke();
      ripple.age += RIPPLE_SPEED;
    }
    ripples = ripples.filter((ripple) => ripple.age < 1);
    context.globalAlpha = 1;

    if (particles.length > 0 || ripples.length > 0) schedule();
  }

  /** Queues a frame unless one is already pending */
  function schedule(): void {
    if (!frame) frame = requestAnimationFrame(draw);
  }

  /** Sends one impulse from the active model to the active case */
  function spawnToken(): void {
    const start = modelNodes[models.indexOf(live.model)];
    const end = caseNodes[cases.findIndex((item) => item.id === live.caseId)];
    if (!start || !end || particles.length >= MAX_PARTICLES) return;
    const through = hiddenNodes.map((layer) => layer[Math.floor(Math.random() * layer.length)]);
    particles.push({
      path: [start, ...through, end],
      progress: 0,
      color: resolve(seriesColor(live.model, [...models].sort())),
    });
  }

  onMount(() => {
    const observer = new ResizeObserver(([entry]) => {
      width = entry.contentRect.width;
      height = entry.contentRect.height;
    });
    observer.observe(wrap);
    const unsubscribe = onPulse((pulse) => {
      if (reducedMotion.matches) return;
      if (pulse.type === 'token') {
        spawnToken();
      } else {
        const node = caseNodes[cases.findIndex((item) => item.id === pulse.caseId)];
        if (node) ripples.push({ ...node, age: 0, color: resolve(pulse.failed ? 'var(--critical)' : scoreColor(pulse.score)) });
      }
      schedule();
    });
    return () => {
      observer.disconnect();
      unsubscribe();
      cancelAnimationFrame(frame);
    };
  });

  // Resizing a canvas clears it, so size and repaint follow the layout and the live state together.
  $effect(() => {
    const ratio = window.devicePixelRatio || 1;
    canvas.width = Math.round(width * ratio);
    canvas.height = Math.round(height * ratio);
    void [live.model, live.caseId, caseAverages, modelNodes, caseNodes];
    schedule();
  });
</script>

<div class="network" bind:this={wrap} role="group" aria-label="Réseau reliant {models.length} modèles à {cases.length} cas">
  <canvas bind:this={canvas} aria-hidden="true"></canvas>
  {#each models as model, index (model)}
    {#if modelNodes[index]}
      <span class="label label-model" style:top="{modelNodes[index].y}px" style:width="{leftX - 32}px">
        <ModelName name={compact ? model.split(':')[0] : model} />
      </span>
    {/if}
  {/each}
  {#each cases as item, index (item.id)}
    {@const average = caseAverages.get(item.id)}
    {#if caseNodes[index] && !compact}
      <span class="label label-case" class:active={live.caseId === item.id} style:top="{caseNodes[index].y}px" style:left="{rightX + 16}px">
        <span class="case-title">{item.title}</span>
        {#if average !== undefined}<span class="case-score">{score(average)}</span>{/if}
      </span>
    {/if}
  {/each}
</div>

<style>
  .network {
    position: relative;
    height: clamp(420px, 78vh, 760px);
    border: 1px solid var(--hairline);
    border-radius: var(--radius);
    background: var(--panel);
    overflow: hidden;
  }

  canvas {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
  }

  .label {
    position: absolute;
    transform: translateY(-50%);
    line-height: 1.25;
    pointer-events: none;
  }

  .label-model {
    left: var(--s2);
    text-align: right;
  }

  .label-case {
    display: flex;
    align-items: baseline;
    gap: var(--s2);
    width: 248px;
    color: var(--ink-2);
    font-size: var(--text-sm);
  }

  .label-case.active {
    color: var(--ink);
    font-weight: 600;
  }

  .case-title {
    display: -webkit-box;
    flex: 1;
    overflow: hidden;
    -webkit-box-orient: vertical;
    -webkit-line-clamp: 2;
    line-clamp: 2;
  }

  .case-score {
    color: var(--ink);
    font-weight: 600;
    font-variant-numeric: tabular-nums;
  }
</style>
