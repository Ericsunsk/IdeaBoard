<template>
  <section class="grid min-w-0 gap-6" :aria-labelledby="titleId">
    <header>
      <h2 :id="titleId" class="m-0 text-ib-modal-title leading-tight font-bold text-foreground">
        {{ title }}
      </h2>
    </header>

    <div class="grid min-w-0 gap-5">
      <slot />
    </div>
  </section>
</template>

<script setup lang="ts">
import { useId } from 'vue'

defineProps<{ title: string }>()

const titleId = useId()
</script>
