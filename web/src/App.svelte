<script lang="ts">
  import { flushSync, onMount } from 'svelte';

  import Nav from './components/Nav.svelte';
  import { refreshCampaigns } from './lib/campaigns.svelte';
  import { connectLive, live } from './lib/live.svelte';
  import CaseDetail from './pages/CaseDetail.svelte';
  import CaseList from './pages/CaseList.svelte';
  import Chat from './pages/Chat.svelte';
  import Direction from './pages/Direction.svelte';
  import Labo from './pages/Labo.svelte';
  import Results from './pages/Results.svelte';
  import Settings from './pages/Settings.svelte';

  /**
   * @returns {string[]} - hash route segments
   */
  function readRoute(): string[] {
    return location.hash.replace(/^#\/?/, '').split('/').filter(Boolean).map(decodeURIComponent);
  }

  let route = $state<string[]>(readRoute());

  /** Follows the hash, through a view transition where the browser has one */
  function onHashChange(): void {
    const apply = (): void => {
      route = readRoute();
      window.scrollTo(0, 0);
    };
    if (document.startViewTransition) document.startViewTransition(() => flushSync(apply));
    else apply();
  }

  onMount(() => {
    connectLive();
    void refreshCampaigns();
    window.addEventListener('hashchange', onHashChange);
    return () => window.removeEventListener('hashchange', onHashChange);
  });

  $effect(() => {
    if (live.finished > 0) void refreshCampaigns();
  });
</script>

<Nav active={route[0] ?? ''} />

<main>
  {#if route[0] === 'labo'}
    <Labo />
  {:else if route[0] === 'cas' && route[1]}
    {#key route[1]}
      <CaseDetail id={route[1]} />
    {/key}
  {:else if route[0] === 'cas'}
    <CaseList />
  {:else if route[0] === 'chat'}
    <Chat />
  {:else if route[0] === 'resultats'}
    <Results />
  {:else if route[0] === 'reglages'}
    <Settings />
  {:else}
    <Direction />
  {/if}
</main>
