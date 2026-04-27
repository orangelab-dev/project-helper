import { readonly, ref } from 'vue'

const toast = ref(null)
let toastTimer = 0

export function useToast() {
  function showToast(message, type = 'success', duration = 2600) {
    window.clearTimeout(toastTimer)
    toast.value = { message, type }
    toastTimer = window.setTimeout(() => {
      toast.value = null
    }, duration)
  }

  function clearToast() {
    window.clearTimeout(toastTimer)
    toast.value = null
  }

  return {
    toast: readonly(toast),
    showToast,
    clearToast
  }
}
