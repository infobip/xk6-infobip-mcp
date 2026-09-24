package infobip_mcp

import "go.k6.io/k6/js/modules"

const importPath = "k6/x/infobip_mcp"

const (
	clientName    = "xk6-infobip-mcp"
	clientVersion = "v1.1.0"
)

func init() {
	modules.Register(importPath, new(rootModule))
}
