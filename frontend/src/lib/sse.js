export function parseSSEBlock(block) {
  const lines = block.split(/\r?\n/)
  let event = 'message'
  const data = []
  for (const line of lines) {
    if (line.startsWith('event:')) event = line.slice(6).trim()
    if (line.startsWith('data:')) data.push(line.slice(5).trimStart())
  }
  const raw = data.join('\n')
  return { event, data: parseData(raw) }
}

export function parseData(raw) {
  if (!raw) return null
  try {
    return JSON.parse(raw)
  } catch {
    return raw
  }
}

export async function readSSEStream(response, onEvent, signal) {
  if (!response.ok) {
    const body = await response.text()
    throw new Error(body || `请求失败：${response.status}`)
  }
  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  while (true) {
    if (signal?.aborted) break
    const { value, done } = await reader.read()
    if (done) break
    buffer += decoder.decode(value, { stream: true })
    const blocks = buffer.split(/\n\n/)
    buffer = blocks.pop() || ''
    for (const block of blocks) {
      if (block.trim()) onEvent(parseSSEBlock(block))
    }
  }
  if (buffer.trim()) onEvent(parseSSEBlock(buffer))
}

