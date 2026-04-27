<script setup>
import { computed, ref } from 'vue'
import { List, ChevronRight, ChevronDown } from 'lucide-vue-next'

const props = defineProps({
  headings: { type: Array, default: () => [] },
  containerSelector: { type: String, default: '.report-pane' }
})

const collapsed = ref(false)

const visibleHeadings = computed(() => props.headings.filter(h => h.level >= 1 && h.level <= 3))

const minLevel = computed(() => {
  if (!visibleHeadings.value.length) return 1
  return Math.min(...visibleHeadings.value.map(h => h.level))
})

function indent(level) {
  return (level - minLevel.value) * 16
}

function scrollTo(id) {
  const container = document.querySelector(props.containerSelector)
  const target = container?.querySelector(`#${CSS.escape(id)}`)
  if (target) {
    target.scrollIntoView({ behavior: 'smooth', block: 'start' })
    history.replaceState(null, '', `#${id}`)
  }
}

function toggleCollapse() {
  collapsed.value = !collapsed.value
}
</script>

<template>
  <nav v-if="visibleHeadings.length" class="toc" :class="{ collapsed }" aria-label="目录">
    <button class="toc-toggle" type="button" @click="toggleCollapse">
      <List :size="16" />
      <span>目录</span>
      <ChevronDown v-if="!collapsed" :size="14" />
      <ChevronRight v-else :size="14" />
    </button>
    <ul v-show="!collapsed" class="toc-list">
      <li
        v-for="heading in visibleHeadings"
        :key="heading.id"
        class="toc-item"
        :style="{ paddingLeft: `${12 + indent(heading.level)}px` }"
      >
        <button class="toc-link" type="button" @click="scrollTo(heading.id)">
          {{ heading.text }}
        </button>
      </li>
    </ul>
  </nav>
</template>
