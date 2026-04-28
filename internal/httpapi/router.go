package httpapi

import (
	"errors"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"project-helper/internal/agent"
	"project-helper/internal/ai"
	"project-helper/internal/analyzer"
	"project-helper/internal/config"
	"project-helper/internal/repo"
	"project-helper/internal/store"
)

type API struct {
	cfg      config.Config
	store    *store.Store
	analyzer *analyzer.Analyzer
	agent    *agent.Agent
}

func NewRouter(cfg config.Config, st *store.Store, an *analyzer.Analyzer, aiClient ai.Client, frontendFS fs.FS) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{cfg.FrontendURL, "http://localhost:5173", "http://127.0.0.1:5173"},
		AllowMethods:     []string{http.MethodGet, http.MethodPost, http.MethodOptions},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: false,
		MaxAge:           12 * time.Hour,
	}))

	api := &API{cfg: cfg, store: st, analyzer: an, agent: agent.New(st, aiClient)}
	r.GET("/healthz", api.health)

	group := r.Group("/api")
	{
		group.GET("/projects", api.listProjects)
		group.POST("/projects", api.createProject)
		group.GET("/projects/:id", api.getProject)
		group.GET("/projects/:id/events", api.projectEvents)
		group.GET("/projects/:id/report", api.getReport)
		group.POST("/projects/:id/regenerate", api.regenerateProject)
		group.POST("/projects/:id/chat/stream", api.chatStream)
	}
	mountFrontend(r, frontendFS)
	return r
}

func mountFrontend(r *gin.Engine, frontendFS fs.FS) {
	if frontendFS == nil {
		return
	}
	r.NoRoute(func(c *gin.Context) {
		requestPath := c.Request.URL.Path
		if requestPath == "/api" || strings.HasPrefix(requestPath, "/api/") {
			errorJSON(c, http.StatusNotFound, errors.New("接口不存在"))
			return
		}
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.Status(http.StatusNotFound)
			return
		}

		filePath := strings.TrimPrefix(path.Clean("/"+requestPath), "/")
		if filePath == "" || filePath == "." {
			filePath = "index.html"
		}
		if serveFrontendFile(c, frontendFS, filePath) {
			return
		}
		if serveFrontendFile(c, frontendFS, "index.html") {
			return
		}
		c.Status(http.StatusNotFound)
	})
}

func serveFrontendFile(c *gin.Context, frontendFS fs.FS, name string) bool {
	content, err := fs.ReadFile(frontendFS, name)
	if err != nil {
		return false
	}
	contentType := mime.TypeByExtension(path.Ext(name))
	if contentType == "" {
		contentType = http.DetectContentType(content)
	}
	c.Header("Content-Type", contentType)
	c.Header("Content-Length", strconv.Itoa(len(content)))
	if c.Request.Method == http.MethodHead {
		c.Status(http.StatusOK)
		return true
	}
	c.Data(http.StatusOK, contentType, content)
	return true
}

func (a *API) health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (a *API) listProjects(c *gin.Context) {
	projects, err := a.store.ListProjects(c.Request.Context())
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"projects": projects})
}

func (a *API) createProject(c *gin.Context) {
	var req struct {
		RepoURL string `json:"repo_url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		errorJSON(c, http.StatusBadRequest, errors.New("请求体需要 repo_url"))
		return
	}
	parsed, err := repo.ParseGitHubURL(req.RepoURL)
	if err != nil {
		errorJSON(c, http.StatusBadRequest, err)
		return
	}
	project, _, err := a.store.UpsertProject(c.Request.Context(), store.ProjectInput{
		RepoURL:       parsed.Original,
		NormalizedURL: parsed.Normalized,
		Owner:         parsed.Owner,
		Name:          parsed.Name,
	})
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, err)
		return
	}
	if has, _ := a.store.HasReport(c.Request.Context(), project.ID, project.CommitSHA); has {
		project.HasReport = true
		run, _ := a.store.LatestRun(c.Request.Context(), project.ID)
		project.CurrentRun = run
		c.JSON(http.StatusOK, gin.H{"project": project, "run": run, "cached": true})
		return
	}
	run, _, err := a.analyzer.Start(c.Request.Context(), project, parsed)
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, err)
		return
	}
	project.CurrentRun = run
	c.JSON(http.StatusAccepted, gin.H{"project": project, "run": run})
}

func (a *API) regenerateProject(c *gin.Context) {
	project, err := a.projectFromParam(c)
	if err != nil {
		errorJSON(c, http.StatusNotFound, err)
		return
	}
	parsed, err := repo.ParseGitHubURL(project.NormalizedURL)
	if err != nil {
		errorJSON(c, http.StatusBadRequest, err)
		return
	}
	run, _, err := a.analyzer.Regenerate(c.Request.Context(), project, parsed)
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, err)
		return
	}
	project.CurrentRun = run
	project.HasReport = false
	c.JSON(http.StatusAccepted, gin.H{"project": project, "run": run, "regenerating": true})
}

func (a *API) getProject(c *gin.Context) {
	project, err := a.projectFromParam(c)
	if err != nil {
		errorJSON(c, http.StatusNotFound, err)
		return
	}
	has, _ := a.store.HasReport(c.Request.Context(), project.ID, project.CommitSHA)
	project.HasReport = has
	run, _ := a.store.LatestRun(c.Request.Context(), project.ID)
	project.CurrentRun = run
	c.JSON(http.StatusOK, gin.H{"project": project})
}

func (a *API) projectEvents(c *gin.Context) {
	project, err := a.projectFromParam(c)
	if err != nil {
		errorJSON(c, http.StatusNotFound, err)
		return
	}
	setSSEHeaders(c)
	ch, unsubscribe := a.analyzer.Subscribe(project.ID)
	defer unsubscribe()

	if run, err := a.store.LatestRun(c.Request.Context(), project.ID); err == nil {
		writeSSE(c, "snapshot", run)
	}
	c.Stream(func(w io.Writer) bool {
		select {
		case <-c.Request.Context().Done():
			return false
		case event, ok := <-ch:
			if !ok {
				return false
			}
			writeSSE(c, event.Type, event)
			return event.Type != "done" && event.Type != "error"
		case <-time.After(20 * time.Second):
			writeSSE(c, "ping", gin.H{"ts": time.Now().Unix()})
			return true
		}
	})
}

func (a *API) getReport(c *gin.Context) {
	project, err := a.projectFromParam(c)
	if err != nil {
		errorJSON(c, http.StatusNotFound, err)
		return
	}
	if project.CommitSHA == "" {
		errorJSON(c, http.StatusNotFound, errors.New("项目尚未完成分析"))
		return
	}
	report, err := a.store.GetReport(c.Request.Context(), project.ID, project.CommitSHA)
	if err != nil {
		errorJSON(c, http.StatusNotFound, errors.New("报告不存在或仍在生成中"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"markdown": report})
}

func (a *API) chatStream(c *gin.Context) {
	project, err := a.projectFromParam(c)
	if err != nil {
		errorJSON(c, http.StatusNotFound, err)
		return
	}
	var req struct {
		Question string `json:"question"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		errorJSON(c, http.StatusBadRequest, errors.New("请求体需要 question"))
		return
	}
	setSSEHeaders(c)
	events, errs := a.agent.Answer(c.Request.Context(), project, req.Question)
	c.Stream(func(w io.Writer) bool {
		select {
		case event, ok := <-events:
			if !ok {
				return false
			}
			writeSSE(c, event.Type, event)
			return event.Type != "done"
		case err, ok := <-errs:
			if ok && err != nil {
				writeSSE(c, "error", gin.H{"error": err.Error()})
			}
			return false
		case <-c.Request.Context().Done():
			return false
		}
	})
}

func (a *API) projectFromParam(c *gin.Context) (*store.Project, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errors.New("项目 ID 不正确")
	}
	return a.store.GetProject(c.Request.Context(), id)
}

func errorJSON(c *gin.Context, status int, err error) {
	c.JSON(status, gin.H{"error": err.Error()})
}

func setSSEHeaders(c *gin.Context) {
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
}

func writeSSE(c *gin.Context, event string, payload any) {
	c.SSEvent(event, payload)
	c.Writer.Flush()
}
