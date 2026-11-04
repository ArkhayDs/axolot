<script lang="ts">
  import Icon from './Icon.svelte';

  let { message }: { message: string } = $props();
</script>

{#if message}
  <p class="notice" role="alert">
    <span class="state-critical"><Icon name="alert" /></span>
    <span>{message}</span>
  </p>
{/if}
