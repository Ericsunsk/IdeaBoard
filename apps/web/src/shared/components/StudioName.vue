<template>
  <span class="inline-flex min-w-0 items-center gap-2 text-ib-heading font-bold tracking-normal">
    <img
      :src="studioLogo"
      alt=""
      aria-hidden="true"
      class="h-[18px] w-auto shrink-0 object-contain"
    />
    <span class="min-w-0 truncate">Eric Studio</span>
  </span>
</template>

<script setup lang="ts">
import studioLogo from '../assets/eric-studio-logo.svg'
</script>
