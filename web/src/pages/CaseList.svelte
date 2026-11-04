<script lang="ts">
  import { onMount } from 'svelte';

  import Icon from '../components/Icon.svelte';
  import ModelName from '../components/ModelName.svelte';
  import Notice from '../components/Notice.svelte';
  import { api, errorMessage } from '../lib/api';
  import type { Case, CasesResponse } from '../lib/api';
  import { categoryLabel } from '../lib/format';

  let catalog = $state<CasesResponse | null>(null);
  let error = $state('');

  const KINDS: Record<Case['kind'], string> = { text: 'Texte', decision: 'Décision', rag: 'Documents' };

  onMount(async () => {
    try {
      catalog = await api.cases();
    } catch (loadError) {
      error = errorMessage(loadError);
    }
  });
</script>

<div class="page">
  <div class="page-head">
    <div>
      <h1>Scénarios</h1>
      <p>Chaque question a un résultat attendu, vérifié automatiquement.</p>
    </div>
  </div>

  <Notice message={error} />

  {#if catalog}
    <div class="table-wrap">
      <table>
        <thead>
          <tr>
            <th scope="col">Scénario</th>
            <th scope="col" class="wide">Type</th>
            <th scope="col" class="wide">Thème</th>
            <th scope="col" class="num">Questions</th>
            <th scope="col" class="wide">Modèles choisis</th>
          </tr>
        </thead>
        <tbody>
          {#each catalog.cases as item (item.id)}
            <tr>
              <td><a href="#/cas/{encodeURIComponent(item.id)}" class="case-link">{item.title} <Icon name="arrow" size={16} /></a></td>
              <td class="wide">{KINDS[item.kind]}</td>
              <td class="wide">{categoryLabel(item.category)}</td>
              <td class="num">{item.items.length}</td>
              <td class="wide">
                <span class="chosen">
                  {#each catalog.selections.filter((selection) => selection.case_id === item.id) as selection (selection.provider_id + selection.model)}
                    <ModelName name={selection.model} />
                  {:else}
                    <span class="muted">Aucun</span>
                  {/each}
                </span>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>

<style>
  .case-link {
    display: inline-flex;
    align-items: center;
    gap: var(--s2);
    font-weight: 550;
    text-decoration: none;
  }

  .case-link:hover {
    color: var(--accent);
  }

  .chosen {
    display: flex;
    flex-wrap: wrap;
    gap: var(--s1) var(--s3);
  }

  @media (max-width: 760px) {
    .wide {
      display: none;
    }
  }
</style>
