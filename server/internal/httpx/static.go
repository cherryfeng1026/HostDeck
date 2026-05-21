package httpx

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func WithStaticFallback(apiHandler http.Handler, staticDir string) http.Handler {
	if apiHandler == nil {
		apiHandler = http.NotFoundHandler()
	}

	staticDir = strings.TrimSpace(staticDir)
	if staticDir == "" {
		return apiHandler
	}

	indexPath := filepath.Join(staticDir, "index.html")
	fileServer := http.FileServer(http.Dir(staticDir))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isAPIRequest(r.URL.Path) {
			apiHandler.ServeHTTP(w, r)
			return
		}

		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			apiHandler.ServeHTTP(w, r)
			return
		}

		if assetPath, ok := resolveStaticAssetPath(staticDir, r.URL.Path); ok && fileExists(assetPath) {
			fileServer.ServeHTTP(w, r)
			return
		}

		if fileExists(indexPath) {
			http.ServeFile(w, r, indexPath)
			return
		}

		apiHandler.ServeHTTP(w, r)
	})
}

func isAPIRequest(requestPath string) bool {
	return requestPath == "/api" || strings.HasPrefix(requestPath, "/api/")
}

func resolveStaticAssetPath(staticDir string, requestPath string) (string, bool) {
	cleaned := path.Clean("/" + strings.TrimSpace(requestPath))
	relative := strings.TrimPrefix(cleaned, "/")
	if relative == "" || relative == "." {
		return "", false
	}
	if relative == ".." || strings.HasPrefix(relative, "../") {
		return "", false
	}

	resolved := filepath.Join(staticDir, filepath.FromSlash(relative))
	rel, err := filepath.Rel(staticDir, resolved)
	if err != nil {
		return "", false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return resolved, true
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}
