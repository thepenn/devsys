// Package web embeds the built React frontend (web/dist) into the binary so
// that a single Go executable serves both the API and the static SPA bundle.
//
// Build prerequisite: 前端必须先 `make web` (或 `npm run build`) 把 SPA 输出
// 到 web/dist/. 本目录下提交了一个 .gitkeep, 让 `dist` 目录始终存在 —
// 这样开发时即使没构建前端, `go build ./...` 也能成功 (Lookup 会在请求时
// 返回 404, 但服务能起来).
package web

import (
	"embed"
	"io"
	"io/fs"
	"net/http"
)

// 用 `//go:embed all:dist` 替代 `dist/*` (glob): glob 要求至少匹配一个路径,
// 否则 `pattern dist/*: no matching files found`. `all:` 前缀让 embed 把以
// `.` 开头的文件 (例如 .gitkeep 占位符) 也算上, 因此前端没构建时目录里只有
// .gitkeep 也能编译通过.
//
//go:embed all:dist
var webFiles embed.FS

func HTTPFS() (http.FileSystem, error) {
	httpFS, err := fs.Sub(webFiles, "dist")
	if err != nil {
		return nil, err
	}
	return http.FS(httpFS), nil
}

func Lookup(path string) (buf []byte, err error) {
	httpFS, err := HTTPFS()
	if err != nil {
		return nil, err
	}
	file, err := httpFS.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	buf, err = io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	return buf, nil
}
