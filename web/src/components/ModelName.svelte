<script lang="ts">
  import { modelParts } from '../lib/format';

  let { name, color = '', short = false }: { name: string; color?: string; short?: boolean } = $props();

  const parts = $derived(modelParts(name));
</script>

<span class="model">
  {#if color}<span class="dot" style:background={color}></span>{/if}
  <span><strong>{parts.family}</strong>{#if !short}<span class="tag">{parts.tag}</span>{/if}<span class="tag">{parts.via}</span></span>
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
