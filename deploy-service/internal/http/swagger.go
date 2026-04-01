package http

import (
	_ "embed"
	"fmt"
	"net/http"
)

//go:embed swagger/openapi.yaml
var openAPISpec []byte

func (h *Handler) SwaggerSpec(responseWriter http.ResponseWriter, _ *http.Request) {
	responseWriter.Header().Set("Content-Type", "application/yaml")
	responseWriter.WriteHeader(http.StatusOK)
	_, _ = responseWriter.Write(openAPISpec)
}

func (h *Handler) SwaggerUI(responseWriter http.ResponseWriter, request *http.Request) {
	html := fmt.Sprintf(`<!doctype html>
<html>
  <head>
    <meta charset="UTF-8" />
    <title>Deploy Service API</title>
    <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css" />
  </head>
  <body>
    <div id="swagger-ui"></div>
    <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
    <script>
      window.ui = SwaggerUIBundle({
        url: "%s://%s/swagger/openapi.yaml",
        dom_id: '#swagger-ui'
      });
    </script>
  </body>
</html>`, schemaFromRequest(request), request.Host)

	responseWriter.Header().Set("Content-Type", "text/html; charset=utf-8")
	responseWriter.WriteHeader(http.StatusOK)
	_, _ = responseWriter.Write([]byte(html))
}

func schemaFromRequest(request *http.Request) string {
	if request.TLS != nil {
		return "https"
	}
	return "http"
}
