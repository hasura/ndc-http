package configuration

import (
	"embed"
	"io"
	"os"
	"path/filepath"
	"text/template"
)

//go:embed all:templates/* templates
var templateFS embed.FS
var _templates *template.Template

const (
	templateEmptySettings = "server_empty.tmpl"
	templateEnvVariables  = "env_variables.tmpl"
	templateReadme        = "readme.tmpl"
)

const (
	ansiReset        = "\033[0m"
	ansiBrightRed    = "\033[91m"
	ansiBrightYellow = "\033[93m"
)

func getTemplates() (*template.Template, error) {
	if _templates != nil {
		return _templates, nil
	}

	var err error

	_templates, err = template.ParseFS(templateFS, "templates/*.tmpl")
	if err != nil {
		return nil, err
	}

	return _templates, nil
}

func writeColorTextIf(w io.Writer, text string, color string, noColor bool) {
	if noColor {
		_, _ = w.Write([]byte(text))

		return
	}

	_, _ = w.Write([]byte(color))
	_, _ = w.Write([]byte(text))
	_, _ = w.Write([]byte(ansiReset))
}

func writeErrorIf(w io.Writer, text string, noColor bool) {
	writeColorTextIf(w, "ERROR", ansiBrightRed, noColor)

	if text != "" {
		_, _ = w.Write([]byte(text))
	}
}

func writeWarningIf(w io.Writer, text string, noColor bool) {
	writeColorTextIf(w, "WARNING", ansiBrightYellow, noColor)

	if text != "" {
		_, _ = w.Write([]byte(text))
	}
}

// extract the relative context path for templates.
func tryRelPath(maybeAbsPath string, basePath string) string {
	if !filepath.IsAbs(maybeAbsPath) {
		return maybeAbsPath
	}

	if basePath == "" {
		currentDir, err := os.Getwd()
		if err != nil {
			return maybeAbsPath
		}

		basePath = currentDir
	}

	relativePath, err := filepath.Rel(basePath, maybeAbsPath)
	if err != nil {
		return maybeAbsPath
	}

	return relativePath
}
