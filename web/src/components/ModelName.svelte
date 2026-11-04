<script lang="ts">
  let { name, color = '', short = false }: { name: string; color?: string; short?: boolean } = $props();

  const parts = $derived(name.split(':'));
</script>

<span class="model">
  {#if color}<span class="dot" style:background={color}></span>{/if}
  <span><strong>{parts[0]}</strong>{#if parts[1] && !short}<span class="tag">:{parts[1]}</span>{/if}</span>
</span>

<style>
  .model {
    display: inline-flex;
    align-items: baseline;
    gap: var(--s2);
    font-family: var(--mono);
    font-size: var(--text-sm);
    overflow-wrap: anywhere;
  }

  strong {
    font-weight: 600;
  }

  .tag {
    color: var(--ink-3);
  }
</style>
