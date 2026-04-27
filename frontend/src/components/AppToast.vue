<script setup>
import { useToast } from '../composables/useToast'

const { toast } = useToast()
</script>

<template>
  <Transition name="toast">
    <div v-if="toast" class="app-toast" :class="`app-toast-${toast.type}`" role="status" aria-live="polite">
      {{ toast.message }}
    </div>
  </Transition>
</template>

<style scoped>
.app-toast {
  position: fixed;
  right: 24px;
  bottom: 24px;
  z-index: 50;
  max-width: min(360px, calc(100vw - 32px));
  padding: 12px 14px;
  border: 1px solid var(--line);
  border-radius: 8px;
  color: var(--text);
  background: var(--panel-solid);
  box-shadow: var(--shadow);
  font-size: 14px;
  line-height: 1.45;
}

.app-toast-success {
  border-color: color-mix(in srgb, var(--success) 46%, var(--line));
}

.app-toast-error {
  border-color: color-mix(in srgb, var(--danger) 52%, var(--line));
}

.app-toast-info {
  border-color: color-mix(in srgb, var(--accent) 48%, var(--line));
}

.toast-enter-active,
.toast-leave-active {
  transition: opacity 180ms ease, transform 180ms ease;
}

.toast-enter-from,
.toast-leave-to {
  opacity: 0;
  transform: translateY(10px);
}

@media (max-width: 720px) {
  .app-toast {
    right: 16px;
    bottom: 16px;
  }
}
</style>
