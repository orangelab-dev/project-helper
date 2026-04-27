package config

import (
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
)

func SetupLogging(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	writer := io.MultiWriter(os.Stdout, file)
	log.SetOutput(writer)
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	gin.DefaultWriter = writer
	gin.DefaultErrorWriter = writer
	return file, nil
}
