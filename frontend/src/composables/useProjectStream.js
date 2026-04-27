import { ref } from 'vue'

export function useProjectStream() {
  const events = ref([])
  const connected = ref(false)
  let source = null

  function connect(projectId, onDone) {
    disconnect()
    events.value = []
    source = new EventSource(`/api/projects/${projectId}/events`)
    connected.value = true
    const names = ['snapshot', 'queued', 'cloning', 'indexing', 'summarizing', 'reporting', 'done', 'error', 'ping']
    for (const name of names) {
      source.addEventListener(name, (evt) => {
        const data = JSON.parse(evt.data)
        if (name !== 'ping') events.value.push({ event: name, ...data })
        if (name === 'done' || name === 'error') {
          disconnect()
          onDone?.(name, data)
        }
      })
    }
    source.onerror = () => {
      connected.value = false
    }
  }

  function disconnect() {
    if (source) source.close()
    source = null
    connected.value = false
  }

  return { events, connected, connect, disconnect }
}

