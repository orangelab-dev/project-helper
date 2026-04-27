import MarkdownIt from 'markdown-it'
import hljs from 'highlight.js'

const md = new MarkdownIt({
  html: false,
  linkify: true,
  typographer: true,
  highlight(code, lang) {
    const language = lang && hljs.getLanguage(lang) ? lang : 'plaintext'
    const value = hljs.highlight(code, { language }).value
    return `<pre class="code-block"><code class="hljs language-${language}">${value}</code></pre>`
  }
})

const headingIds = new Map()

function slugify(text) {
  return text
    .replace(/<[^>]+>/g, '')
    .replace(/[^\w\u4e00-\u9fff]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .toLowerCase()
}

md.renderer.rules.heading_open = (tokens, idx) => {
  const token = tokens[idx]
  const level = token.markup.length
  const next = tokens[idx + 1]
  const text = next ? next.content || '' : ''
  const base = slugify(text) || `heading-${level}`
  const count = headingIds.get(base) || 0
  headingIds.set(base, count + 1)
  const id = count > 0 ? `${base}-${count}` : base
  return `<h${level} id="${id}">`
}

md.renderer.rules.heading_close = (tokens, idx) => {
  const level = tokens[idx].markup.length
  return `</h${level}>`
}

export function renderMarkdown(source) {
  headingIds.clear()
  return md.render(source || '')
}

export function extractHeadings(source) {
  headingIds.clear()
  const tokens = md.parse(source || '', {})
  const headings = []
  const idCount = new Map()

  for (let i = 0; i < tokens.length; i++) {
    if (tokens[i].type === 'heading_open') {
      const level = tokens[i].markup.length
      const next = tokens[i + 1]
      const text = next?.content || ''
      const base = slugify(text) || `heading-${level}`
      const count = idCount.get(base) || 0
      idCount.set(base, count + 1)
      const id = count > 0 ? `${base}-${count}` : base
      headings.push({ level, text, id })
    }
  }

  return headings
}

