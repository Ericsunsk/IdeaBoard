<template>
  <button
    v-for="port in visiblePorts"
    :key="port.key"
    type="button"
    class="node-port"
    :class="[
      port.direction === 'input' ? 'is-input' : 'is-output',
      { 'is-connected': !!port.connected },
    ]"
    :style="port.top != null ? { top: `${port.top}px` } : undefined"
    :data-port-kind="port.kind"
    :data-port-direction="port.direction"
    :data-port-name="port.name"
    :data-node-kind="nodeKind"
    :data-node-id="nodeId"
    @mousedown.stop="onMouseDown($event, port)"
  ></button>
</template>

<script setup lang="ts">
import { computed } from 'vue'

type PortKind = 'image' | 'mask' | 'prompt'
type PortDirection = 'input' | 'output'

type NodePortItem = {
  key: string
  direction: PortDirection
  kind: PortKind
  name: string
  top?: number
  connected?: boolean
  visible?: boolean
}

const props = defineProps<{
  nodeKind: string
  nodeId: number
  ports: NodePortItem[]
}>()

const emit = defineEmits<{
  (e: 'port-mousedown', event: MouseEvent, port: string): void
}>()

const visiblePorts = computed(() => props.ports.filter((port) => port.visible !== false))

function onMouseDown(event: MouseEvent, port: NodePortItem) {
  if (port.direction !== 'output') return
  event.preventDefault()
  emit('port-mousedown', event, port.name)
}
</script>

<style scoped src="./node-ports.css"></style>
