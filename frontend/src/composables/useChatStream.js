import { ref } from 'vue'
import { readSSEStream } from '../lib/sse'

export function useChatStream() {
  const answer = ref('')
  const toolEvents = ref([])
  const loading = ref(false)
  const error = ref('')
  let controller = null

  async function ask(projectId, question) {
    stop()
    answer.value = ''
    toolEvents.value = []
    error.value = ''
    loading.value = true
    controller = new AbortController()
    try {
      const response = await fetch(`/api/projects/${projectId}/chat/stream`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ question }),
        signal: controller.signal
      })
      await readSSEStream(response, ({ event, data }) => {
        if (event === 'token') answer.value += data.data || ''
        if (event === 'tool_call' || event === 'tool_result') toolEvents.value.push({ event, ...data })
        if (event === 'error') error.value = data.error || '问答失败'
      }, controller.signal)
    } catch (err) {
      if (err.name !== 'AbortError') error.value = err.message
    } finally {
      loading.value = false
      controller = null
    }
  }

  function stop() {
    if (controller) controller.abort()
  }

  return { answer, toolEvents, loading, error, ask, stop }
}

