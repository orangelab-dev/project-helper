<script setup>
import { Github } from 'lucide-vue-next'
import avatarSvg from '../assets/avatar.svg'

defineProps({
  name: { type: String, default: '橘子' },
  githubUrl: { type: String, default: 'https://github.com/orangelab-dev' }
})
</script>

<template>
  <div class="author-credit">
    <img :src="avatarSvg" alt="" class="author-avatar" />
    <div class="author-info">
      <span class="author-by">by</span>
      <a :href="githubUrl" target="_blank" rel="noopener noreferrer" class="author-link">
        <span class="author-name">{{ name }}</span>
        <Github :size="14" />
      </a>
    </div>
  </div>
</template>
