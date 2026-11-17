<script lang="ts" generics="T extends { key: string }">
  import type { Snippet } from 'svelte';

  let {
    title,
    items,
    chosen = $bindable(),
    row,
    note = '',
  }: { title: string; items: T[]; chosen: string[]; row: Snippet<[T]>; note?: string } = $props();

  const picked = $derived(items.filter((item) => chosen.includes(item.key)).length);
  const all = $derived(items.length > 0 && picked === items.length);

  /** Checks every item of the group, or unchecks them all when they already are */
  function toggle(): void {
    const keys = new Set(items.map((item) => item.key));
    chosen = all ? chosen.filter((key) => !keys.has(key)) : [...chosen.filter((key) => !keys.has(key)), ...keys];
  }
</script>

<div class="group">
  <label class="check head">
    <input type="checkbox" checked={all} indeterminate={picked > 0 && !all} disabled={items.length === 0} onchange={toggle} />
    <span class="title">
      <span class="name">{title}</span>
      {#if items.length === 1}
        <span class="sub">{@render row(items[0])}</span>
      {/if}
    </span>
    <span class="count">{note || (items.length > 1 ? `${picked}/${items.length}` : '')}</span>
  </label>
  {#if items.length > 1}
    <div class="items">
      {#each items as item (item.key)}
        <label class="check">
          <input type="checkbox" bind:group={chosen} value={item.key} />
          {@render row(item)}
        </label>
      {/each}
    </div>
  {/if}
</div>

<style>
  .group {
    display: grid;
    gap: var(--s2);
  }

  .check {
    display: flex;
    align-items: baseline;
    gap: var(--s2);
    cursor: pointer;
  }

  .check:has(:disabled) {
    cursor: default;
  }

  .title {
    display: grid;
    flex: 1;
    min-width: 0;
  }

  .name {
    font-weight: 600;
  }

  .sub {
    color: var(--ink-2);
    font-size: var(--text-sm);
  }

  .count {
    color: var(--ink-3);
    font-size: var(--text-sm);
    font-variant-numeric: tabular-nums;
  }

  /* Aligns item text under the group title, past the head checkbox. */
  .items {
    display: grid;
    gap: var(--s2);
    padding-left: calc(1em + var(--s2));
    font-size: var(--text-sm);
  }
</style>
