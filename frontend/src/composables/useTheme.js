import { computed, ref, watchEffect } from 'vue'

const stored = typeof localStorage !== 'undefined' ? localStorage.getItem('theme') : null
const prefersDark = typeof matchMedia !== 'undefined' && matchMedia('(prefers-color-scheme: dark)').matches
const theme = ref(stored || (prefersDark ? 'dark' : 'light'))

export function useTheme() {
  const isDark = computed(() => theme.value === 'dark')
  function toggleTheme() {
    theme.value = isDark.value ? 'light' : 'dark'
  }
  watchEffect(() => {
    document.documentElement.dataset.theme = theme.value
    localStorage.setItem('theme', theme.value)
  })
  return { theme, isDark, toggleTheme }
}

