<script lang="ts">
  import { live } from '../lib/live.svelte';
  import Icon from './Icon.svelte';
  import type { IconName } from './Icon.svelte';
  import Logo from './Logo.svelte';

  let { active }: { active: string } = $props();

  const LINKS: { route: string; label: string; icon: IconName }[] = [
    { route: '', label: 'Direction', icon: 'target' },
    { route: 'labo', label: 'Labo', icon: 'flask' },
    { route: 'cas', label: 'Cas', icon: 'list' },
    { route: 'chat', label: 'Chat', icon: 'chat' },
    { route: 'resultats', label: 'Résultats', icon: 'bars' },
    { route: 'reglages', label: 'Réglages', icon: 'sliders' },
  ];
</script>

<header class="bar glass">
  <a class="brand" href="#/" aria-label="Axolot, accueil">
    <Logo active={live.campaign !== ''} />
    <span>Axolot</span>
  </a>
  <nav aria-label="Navigation principale">
    {#each LINKS as link (link.route)}
      <a href="#/{link.route}" class:current={active === link.route} aria-current={active === link.route ? 'page' : undefined}>
        <Icon name={link.icon} size={20} />
        <span>{link.label}</span>
      </a>
    {/each}
  </nav>
  {#if live.campaign}
    <a class="running" href="#/labo" aria-label="Campagne en cours, {live.done} sur {live.total} réponses">
      <span class="running-count">{live.done}/{live.total}</span>
      <span class="progress"><span style:width="{live.total ? (100 * live.done) / live.total : 0}%"></span></span>
    </a>
  {:else if !live.connected}
    <span class="state state-warning"><Icon name="alert" size={16} /> Hors ligne</span>
  {/if}
</header>

<style>
  .bar {
    position: sticky;
    top: var(--s3);
    z-index: 10;
    display: flex;
    align-items: center;
    gap: var(--s5);
    width: min(calc(100% - 2 * var(--s5)), var(--page-width));
    margin: var(--s3) auto 0;
    padding: var(--s2) var(--s4);
    border-radius: var(--radius-lg);
  }

  .brand {
    display: flex;
    align-items: center;
    gap: var(--s2);
    font-size: var(--text-lg);
    font-weight: 650;
    letter-spacing: -0.01em;
    text-decoration: none;
  }

  nav {
    display: flex;
    flex: 1;
    gap: var(--s1);
  }

  nav a {
    display: flex;
    align-items: center;
    gap: var(--s2);
    min-height: 40px;
    padding: var(--s1) var(--s3);
    border-radius: var(--radius);
    color: var(--ink-2);
    text-decoration: none;
    transition:
      background-color 0.15s,
      color 0.15s;
  }

  nav a:hover {
    color: var(--ink);
    background: color-mix(in srgb, var(--ink) 8%, transparent);
  }

  nav a.current {
    color: var(--ink);
    background: color-mix(in srgb, var(--accent) 20%, transparent);
  }

  .running {
    display: grid;
    gap: var(--s1);
    min-width: 96px;
    font-size: var(--text-sm);
    font-variant-numeric: tabular-nums;
    text-decoration: none;
  }

  /* The bar has no room for the wordmark and five labels on a phone: it splits into a bottom tab bar. */
  @media (max-width: 760px) {
    .bar {
      position: fixed;
      inset: auto var(--s2) var(--s2);
      width: auto;
      margin: 0;
      padding: var(--s1);
      gap: var(--s1);
    }

    .brand,
    .running,
    .state {
      display: none;
    }

    nav a {
      flex: 1;
      flex-direction: column;
      gap: 2px;
      min-height: 52px;
      padding: var(--s1);
      font-size: 0.6875rem;
      justify-content: center;
    }
  }
</style>
