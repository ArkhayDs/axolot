<script lang="ts">
  import type { Campaign } from '../lib/api';
  import { benchmarkName, statusLabel, when } from '../lib/format';

  let { value = $bindable(), campaigns }: { value: string; campaigns: Campaign[] } = $props();

  const played = $derived(campaigns.filter((campaign) => campaign.done_items > 0));
</script>

{#if played.length > 0}
  <label class="field picker">
    Historique des Benchmarks
    <select class="input" bind:value>
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
