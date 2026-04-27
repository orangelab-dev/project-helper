export async function getProjects() {
  const res = await fetch('/api/projects')
  return unwrap(res)
}

export async function createProject(repoUrl) {
  const res = await fetch('/api/projects', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ repo_url: repoUrl })
  })
  return unwrap(res)
}

export async function getProject(id) {
  const res = await fetch(`/api/projects/${id}`)
  return unwrap(res)
}

export async function regenerateProject(id) {
  const res = await fetch(`/api/projects/${id}/regenerate`, {
    method: 'POST'
  })
  return unwrap(res)
}

export async function getReport(id) {
  const res = await fetch(`/api/projects/${id}/report`)
  return unwrap(res)
}

async function unwrap(res) {
  const payload = await res.json().catch(() => ({}))
  if (!res.ok) {
    throw new Error(payload.error || `请求失败：${res.status}`)
  }
  return payload
}
