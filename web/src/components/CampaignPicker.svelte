<script lang="ts">
  import type { Campaign } from '../lib/api';
  import { statusLabel, when } from '../lib/format';

  let { value = $bindable(), campaigns }: { value: string; campaigns: Campaign[] } = $props();
</script>

{#if campaigns.length > 0}
  <label class="field picker">
    Historique des campagnes
    <select class="input" bind:value>
      {#each campaigns as campaign (campaign.id)}
        <option value={campaign.id}>
          {campaign.name}, {when(campaign.created_at)}{campaign.status === 'done' ? '' : `, ${statusLabel(campaign.status).toLowerCase()}`}
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
