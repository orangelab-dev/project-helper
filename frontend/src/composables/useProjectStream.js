import { ref } from 'vue'

export function useProjectStream() {
  const events = ref([])
  const connected = ref(false)
  let source = null

  function connect(projectId, onDone, onEvent) {
    disconnect()
    events.value = []
    source = new EventSource(`/api/projects/${projectId}/events`)
    connected.value = true
    const names = ['snapshot', 'queued', 'cloning', 'indexing', 'summarizing', 'reporting', 'report_token', 'done', 'error', 'ping']
    for (const name of names) {
      source.addEventListener(name, (evt) => {
        const data = JSON.parse(evt.data)
        if (name !== 'ping') events.value.push({ event: name, ...data })
        if (name !== 'ping') onEvent?.(name, data)
        const terminalEvent = terminalEventFor(name, data)
        if (terminalEvent) {
          disconnect()
          onDone?.(terminalEvent, data)
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

function terminalEventFor(name, data) {
  if (name === 'done' || name === 'error') return name
  if (name !== 'snapshot') return ''
  if (data?.status === 'completed' || data?.step === 'done') return 'done'
  if (data?.status === 'failed' || data?.step === 'error') return 'error'
  return ''
}
