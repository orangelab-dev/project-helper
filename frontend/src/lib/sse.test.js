import { describe, expect, it } from 'vitest'
import { parseSSEBlock } from './sse'

describe('parseSSEBlock', () => {
  it('parses event and json data', () => {
    const parsed = parseSSEBlock('event: token\ndata: {"data":"hello"}')
    expect(parsed.event).toBe('token')
    expect(parsed.data.data).toBe('hello')
  })
})

