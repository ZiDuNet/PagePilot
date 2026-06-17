package api

import "net/http"

func (s *Server) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	base := s.deployer.PublicBaseURL()
	writeJSON(w, http.StatusOK, map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":       "hostctl API",
			"version":     s.version,
			"description": "Agent-friendly static site hosting API with deploys, versions, marketplace, and admin operations.",
		},
		"servers": []map[string]any{{"url": base}},
		"security": []map[string]any{
			{"bearerAuth": []string{}},
		},
		"paths": map[string]any{
			"/api/health": map[string]any{
				"get": map[string]any{
					"summary":  "Health check",
					"security": []any{},
					"responses": map[string]any{
						"200": map[string]any{"description": "Service is healthy"},
					},
				},
			},
			"/api/session": map[string]any{
				"get": map[string]any{
					"summary":     "Create or read an anonymous deploy session",
					"description": "Agents without a bearer token should call this once and send X-Hostctl-Session on anonymous write requests.",
					"security":    []any{},
					"responses": map[string]any{
						"200": map[string]any{"description": "Anonymous session", "content": jsonSchemaRef("AnonymousSessionResponse")},
					},
				},
			},
			"/api/session/claim": map[string]any{
				"post": map[string]any{
					"summary":     "Claim an anonymous session into the current user",
					"description": "Bearer token or login cookie required. Moves sites owned by anon:{sessionId} to user:{userId}.",
					"requestBody": jsonBodyRef("SessionClaimRequest"),
					"responses": map[string]any{
						"200": map[string]any{"description": "Session claimed", "content": jsonSchemaRef("SessionClaimResponse")},
						"401": errorResponse(),
						"403": errorResponse(),
					},
				},
			},
			"/api/config": map[string]any{
				"get": map[string]any{
					"summary":  "Read runtime configuration",
					"security": []any{},
					"responses": map[string]any{
						"200": map[string]any{"description": "Runtime configuration", "content": jsonSchemaRef("ConfigResponse")},
					},
				},
				"put": map[string]any{
					"summary":     "Update mutable runtime configuration",
					"description": "Admin token required in production.",
					"requestBody": jsonBodyRef("ConfigUpdateRequest"),
					"responses": map[string]any{
						"200": map[string]any{"description": "Configuration updated", "content": jsonSchemaRef("ConfigUpdateResponse")},
						"401": errorResponse(),
						"403": errorResponse(),
					},
				},
			},
			"/api/deploy": map[string]any{
				"post": map[string]any{
					"summary":     "Deploy a new static site or append a version",
					"description": "Use content for a single HTML file, or files[] for a multi-file static site. Without a bearer token, X-Hostctl-Session is allowed up to the anonymous deploy quota.",
					"requestBody": jsonBodyRef("DeployRequest"),
					"responses": map[string]any{
						"200": map[string]any{"description": "Deploy created", "content": jsonSchemaRef("DeployResponse")},
						"400": errorResponse(),
						"401": errorResponse(),
						"409": errorResponse(),
						"413": errorResponse(),
						"429": errorResponse(),
					},
				},
			},
			"/api/deploy/content": map[string]any{
				"get": map[string]any{
					"summary":  "Read deployed content metadata or download files",
					"security": []any{},
					"parameters": []map[string]any{
						queryParam("code", "string", true),
						queryParam("version", "integer", false),
						queryParam("download", "boolean", false),
					},
					"responses": map[string]any{
						"200": map[string]any{"description": "Content metadata or raw download", "content": jsonSchemaRef("GetContentResponse")},
						"404": errorResponse(),
					},
				},
				"patch": map[string]any{
					"summary":     "Append a version by code or URL",
					"requestBody": jsonBodyRef("ContentPatchRequest"),
					"responses": map[string]any{
						"200": map[string]any{"description": "Version created", "content": jsonSchemaRef("VersionCreatedResponse")},
						"401": errorResponse(),
					},
				},
			},
			"/api/deploys": map[string]any{
				"get": map[string]any{
					"summary":  "Search public marketplace",
					"security": []any{},
					"parameters": []map[string]any{
						queryParam("q", "string", false),
						queryParam("status", "string", false),
						queryParam("sort", "string", false),
						queryParam("page", "integer", false),
						queryParam("pageSize", "integer", false),
					},
					"responses": map[string]any{"200": map[string]any{"description": "Marketplace deploy list"}},
				},
			},
			"/api/deploys/{publicId}": map[string]any{
				"get": map[string]any{
					"summary":    "Read public deploy metadata by UUID or code",
					"security":   []any{},
					"parameters": []map[string]any{pathParam("publicId", "string")},
					"responses":  map[string]any{"200": map[string]any{"description": "Deploy metadata"}, "404": errorResponse()},
				},
			},
			"/api/deploys/{code}/like": map[string]any{
				"post": map[string]any{
					"summary":    "Like a deploy",
					"security":   []any{},
					"parameters": []map[string]any{pathParam("code", "string")},
					"responses":  map[string]any{"200": map[string]any{"description": "Like count updated"}},
				},
			},
			"/api/deploys/{code}/access": map[string]any{
				"post": map[string]any{
					"summary":     "Verify a site access password",
					"description": "Public endpoint. Anonymous visitors can submit the password. On success, the browser receives a signed 5-minute HttpOnly cookie for viewing this protected site.",
					"security":    []any{},
					"parameters":  []map[string]any{pathParam("code", "string")},
					"requestBody": jsonBodyRef("SiteAccessRequest"),
					"responses": map[string]any{
						"200": map[string]any{"description": "Access granted"},
						"401": errorResponse(),
					},
				},
				"patch": map[string]any{
					"summary":     "Set or clear a site access password",
					"description": "Site owner or admin required. Changing the password invalidates previously issued browser access cookies.",
					"parameters":  []map[string]any{pathParam("code", "string")},
					"requestBody": jsonBodyRef("SiteAccessRequest"),
					"responses": map[string]any{
						"200": map[string]any{"description": "Access password updated"},
						"401": errorResponse(),
						"403": errorResponse(),
					},
				},
			},
			"/api/deploys/{code}/versions": map[string]any{
				"get": map[string]any{
					"summary":    "List versions for a code",
					"parameters": []map[string]any{pathParam("code", "string")},
					"responses":  map[string]any{"200": map[string]any{"description": "Version list", "content": jsonSchemaRef("ListVersionsResponse")}},
				},
			},
			"/api/deploys/{code}/versions/{version}": map[string]any{
				"patch": map[string]any{
					"summary":     "Overwrite an unlocked version or change its status",
					"parameters":  []map[string]any{pathParam("code", "string"), pathParam("version", "integer")},
					"requestBody": jsonBodyRef("OverwriteRequest"),
					"responses":   map[string]any{"200": map[string]any{"description": "Version updated"}, "423": errorResponse()},
				},
				"delete": map[string]any{
					"summary":    "Delete an unlocked version",
					"parameters": []map[string]any{pathParam("code", "string"), pathParam("version", "integer")},
					"responses":  map[string]any{"200": map[string]any{"description": "Version deleted"}, "423": errorResponse()},
				},
			},
			"/api/deploys/{code}/versions/{version}/lock": map[string]any{
				"post": map[string]any{
					"summary":     "Lock or unlock a version",
					"parameters":  []map[string]any{pathParam("code", "string"), pathParam("version", "integer")},
					"requestBody": jsonBodyRef("LockRequest"),
					"responses":   map[string]any{"200": map[string]any{"description": "Lock status updated", "content": jsonSchemaRef("LockResponse")}},
				},
			},
			"/api/deploys/{code}/current": map[string]any{
				"patch": map[string]any{
					"summary":     "Switch the current public version",
					"parameters":  []map[string]any{pathParam("code", "string")},
					"requestBody": jsonBodyRef("SetCurrentRequest"),
					"responses":   map[string]any{"200": map[string]any{"description": "Current version switched"}},
				},
			},
			"/api/deploys/{code}/primary-strategy": map[string]any{
				"get": map[string]any{
					"summary":    "Read primary version strategy",
					"parameters": []map[string]any{pathParam("code", "string")},
					"responses":  map[string]any{"200": map[string]any{"description": "Primary strategy"}},
				},
				"patch": map[string]any{
					"summary":     "Set primary version strategy",
					"parameters":  []map[string]any{pathParam("code", "string")},
					"requestBody": jsonBodyRef("PrimaryStrategyRequest"),
					"responses":   map[string]any{"200": map[string]any{"description": "Primary strategy updated"}},
				},
			},
			"/api/admin/session": map[string]any{
				"get": map[string]any{
					"summary":     "Validate admin login session",
					"description": "Returns dev session without a token in development mode. Requires an admin token in production.",
					"responses":   map[string]any{"200": map[string]any{"description": "Admin session", "content": jsonSchemaRef("AdminSessionResponse")}, "403": errorResponse()},
				},
			},
			"/api/admin/sites": map[string]any{
				"get": map[string]any{
					"summary":     "List all sites",
					"description": "Admin token required in production.",
					"responses":   map[string]any{"200": map[string]any{"description": "Site list", "content": jsonSchemaRef("SiteListResponse")}},
				},
			},
			"/api/admin/anonymous-sessions": map[string]any{
				"get": map[string]any{
					"summary":     "List recent anonymous deploy sessions",
					"description": "Admin token required in production.",
					"responses":   map[string]any{"200": map[string]any{"description": "Anonymous session list", "content": jsonSchemaRef("AnonymousSessionListResponse")}},
				},
			},
			"/api/admin/sites/{code}": map[string]any{
				"delete": map[string]any{
					"summary":     "Delete a whole site and its files",
					"description": "Admin token required in production.",
					"parameters":  []map[string]any{pathParam("code", "string")},
					"responses":   map[string]any{"200": map[string]any{"description": "Site deleted"}, "403": errorResponse()},
				},
			},
			"/api/token": map[string]any{
				"post": map[string]any{
					"summary":     "Create a bearer token",
					"description": "Admin token required in production. Plaintext token is returned only once.",
					"requestBody": jsonBodyRef("TokenCreateRequest"),
					"responses":   map[string]any{"200": map[string]any{"description": "Token created", "content": jsonSchemaRef("TokenCreateResponse")}, "403": errorResponse()},
				},
			},
			"/api/tokens": map[string]any{
				"get": map[string]any{
					"summary":     "List tokens",
					"description": "Admin token required in production.",
					"responses":   map[string]any{"200": map[string]any{"description": "Token list", "content": jsonSchemaRef("TokenListResponse")}},
				},
			},
			"/api/tokens/{id}": map[string]any{
				"delete": map[string]any{
					"summary":     "Revoke a token",
					"description": "Admin token required in production.",
					"parameters":  []map[string]any{pathParam("id", "string")},
					"responses":   map[string]any{"200": map[string]any{"description": "Token revoked"}, "403": errorResponse()},
				},
			},
		},
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"bearerAuth": map[string]any{"type": "http", "scheme": "bearer"},
			},
			"schemas": openAPISchemas(),
		},
	})
}

func jsonBodyRef(name string) map[string]any {
	return map[string]any{
		"required": true,
		"content": map[string]any{
			"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/" + name}},
		},
	}
}

func jsonSchemaRef(name string) map[string]any {
	return map[string]any{
		"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/" + name}},
	}
}

func errorResponse() map[string]any {
	return map[string]any{"description": "Structured API error", "content": jsonSchemaRef("APIError")}
}

func pathParam(name, typ string) map[string]any {
	return map[string]any{
		"name": name, "in": "path", "required": true,
		"schema": map[string]any{"type": typ},
	}
}

func queryParam(name, typ string, required bool) map[string]any {
	return map[string]any{
		"name": name, "in": "query", "required": required,
		"schema": map[string]any{"type": typ},
	}
}

func openAPISchemas() map[string]any {
	str := map[string]any{"type": "string"}
	boolSchema := map[string]any{"type": "boolean"}
	intSchema := map[string]any{"type": "integer"}
	timeSchema := map[string]any{"type": "string", "format": "date-time"}

	return map[string]any{
		"APIError": map[string]any{"type": "object", "properties": map[string]any{
			"success": boolSchema, "errorCode": str, "stage": str, "detail": str, "hint": str,
			"retryAfterSeconds": intSchema, "requestId": str,
		}},
		"DeployFile": map[string]any{"type": "object", "required": []string{"path"}, "properties": map[string]any{
			"path": str, "content": str, "contentBase64": str,
		}},
		"DeployRequest": map[string]any{"type": "object", "required": []string{"filename", "description"}, "properties": map[string]any{
			"filename": str, "description": str, "title": str, "content": str,
			"files":            map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/DeployFile"}},
			"enableCustomCode": boolSchema, "customCode": str, "createVersion": boolSchema, "source": str,
			"accessPassword": map[string]any{
				"type":        "string",
				"description": "Optional visit password for the new site. Anonymous visitors can enter it later to receive a signed 5-minute browser access cookie.",
			},
		}},
		"DeployResponse": map[string]any{"type": "object", "properties": map[string]any{
			"success": boolSchema, "id": str, "code": str, "url": str, "detailUrl": str, "versionUrl": str,
			"qrCode": str, "description": str, "versionId": str, "versionNumber": intSchema,
			"currentVersionId": str, "preserveHint": str, "agentGuideUrl": str,
			"primaryVersionStrategy": str, "requestId": str, "cooldownSeconds": intSchema,
			"nextAvailableAt": timeSchema, "versionCount": intSchema, "size": intSchema, "createdAt": str,
		}},
		"VersionCreatedResponse": map[string]any{"type": "object", "properties": map[string]any{
			"success": boolSchema, "code": str, "versionId": str, "versionNumber": intSchema,
			"url": str, "detailUrl": str, "versionUrl": str, "currentVersionId": str,
			"preserveHint": str, "primaryVersionStrategy": str,
		}},
		"ListVersionsResponse": map[string]any{"type": "object", "properties": map[string]any{
			"success": boolSchema, "code": str, "currentVersion": intSchema,
			"versions": map[string]any{"type": "array", "items": map[string]any{"type": "object"}},
		}},
		"GetContentResponse": map[string]any{"type": "object", "properties": map[string]any{
			"success": boolSchema, "code": str, "version": intSchema, "title": str,
			"description": str, "mainEntry": str, "totalSize": intSchema, "isLocked": boolSchema,
			"files":     map[string]any{"type": "array", "items": map[string]any{"type": "object"}},
			"createdAt": timeSchema,
		}},
		"OverwriteRequest": map[string]any{"type": "object", "properties": map[string]any{
			"description": str, "title": str, "filename": str, "content": str, "status": str,
			"files": map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/DeployFile"}},
		}},
		"ContentPatchRequest": map[string]any{"type": "object", "properties": map[string]any{
			"code": str, "url": str, "content": str, "description": str, "title": str, "filename": str,
		}},
		"LockRequest":            map[string]any{"type": "object", "properties": map[string]any{"locked": boolSchema}},
		"LockResponse":           map[string]any{"type": "object", "properties": map[string]any{"success": boolSchema, "code": str, "versionNumber": intSchema, "isLocked": boolSchema}},
		"SetCurrentRequest":      map[string]any{"type": "object", "properties": map[string]any{"versionNumber": intSchema, "versionId": str}},
		"PrimaryStrategyRequest": map[string]any{"type": "object", "properties": map[string]any{"primaryVersionStrategy": map[string]any{"type": "string", "enum": []string{"likes", "latest"}}}},
		"ConfigResponse": map[string]any{"type": "object", "properties": map[string]any{
			"success": boolSchema, "publicBaseURL": str, "mode": str, "corsAllowOrigins": str, "cooldownSeconds": intSchema,
			"limits": map[string]any{"$ref": "#/components/schemas/Limits"}, "anonymousPolicy": map[string]any{"$ref": "#/components/schemas/AnonymousPolicy"}, "version": str,
		}},
		"Limits":          map[string]any{"type": "object", "properties": map[string]any{"maxSingleFileBytes": intSchema, "maxSiteTotalBytes": intSchema, "maxFilesPerSite": intSchema}},
		"AnonymousPolicy": map[string]any{"type": "object", "properties": map[string]any{"deployLimit": intSchema}},
		"AnonymousSessionResponse": map[string]any{"type": "object", "properties": map[string]any{
			"success": boolSchema, "sessionId": str, "agentId": str, "agentLabel": str, "deployCount": intSchema, "deployLimit": intSchema, "remaining": intSchema,
		}},
		"SessionClaimRequest": map[string]any{"type": "object", "properties": map[string]any{
			"sessionId": str,
		}},
		"SessionClaimResponse": map[string]any{"type": "object", "properties": map[string]any{
			"success": boolSchema, "sessionId": str, "userId": str, "siteCount": intSchema, "deployCount": intSchema, "alreadyClaimed": boolSchema,
		}},
		"SiteAccessRequest": map[string]any{"type": "object", "properties": map[string]any{
			"password": str,
		}},
		"AnonymousSessionListResponse": map[string]any{"type": "object", "properties": map[string]any{
			"success": boolSchema, "deployLimit": intSchema,
			"sessions": map[string]any{"type": "array", "items": map[string]any{"type": "object", "properties": map[string]any{
				"id": str, "agentId": str, "agentLabel": str, "deviceIp": str, "userAgent": str, "deployCount": intSchema, "remaining": intSchema, "claimedByUserId": str, "claimedAt": timeSchema, "createdAt": timeSchema, "lastUsedAt": timeSchema,
			}}},
		}},
		"ConfigUpdateRequest": map[string]any{"type": "object", "properties": map[string]any{
			"publicBaseURL": str, "anonymousDeployLimit": intSchema, "cooldownSeconds": intSchema,
			"maxSingleFileBytes": intSchema, "maxSiteTotalBytes": intSchema, "maxFilesPerSite": intSchema, "corsAllowOrigins": str,
		}},
		"ConfigUpdateResponse": map[string]any{"type": "object", "properties": map[string]any{
			"success": boolSchema, "publicBaseURL": str, "corsAllowOrigins": str, "cooldownSeconds": intSchema,
			"limits": map[string]any{"$ref": "#/components/schemas/Limits"}, "anonymousPolicy": map[string]any{"$ref": "#/components/schemas/AnonymousPolicy"},
		}},
		"AdminSessionResponse": map[string]any{"type": "object", "properties": map[string]any{"success": boolSchema, "mode": str, "tokenId": str, "label": str, "userId": str, "username": str, "isAdmin": boolSchema}},
		"SiteListResponse":     map[string]any{"type": "object", "properties": map[string]any{"success": boolSchema, "sites": map[string]any{"type": "array", "items": map[string]any{"type": "object"}}}},
		"TokenCreateRequest":   map[string]any{"type": "object", "properties": map[string]any{"label": str, "ownerUserId": str, "isAdmin": boolSchema, "expiresAt": timeSchema, "ttlSeconds": intSchema}},
		"TokenCreateResponse":  map[string]any{"type": "object", "properties": map[string]any{"success": boolSchema, "id": str, "token": str, "label": str, "ownerUserId": str, "isAdmin": boolSchema, "expiresAt": timeSchema, "createdAt": timeSchema}},
		"TokenListResponse":    map[string]any{"type": "object", "properties": map[string]any{"success": boolSchema, "tokens": map[string]any{"type": "array", "items": map[string]any{"type": "object"}}}},
	}
}
