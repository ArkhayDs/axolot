<script lang="ts">
  import Icon from './Icon.svelte';
  import type { Fit } from '../lib/api';
  import { fitLabel } from '../lib/format';

  let { fit }: { fit: Fit } = $props();
</script>

<span class="state" class:state-good={fit === 'ok'} class:state-warning={fit === 'tight'} class:state-critical={fit === 'too_big'} class:muted={fit === 'unknown'}>
  <Icon name={fit === 'ok' ? 'check' : fit === 'too_big' ? 'cross' : 'alert'} size={16} />
  {fitLabel(fit)}
</span>
