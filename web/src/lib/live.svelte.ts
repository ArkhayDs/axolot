// Single event-stream connection, exposed as reactive state shared by every page.

import { api } from './api';
import type { Pull, Run } from './api';

export interface Ingest {
  done: number;
  total: number;
  finished: boolean;
  error: string;
}

export const live = $state({
  // Optimistic until the stream errors, so a normal page load never flashes as offline.
  connected: true,
  campaign: '',
  caseId: '',
  item: '',
  model: '',
  output: '',
  tokens: 0,
  done: 0,
  total: 0,
  runs: [] as Run[],
  // Bumped when a campaign ends, so pages reload their results.
  finished: 0,
  // Bumped when the stream comes back after a loss, so pages reload what they missed.
  reconnects: 0,
  pulls: {} as Record<string, Pull>,
  ingest: null as Ingest | null,
});

export type Pulse = { type: 'token' } | { type: 'score'; caseId: string; itemId: string; score: number; failed: boolean };

const RECONNECT_MS = 2000;

const pulseListeners = new Set<(pulse: Pulse) => void>();

/**
 * Subscribes to token and score moments, the canvas animates outside Svelte's reactivity
 * @param {(pulse: Pulse) => void} listener - called on each moment
 * @returns {() => void} - unsubscribe
 */
export function onPulse(listener: (pulse: Pulse) => void): () => void {
  pulseListeners.add(listener);
  return () => pulseListeners.delete(listener);
}

/**
 * Loads progress of a campaign already underway, after a page load or a missed start
 * @param {string} id - campaign id
 */
async function attach(id: string): Promise<void> {
  live.campaign = id;
  const detail = await api.campaign(id);
  if (live.campaign !== id) return;
  live.total = detail.campaign.total_items;
  live.runs = detail.runs;
  live.done = detail.runs.length;
}

/** Clears the campaign part of the state */
function reset(): void {
  live.campaign = '';
  live.caseId = '';
  live.item = '';
  live.model = '';
  live.output = '';
  live.tokens = 0;
  live.done = 0;
  live.total = 0;
  live.runs = [];
}

/**
 * Applies one server event
 * @param {Record<string, unknown>} event - decoded payload, discriminated by type
 */
function apply(event: Record<string, unknown>): void {
  switch (event.type) {
    case 'item_start':
      if (live.campaign !== event.campaign) void attach(event.campaign as string).catch(() => undefined);
      live.caseId = event.case as string;
      live.item = event.item as string;
      live.model = event.model as string;
      live.output = '';
      live.tokens = 0;
      break;
    case 'delta':
      live.output += event.text as string;
      live.tokens += 1;
      pulseListeners.forEach((listener) => listener({ type: 'token' }));
      break;
    case 'item_end': {
      const run = event.run as Run;
      live.runs.push(run);
      live.done = live.runs.length;
      pulseListeners.forEach((listener) =>
        listener({ type: 'score', caseId: run.case_id, itemId: run.item_id, score: run.score, failed: run.error !== '' }),
      );
      break;
    }
    case 'campaign_end':
      reset();
      live.finished += 1;
      break;
    case 'pull':
      live.pulls[event.name as string] = event as unknown as Pull;
      break;
    case 'ingest':
      live.ingest = event as unknown as Ingest;
      break;
  }
}

/**
 * Opens the stream; status resynchronizes the campaign on every opening.
 * EventSource retries by itself after a network error but gives up on an HTTP error, which is what a proxy answers while the app restarts.
 */
export function connectLive(): void {
  const source = new EventSource('/api/events');
  source.onopen = () => {
    if (!live.connected) live.reconnects += 1;
    live.connected = true;
    void api
      .status()
      .then((status) => {
        if (status.active_campaign) return attach(status.active_campaign);
        if (live.campaign) {
          reset();
          live.finished += 1;
        }
      })
      .catch(() => undefined);
  };
  source.onerror = () => {
    live.connected = false;
    if (source.readyState === EventSource.CLOSED) setTimeout(connectLive, RECONNECT_MS);
  };
  source.onmessage = (message: MessageEvent<string>) => apply(JSON.parse(message.data) as Record<string, unknown>);
}

/**
 * Starts a campaign and shows it right away, without waiting for its first event
 * @param {string} name - campaign name
 * @param {string[]} cases - case ids
 * @returns {Promise<string>} - campaign id
 */
export async function launch(name: string, cases: string[]): Promise<string> {
  const { id } = await api.startCampaign(name, cases);
  await attach(id);
  return id;
}
