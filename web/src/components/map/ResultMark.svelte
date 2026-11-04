<script lang="ts">
  import type { ResultState } from '../../lib/map/results';

  let { state }: { state: ResultState } = $props();
</script>

<span class="mark {state}" aria-hidden="true"></span>

<style>
  .mark {
    flex: none;
    display: inline-block;
    width: 12px;
    height: 12px;
    border: 1px solid transparent;
    border-radius: 50%;
  }

  .passed {
    background: var(--good);
  }

  /* Same shapes as the points on the Atlas: half filled, hollow, faint, so the state reads without color. */
  .partial {
    border-color: var(--warning);
    background: linear-gradient(to top, var(--warning) 50%, transparent 50%);
  }

  .failed {
    border: 2px solid var(--critical);
  }

  .none {
    border-color: var(--axis);
  }
</style>
