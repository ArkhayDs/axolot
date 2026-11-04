<script lang="ts">
  import type { Campaign } from '../lib/api';
  import { BASE_VIEW } from '../lib/campaigns.svelte';
  import { benchmarkName, statusLabel, when } from '../lib/format';

  let { value = $bindable(), campaigns, base = false }: { value: string; campaigns: Campaign[]; base?: boolean } = $props();

  const played = $derived(campaigns.filter((campaign) => campaign.done_items > 0));
</script>

{#if base || played.length > 0}
  <label class="field picker">
    {base ? 'Vue' : 'Historique des Benchmarks'}
    <select class="input" bind:value>
      {#if base}<option value={BASE_VIEW}>Toute la base</option>{/if}
      {#each played as campaign (campaign.id)}
        <option value={campaign.id}>
          {benchmarkName(campaign.name)}, {when(campaign.created_at)}{campaign.status === 'done' ? '' : `, ${statusLabel(campaign.status).toLowerCase()}`}
        </option>
      {/each}
    </select>
  </label>
{/if}

<style>
  .picker {
    width: min(100%, 26rem);
  }
</style>
