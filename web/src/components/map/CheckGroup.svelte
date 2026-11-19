<script lang="ts" generics="T extends { key: string }">
  import { untrack } from 'svelte';
  import type { Snippet } from 'svelte';
  import Icon from '../Icon.svelte';

  let {
    title,
    items,
    chosen = $bindable(),
    row,
    note = '',
  }: { title: string; items: T[]; chosen: string[]; row: Snippet<[T]>; note?: string } = $props();

  const picked = $derived(items.filter((item) => chosen.includes(item.key)).length);
  const all = $derived(items.length > 0 && picked === items.length);
  // Opens on what is already checked; closed groups keep their checkbox and count in view.
  let open = $state(untrack(() => picked > 0));

  /** Checks every item of the group, or unchecks them all when they already are */
  function toggle(): void {
    const keys = new Set(items.map((item) => item.key));
    chosen = all ? chosen.filter((key) => !keys.has(key)) : [...chosen.filter((key) => !keys.has(key)), ...keys];
  }

  /**
   * Adds or removes one item, keeping picks of sibling groups bound to the same list
   * @param {string} key - item key
   * @param {boolean} on - checked state
   */
  function flip(key: string, on: boolean): void {
    chosen = on ? [...chosen, key] : chosen.filter((k) => k !== key);
  }
</script>

<div class="group">
  <div class="head">
    <label class="check">
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
      <button class="fold" type="button" aria-expanded={open} aria-label={`${open ? 'Replier' : 'Déplier'} ${title}`} onclick={() => (open = !open)}>
        <Icon name="chevron" size={16} />
      </button>
    {/if}
  </div>
  {#if items.length > 1 && open}
    <div class="items">
      {#each items as item (item.key)}
        <label class="check">
          <input type="checkbox" checked={chosen.includes(item.key)} onchange={(e) => flip(item.key, e.currentTarget.checked)} />
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

  .head {
    display: flex;
    align-items: baseline;
    gap: var(--s2);
  }

  .head .check {
    flex: 1;
    min-width: 0;
  }

  .check {
    display: flex;
    align-items: baseline;
    gap: var(--s2);
    cursor: pointer;
  }

  .fold {
    display: grid;
    align-self: center;
    padding: 2px;
    border: 0;
    border-radius: var(--radius);
    background: none;
    color: var(--ink-3);
    cursor: pointer;
    transition: transform 0.15s;
  }

  .fold:hover {
    color: var(--ink);
  }

  .fold[aria-expanded='true'] {
    transform: rotate(90deg);
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
