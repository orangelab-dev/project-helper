<script setup>
import { computed, onMounted, ref } from 'vue'
import { BookOpen, Bot, CheckCircle2, CircleStop, Code2, Download, Menu, Moon, PanelLeftClose, PanelRightClose, Play, RefreshCw, Search, Send, Sun } from 'lucide-vue-next'
import AppToast from './components/AppToast.vue'
import TableOfContents from './components/TableOfContents.vue'
import { createProject, getProject, getProjects, getReport, regenerateProject } from './lib/api'
import { renderMarkdown, extractHeadings } from './composables/useMarkdown'
import { useTheme } from './composables/useTheme'
import { useProjectStream } from './composables/useProjectStream'
import { useChatStream } from './composables/useChatStream'
import { useToast } from './composables/useToast'

const repoUrl = ref('https://github.com/gin-gonic/gin')
const projects = ref([])
const selectedProject = ref(null)
const report = ref('')
const loadingProject = ref(false)
const regeneratingProject = ref(false)
const projectError = ref('')
const question = ref('')
const sidebarCollapsed = ref(false)
const assistantCollapsed = ref(false)
const { isDark, toggleTheme } = useTheme()
const { showToast } = useToast()
const projectStream = useProjectStream()
const chat = useChatStream()

const renderedReport = computed(() => renderMarkdown(report.value || placeholderReport.value))
const tocHeadings = computed(() => extractHeadings(report.value || placeholderReport.value))
const activeRun = computed(() => {
  const last = projectStream.events.value.at(-1)
  return last || selectedProject.value?.current_run || null
})
const stepLabels = {
  snapshot: '同步最新状态',
  queued: '读取仓库信息',
  cloning: '克隆源码',
  indexing: '扫描源码文件',
  summarizing: '整理项目结构',
  reporting: '生成阅读地图',
  done: '分析完成',
  error: '分析失败'
}
const statusLabels = {
  queued: '等待分析',
  running: '分析中',
  completed: '分析完成',
  failed: '分析失败'
}
const currentStep = computed(() => activeRun.value?.step || activeRun.value?.event || selectedProject.value?.status || '')
const currentStepTitle = computed(() => stepLabels[currentStep.value] || statusLabels[currentStep.value] || '等待开始')
const statusText = computed(() => activeRun.value?.message || statusLabels[selectedProject.value?.status] || '选择仓库后开始分析')
const isAnalyzing = computed(() => projectStream.connected.value || selectedProject.value?.status === 'running' || selectedProject.value?.status === 'queued')
const placeholderReport = computed(() => selectedProject.value
  ? '# 报告生成中\n\n分析完成后，这里会出现完整源码阅读报告。'
  : '# 选择或输入一个仓库\n\n输入公开 GitHub 仓库地址后，project-helper 会克隆源码、建立索引并生成中文报告。')

onMounted(loadProjects)

async function loadProjects(notify = false) {
  try {
    const data = await getProjects()
    projects.value = data.projects || []
    if (!selectedProject.value && projects.value.length) await selectProject(projects.value[0], false)
    if (notify) showToast('项目列表已刷新')
  } catch (err) {
    if (notify) showToast(err.message || '项目列表刷新失败', 'error')
    else projects.value = []
  }
}

async function startAnalysis() {
  projectError.value = ''
  loadingProject.value = true
  try {
    const data = await createProject(repoUrl.value)
    selectedProject.value = data.project
    if (data.cached) {
      await loadReport()
      showToast('命中缓存，已加载历史报告')
    } else {
      report.value = ''
      watchAnalysis(data.project.id)
      showToast('分析任务已提交')
    }
    await loadProjects()
  } catch (err) {
    projectError.value = err.message
    showToast(err.message || '分析任务提交失败', 'error')
  } finally {
    loadingProject.value = false
  }
}

async function regenerateAnalysis() {
  if (!selectedProject.value) return
  projectError.value = ''
  regeneratingProject.value = true
  try {
    const data = await regenerateProject(selectedProject.value.id)
    selectedProject.value = data.project
    report.value = ''
    watchAnalysis(data.project.id)
    await loadProjects()
    showToast('已提交重新生成任务')
  } catch (err) {
    projectError.value = err.message
    showToast(err.message || '重新生成提交失败', 'error')
  } finally {
    regeneratingProject.value = false
  }
}

function watchAnalysis(projectId) {
  projectStream.connect(projectId, async (eventName) => {
    await refreshSelected()
    if (eventName === 'done') {
      await loadReport()
      showToast('阅读地图已生成')
    }
    if (eventName === 'error') showToast('分析失败，请查看当前步骤提示', 'error')
    await loadProjects()
  })
}

async function selectProject(project, notify = true) {
  selectedProject.value = project
  report.value = ''
  projectStream.disconnect()
  await refreshSelected()
  if (selectedProject.value?.has_report) await loadReport()
  if (notify) showToast(`已切换到 ${project.owner}/${project.name}`)
}

async function refreshSelected() {
  if (!selectedProject.value) return
  const data = await getProject(selectedProject.value.id)
  selectedProject.value = data.project
}

async function loadReport(notify = false) {
  if (!selectedProject.value) return
  try {
    const data = await getReport(selectedProject.value.id)
    report.value = data.markdown || ''
    if (notify) showToast(report.value ? '报告已刷新' : '报告暂不可用', report.value ? 'success' : 'error')
  } catch (err) {
    if (notify) showToast(err.message || '报告刷新失败', 'error')
    else report.value = ''
  }
}

async function downloadReport() {
  if (!selectedProject.value) return
  try {
    let markdown = report.value
    if (!markdown) {
      const data = await getReport(selectedProject.value.id)
      markdown = data.markdown || ''
      report.value = markdown
    }
    if (!markdown) {
      showToast('报告暂不可下载', 'error')
      return
    }

    const blob = new Blob([markdown], { type: 'text/markdown;charset=utf-8' })
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = reportFilename(selectedProject.value)
    document.body.appendChild(link)
    link.click()
    link.remove()
    URL.revokeObjectURL(url)
    showToast('报告下载已开始')
  } catch (err) {
    showToast(err.message || '报告下载失败', 'error')
  }
}

function reportFilename(project) {
  const sha = project.commit_sha ? `_${project.commit_sha.slice(0, 12)}` : ''
  return `${safeFilePart(project.owner)}_${safeFilePart(project.name)}${sha}_report.md`
}

function safeFilePart(value) {
  return String(value || 'project').replace(/[^a-zA-Z0-9_.-]+/g, '_')
}

function toggleAppTheme() {
  toggleTheme()
  showToast(isDark.value ? '已切换到黑夜模式' : '已切换到白天模式')
}

function collapseSidebar() {
  sidebarCollapsed.value = true
  showToast('左侧栏已隐藏')
}

function expandSidebar() {
  sidebarCollapsed.value = false
  showToast('左侧栏已显示')
}

function collapseAssistant() {
  assistantCollapsed.value = true
  showToast('右侧问答面板已隐藏')
}

function expandAssistant() {
  assistantCollapsed.value = false
  showToast('右侧问答面板已显示')
}

function stopAnswer() {
  chat.stop()
  showToast('已停止生成', 'info')
}

async function askQuestion() {
  if (!selectedProject.value || !question.value.trim()) return
  showToast('问题已发送')
  await chat.ask(selectedProject.value.id, question.value.trim())
  if (chat.error.value) showToast(chat.error.value, 'error')
  else showToast('回答已生成')
}
</script>

<template>
  <div class="app-shell" :class="{ 'sidebar-collapsed': sidebarCollapsed, 'assistant-collapsed': assistantCollapsed }">
    <aside class="sidebar" aria-label="项目导航">
      <header class="brand-row">
        <div class="brand-mark" aria-hidden="true"><Code2 :size="22" /></div>
        <div>
          <p class="eyebrow">project-helper</p>
          <h1>项目学习助手</h1>
        </div>
        <div class="brand-actions">
          <button class="icon-button" :aria-label="isDark ? '切换到白天模式' : '切换到黑夜模式'" @click="toggleAppTheme">
            <Sun v-if="isDark" :size="18" />
            <Moon v-else :size="18" />
          </button>
          <button class="icon-button" aria-label="隐藏左侧栏" @click="collapseSidebar">
            <PanelLeftClose :size="18" />
          </button>
        </div>
      </header>

      <form class="repo-form" @submit.prevent="startAnalysis">
        <label for="repo-url">GitHub 仓库</label>
        <div class="input-row">
          <Search :size="18" aria-hidden="true" />
          <input id="repo-url" v-model="repoUrl" autocomplete="url" placeholder="https://github.com/owner/repo" />
        </div>
        <button class="primary-button" type="submit" :disabled="loadingProject" :aria-busy="loadingProject">
          <Play :size="18" />
          <span>{{ loadingProject ? '提交中' : '开始分析' }}</span>
        </button>
        <p v-if="projectError" class="error-text">{{ projectError }}</p>
      </form>

      <section class="project-list" aria-labelledby="history-title">
        <div class="section-title">
          <h2 id="history-title">历史项目</h2>
          <button class="ghost-button" type="button" aria-label="刷新项目列表" @click="loadProjects(true)">
            <RefreshCw :size="16" />
          </button>
        </div>
        <button
          v-for="project in projects"
          :key="project.id"
          class="project-item"
          :class="{ active: selectedProject?.id === project.id }"
          type="button"
          @click="selectProject(project)"
        >
          <span class="repo-name">{{ project.owner }}/{{ project.name }}</span>
          <span class="repo-status">{{ project.has_report ? '已缓存' : project.status }}</span>
        </button>
      </section>
    </aside>

    <main class="workspace">
      <section class="topbar" aria-label="分析状态">
        <div class="topbar-title">
          <button
            v-if="sidebarCollapsed"
            class="icon-button"
            type="button"
            aria-label="显示左侧栏"
            @click="expandSidebar"
          >
            <Menu :size="18" />
          </button>
          <div>
            <p class="eyebrow">分析进度</p>
            <h2>{{ selectedProject ? `${selectedProject.owner}/${selectedProject.name}` : '等待仓库' }}</h2>
          </div>
        </div>
        <div class="progress-wrap" aria-live="polite">
          <p class="step-kicker">当前步骤</p>
          <strong>{{ currentStepTitle }}</strong>
          <span>{{ statusText }}</span>
        </div>
        <button
          v-if="assistantCollapsed"
          class="icon-button"
          type="button"
          aria-label="显示问答面板"
          @click="expandAssistant"
        >
          <Bot :size="18" />
        </button>
      </section>

      <section class="content-grid">
        <article class="report-pane" aria-labelledby="report-title">
          <div class="pane-header">
            <div>
              <p class="eyebrow">源码报告</p>
              <h2 id="report-title"><BookOpen :size="20" /> 阅读地图</h2>
            </div>
            <div class="button-group">
              <button class="ghost-button text-button" type="button" :disabled="!selectedProject" @click="loadReport(true)">刷新报告</button>
              <button
                class="ghost-button text-button"
                type="button"
                :disabled="!selectedProject || !selectedProject.has_report"
                @click="downloadReport"
              >
                <Download :size="16" />
                <span>下载报告</span>
              </button>
              <button
                class="ghost-button text-button"
                type="button"
                :disabled="!selectedProject || isAnalyzing || regeneratingProject"
                :aria-busy="regeneratingProject"
                @click="regenerateAnalysis"
              >
                <RefreshCw :size="16" />
                <span>{{ regeneratingProject ? '提交中' : '重新生成' }}</span>
              </button>
            </div>
          </div>
          <TableOfContents :headings="tocHeadings" />
          <div class="markdown-body" v-html="renderedReport"></div>
        </article>

        <aside class="assistant-pane" aria-labelledby="chat-title">
          <div class="pane-header compact">
            <div>
              <p class="eyebrow">Agent</p>
              <h2 id="chat-title"><Bot :size="20" /> 源码问答</h2>
            </div>
            <div class="pane-header-actions">
              <button v-if="chat.loading.value" class="icon-button danger" type="button" aria-label="停止生成" @click="stopAnswer">
                <CircleStop :size="18" />
              </button>
              <button class="icon-button" type="button" aria-label="隐藏问答面板" @click="collapseAssistant">
                <PanelRightClose :size="18" />
              </button>
            </div>
          </div>

          <div class="tool-timeline" aria-label="工具调用记录">
            <div v-for="(item, index) in chat.toolEvents.value" :key="index" class="tool-event">
              <CheckCircle2 :size="15" />
              <span>{{ item.name || item.event }}</span>
              <code>{{ item.data }}</code>
            </div>
          </div>

          <div class="answer-box" aria-live="polite">
            <div v-if="chat.answer.value" class="markdown-body small" v-html="renderMarkdown(chat.answer.value)"></div>
            <p v-else class="empty-state">分析完成后，可以问“这个项目从哪里开始读？”或“请求是怎么流转的？”</p>
            <p v-if="chat.error.value" class="error-text">{{ chat.error.value }}</p>
          </div>

          <form class="chat-form" @submit.prevent="askQuestion">
            <textarea v-model="question" rows="4" placeholder="针对源码提问..." :disabled="!selectedProject || chat.loading.value"></textarea>
            <button class="primary-button" type="submit" :disabled="!selectedProject || chat.loading.value || !question.trim()" :aria-busy="chat.loading.value">
              <Send :size="18" />
              <span>{{ chat.loading.value ? '生成中' : '提问' }}</span>
            </button>
          </form>
        </aside>
      </section>
    </main>
    <AppToast />
  </div>
</template>
