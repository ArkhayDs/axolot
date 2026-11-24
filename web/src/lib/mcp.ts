// Connection state and transition timing shared by the MCP panel views.

import { cubicOut } from 'svelte/easing';

import type { McpProbe } from './api';

type IconName = 'check' | 'alert' | 'cross';

// Svelte transitions run in JS, out of reach of the global reduced-motion CSS rule.
export const motion = { duration: window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 0 : 180, easing: cubicOut };

/**
 * Connection state shown beside the server
 * @param {McpProbe | undefined} probe - server's answer, undefined while it is asked
 * @returns {{ label: string; tone: string; icon?: IconName }} - label, its class and icon
 */
export function stateOf(probe: McpProbe | undefined): { label: string; tone: string; icon?: IconName } {
  if (!probe) return { label: 'Vérification...', tone: 'muted' };
  if (probe.login) return { label: 'Connexion requise', tone: 'state-warning', icon: 'alert' };
  if (probe.error) return { label: 'Injoignable', tone: 'state-critical', icon: 'cross' };
  return { label: 'Connecté', tone: 'state-good', icon: 'check' };
}
