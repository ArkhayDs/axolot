// Campaign shown across the synthesis and results pages, and the reactive loader of its results.

import { api, errorMessage } from './api';
import type { Campaign, CampaignDetail } from './api';
import { live } from './live.svelte';

export const picked = $state({ id: '', list: [] as Campaign[], error: '' });

/**
 * Default view: the most complete finished campaign, so a single case run live never replaces the full comparison
 * @param {Campaign[]} list - campaigns, newest first
 * @returns {string} - campaign id, empty when none
 */
function mostComplete(list: Campaign[]): string {
  const finished = list.filter((campaign) => campaign.status === 'done');
  const pool = finished.length > 0 ? finished : list.filter((campaign) => campaign.done_items > 0);
  return pool.reduce<Campaign | null>((best, campaign) => (!best || campaign.done_items > best.done_items ? campaign : best), null)?.id ?? '';
}

/** Refreshes the list and keeps the current pick while it still exists */
export async function refreshCampaigns(): Promise<void> {
  try {
    picked.list = await api.campaigns();
    picked.error = '';
    if (!picked.list.some((campaign) => campaign.id === picked.id)) picked.id = mostComplete(picked.list);
  } catch (error) {
    picked.error = errorMessage(error);
  }
}

/**
 * Keeps a campaign's results loaded, following new answers while it runs; call during component init
 * @param {() => string} id - reactive campaign id
 * @returns {{ detail: CampaignDetail | null; error: string }} - reactive results
 */
export function campaignResults(id: () => string): { detail: CampaignDetail | null; error: string } {
  const state: { detail: CampaignDetail | null; error: string } = $state({ detail: null, error: '' });
  $effect(() => {
    const current = id();
    void live.finished;
    if (live.campaign === current) void live.done;
    if (!current) {
      state.detail = null;
      return;
    }
    let stale = false;
    api
      .campaign(current)
      .then((detail) => {
        if (stale) return;
        state.detail = detail;
        state.error = '';
      })
      .catch((error: unknown) => {
        if (!stale) state.error = errorMessage(error);
      });
    return () => {
      stale = true;
    };
  });
  return state;
}
