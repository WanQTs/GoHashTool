<script setup lang="ts">
// 算法多选 chips：MD5 / SHA-1 / SHA-256 / SHA-512 / CRC32。
import { NTag } from 'naive-ui'
import { ALGO_LIST, ALGO_LABELS } from '../api'

const props = defineProps<{
  modelValue: string[]
  disabled?: boolean
}>()

const emit = defineEmits<{
  'update:modelValue': [value: string[]]
}>()

function toggle(algo: string) {
  const set = new Set(props.modelValue)
  if (set.has(algo)) {
    set.delete(algo)
  } else {
    set.add(algo)
  }
  // 保持固定顺序输出，避免勾选顺序影响列顺序
  emit('update:modelValue', ALGO_LIST.filter((a) => set.has(a)))
}
</script>

<template>
  <div class="algo-chips">
    <n-tag
      v-for="a in ALGO_LIST"
      :key="a"
      checkable
      size="small"
      :bordered="false"
      :disabled="disabled"
      :checked="modelValue.includes(a)"
      class="algo-chip"
      @update:checked="toggle(a)"
    >
      {{ ALGO_LABELS[a] }}
    </n-tag>
  </div>
</template>

<style scoped>
.algo-chips {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}

.algo-chip {
  cursor: pointer;
  font-size: 12px;
  transition:
    background-color 0.18s ease,
    color 0.18s ease;
}
</style>
