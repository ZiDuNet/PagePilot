package api

import "net/http"

func (s *Server) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	base := s.requestBaseURL(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":       "PagePilot API",
			"version":     s.version,
			"description": "Agent-friendly application publishing API with deploys, versions, marketplace, screens, and admin operations.",
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
			"/api/auth/captcha": map[string]any{
				"get": map[string]any{
					"summary":  "Create a numeric image captcha",
					"security": []any{},
					"responses": map[string]any{
						"200": map[string]any{"description": "Captcha created", "content": jsonSchemaRef("CaptchaResponse")},
					},
				},
			},
			"/api/auth/email-code": map[string]any{
				"post": map[string]any{
					"summary":     "Send registration email verification code",
					"description": "Only available when registration and email verification are enabled. Requires a fresh captcha answer.",
					"security":    []any{},
					"requestBody": jsonBodyRef("EmailVerificationRequest"),
					"responses": map[string]any{
						"200": map[string]any{"description": "Verification code sent", "content": jsonSchemaRef("EmailVerificationResponse")},
						"400": errorResponse(),
						"403": errorResponse(),
					},
				},
			},
			"/api/auth/register": map[string]any{
				"post": map[string]any{
					"summary":     "Register a user account",
					"description": "Requires captcha. When email verification is enabled, email and emailCode are also required.",
					"security":    []any{},
					"requestBody": jsonBodyRef("RegisterRequest"),
					"responses": map[string]any{
						"200": map[string]any{"description": "User registered", "content": jsonSchemaRef("RegisterResponse")},
						"400": errorResponse(),
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
			"/api/screens": map[string]any{
				"get": map[string]any{
					"summary":     "List bound hardware screens",
					"description": "Registered user token or login cookie required. Admins see all screens; normal users see their own screens.",
					"responses": map[string]any{
						"200": map[string]any{"description": "Screen list", "content": jsonSchemaRef("ScreenListResponse")},
						"401": errorResponse(),
					},
				},
			},
			"/api/screens/bind": map[string]any{
				"post": map[string]any{
					"summary":     "Bind a hardware screen",
					"description": "Registered user token or login cookie required. Pairing codes are short-lived and one-time use.",
					"requestBody": jsonBodyRef("ScreenBindRequest"),
					"responses": map[string]any{
						"200": map[string]any{"description": "Screen bound", "content": jsonSchemaRef("ScreenBindResponse")},
						"401": errorResponse(),
						"404": errorResponse(),
					},
				},
			},
			"/api/screens/{screenId}/publish": map[string]any{
				"post": map[string]any{
					"summary":     "Publish an app to a hardware screen",
					"description": "Registered user token required. The target screen and app must both belong to the current user.",
					"parameters":  []map[string]any{pathParam("screenId", "string")},
					"requestBody": jsonBodyRef("ScreenPublishRequest"),
					"responses": map[string]any{
						"200": map[string]any{"description": "Screen manifest target updated", "content": jsonSchemaRef("ScreenPublishResponse")},
						"401": errorResponse(),
						"403": errorResponse(),
						"404": errorResponse(),
					},
				},
			},
			"/api/screens/{screenId}/screenshot": map[string]any{
				"post": map[string]any{
					"summary":     "Request a screen screenshot",
					"description": "Registered user token or login cookie required. The device uploads only after receiving this one-time command from its manifest.",
					"parameters":  []map[string]any{pathParam("screenId", "string")},
					"responses": map[string]any{
						"200": map[string]any{"description": "Screenshot request queued", "content": jsonSchemaRef("ScreenScreenshotResponse")},
						"401": errorResponse(),
						"403": errorResponse(),
						"404": errorResponse(),
					},
				},
				"get": map[string]any{
					"summary":     "Read the latest screen screenshot",
					"description": "Registered user token or login cookie required. Returns the last command-triggered image for the screen.",
					"parameters":  []map[string]any{pathParam("screenId", "string")},
					"responses": map[string]any{
						"200": map[string]any{"description": "Screenshot image"},
						"401": errorResponse(),
						"403": errorResponse(),
						"404": errorResponse(),
					},
				},
			},
			"/api/screens/{screenId}/command": map[string]any{
				"post": map[string]any{
					"summary":     "Send an operational command to a screen",
					"description": "Registered user token or login cookie required. Supported types are refresh, sleep, wake, and shutdown. Shutdown is a soft standby command unless the device has OEM power privileges.",
					"parameters":  []map[string]any{pathParam("screenId", "string")},
					"requestBody": jsonBodyRef("ScreenCommandRequest"),
					"responses": map[string]any{
						"200": map[string]any{"description": "Command queued", "content": jsonSchemaRef("ScreenCommandResponse")},
						"401": errorResponse(),
						"403": errorResponse(),
						"404": errorResponse(),
					},
				},
			},
			"/api/screens/{screenId}": map[string]any{
				"delete": map[string]any{
					"summary":     "Unbind a hardware screen",
					"description": "Registered user token or login cookie required. Removes the long-lived device token from the server side.",
					"parameters":  []map[string]any{pathParam("screenId", "string")},
					"responses": map[string]any{
						"200": map[string]any{"description": "Screen unbound", "content": jsonSchemaRef("ScreenDeleteResponse")},
						"401": errorResponse(),
						"403": errorResponse(),
						"404": errorResponse(),
					},
				},
			},
			"/api/device/pairing/start": map[string]any{
				"post": map[string]any{
					"summary":     "Start device pairing",
					"description": "Called by the screen app after the operator configures the PagePilot server address.",
					"security":    []any{},
					"requestBody": jsonBodyRef("DevicePairingStartRequest"),
					"responses":   map[string]any{"200": map[string]any{"description": "Pairing code created", "content": jsonSchemaRef("DevicePairingStartResponse")}},
				},
			},
			"/api/device/pairing/complete": map[string]any{
				"post": map[string]any{
					"summary":     "Exchange a completed pairing for a device token",
					"description": "Called by the screen app with pairingId and pairingSecret. Returns paired=false until a registered user binds the pairing code.",
					"security":    []any{},
					"requestBody": jsonBodyRef("DevicePairingCompleteRequest"),
					"responses":   map[string]any{"200": map[string]any{"description": "Device token issued", "content": jsonSchemaRef("DevicePairingCompleteResponse")}, "202": map[string]any{"description": "Not paired yet"}},
				},
			},
			"/api/device/manifest": map[string]any{
				"get": map[string]any{
					"summary":     "Read the playback manifest for one screen",
					"description": "Requires Authorization: Device <deviceToken>.",
					"security":    []any{},
					"responses":   map[string]any{"200": map[string]any{"description": "Playback manifest", "content": jsonSchemaRef("ScreenManifestResponse")}, "401": errorResponse()},
				},
			},
			"/api/device/heartbeat": map[string]any{
				"post": map[string]any{
					"summary":     "Send device heartbeat and capability details",
					"description": "Requires Authorization: Device <deviceToken>. Device information may include model, Android version, resolution, orientation, density, and WebView runtime.",
					"security":    []any{},
					"requestBody": jsonBodyRef("DeviceHeartbeatRequest"),
					"responses":   map[string]any{"200": map[string]any{"description": "Heartbeat stored", "content": jsonSchemaRef("DeviceHeartbeatResponse")}, "401": errorResponse()},
				},
			},
			"/api/device/screenshot": map[string]any{
				"post": map[string]any{
					"summary":     "Upload a requested screenshot",
					"description": "Requires Authorization: Device <deviceToken>. The requestId must match the pending screenshot command in the manifest.",
					"security":    []any{},
					"requestBody": jsonBodyRef("DeviceScreenshotRequest"),
					"responses":   map[string]any{"200": map[string]any{"description": "Screenshot stored", "content": jsonSchemaRef("DeviceScreenshotResponse")}, "409": errorResponse()},
				},
			},
			"/api/device/command/ack": map[string]any{
				"post": map[string]any{
					"summary":     "Acknowledge a screen command",
					"description": "Requires Authorization: Device <deviceToken>. The requestId must match the pending command in the manifest.",
					"security":    []any{},
					"requestBody": jsonBodyRef("DeviceCommandAckRequest"),
					"responses":   map[string]any{"200": map[string]any{"description": "Command acknowledged", "content": jsonSchemaRef("DeviceCommandAckResponse")}, "409": errorResponse()},
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
			"/api/admin/sites/{code}/pin": map[string]any{
				"patch": map[string]any{
					"summary":     "Pin or unpin a marketplace site",
					"description": "Admin required. Pinned sites appear before normal marketplace ranking; like ranking is still preserved within pinned and unpinned groups.",
					"parameters":  []map[string]any{pathParam("code", "string")},
					"requestBody": jsonBodyRef("SitePinRequest"),
					"responses": map[string]any{
						"200": map[string]any{"description": "Pin state updated", "content": jsonSchemaRef("SitePinResponse")},
						"403": errorResponse(),
					},
				},
			},
			"/api/admin/anonymous-sessions": map[string]any{
				"get": map[string]any{
					"summary":     "List recent anonymous deploy sessions",
					"description": "Admin token required in production.",
					"responses":   map[string]any{"200": map[string]any{"description": "Anonymous session list", "content": jsonSchemaRef("AnonymousSessionListResponse")}},
				},
			},
			"/api/admin/users": map[string]any{
				"get": map[string]any{
					"summary":     "List registered users",
					"description": "Admin token required. Returns account, email verification, role, status, and deploy quota fields.",
					"responses":   map[string]any{"200": map[string]any{"description": "User list", "content": jsonSchemaRef("UserListResponse")}, "403": errorResponse()},
				},
				"post": map[string]any{
					"summary":     "Create a registered user",
					"description": "Admin token required. Optional email can be marked verified when created by an admin.",
					"requestBody": jsonBodyRef("UserCreateRequest"),
					"responses":   map[string]any{"200": map[string]any{"description": "User created", "content": jsonSchemaRef("UserCreateResponse")}, "400": errorResponse(), "403": errorResponse()},
				},
			},
			"/api/admin/users/{id}": map[string]any{
				"patch": map[string]any{
					"summary":     "Update a registered user",
					"description": "Admin token required. Updates username, email, email verification state, role, status, and deploy quota.",
					"parameters":  []map[string]any{pathParam("id", "string")},
					"requestBody": jsonBodyRef("UserUpdateRequest"),
					"responses":   map[string]any{"200": map[string]any{"description": "User updated", "content": jsonSchemaRef("UserUpdateResponse")}, "400": errorResponse(), "403": errorResponse(), "404": errorResponse()},
				},
				"delete": map[string]any{
					"summary":     "Delete a registered user",
					"description": "Admin token required. The last active admin and the current admin account cannot be deleted.",
					"parameters":  []map[string]any{pathParam("id", "string")},
					"responses":   map[string]any{"200": map[string]any{"description": "User deleted", "content": jsonSchemaRef("UserDeleteResponse")}, "400": errorResponse(), "403": errorResponse(), "404": errorResponse()},
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
	objectSchema := map[string]any{"type": "object", "additionalProperties": true}

	return map[string]any{
		"APIError": map[string]any{"type": "object", "properties": map[string]any{
			"success": boolSchema, "errorCode": str, "stage": str, "detail": str, "hint": str,
			"retryAfterSeconds": intSchema, "requestId": str,
		}},
		"CaptchaResponse": map[string]any{"type": "object", "properties": map[string]any{
			"success": boolSchema, "id": str, "prompt": str, "image": str,
		}},
		"EmailVerificationRequest": map[string]any{"type": "object", "required": []string{"email", "captchaId", "captcha"}, "properties": map[string]any{
			"email": str, "captchaId": str, "captcha": str,
		}},
		"EmailVerificationResponse": map[string]any{"type": "object", "properties": map[string]any{
			"success": boolSchema, "email": str, "expiresIn": intSchema,
		}},
		"RegisterRequest": map[string]any{"type": "object", "required": []string{"username", "password", "captchaId", "captcha"}, "properties": map[string]any{
			"username": str, "email": str, "password": str, "captchaId": str, "captcha": str, "emailCode": str,
		}},
		"RegisterResponse": map[string]any{"type": "object", "properties": map[string]any{
			"success": boolSchema, "userId": str, "username": str, "email": str, "emailVerified": boolSchema,
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
			"success": boolSchema, "currentBaseURL": str,
			"mode": str, "corsAllowOrigins": str,
			"embedPolicy": str, "embedAllowOrigins": str, "cooldownSeconds": intSchema,
			"appURL": map[string]any{"$ref": "#/components/schemas/AppURLConfig"},
			"limits": map[string]any{"$ref": "#/components/schemas/Limits"}, "anonymousPolicy": map[string]any{"$ref": "#/components/schemas/AnonymousPolicy"}, "version": str,
		}},
		"AppURLConfig": map[string]any{"type": "object", "properties": map[string]any{
			"appURLMode":      map[string]any{"type": "string", "enum": []string{"path", "domain", "dual"}},
			"appDomainSuffix": str, "appURLScheme": map[string]any{"type": "string", "enum": []string{"https", "http"}}, "appURLPort": str, "appPathBase": str,
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
			"appURLMode": str, "appDomainSuffix": str, "appURLScheme": str, "appURLPort": str,
			"anonymousDeployLimit": intSchema, "cooldownSeconds": intSchema,
			"maxSingleFileBytes": intSchema, "maxSiteTotalBytes": intSchema, "maxFilesPerSite": intSchema, "corsAllowOrigins": str,
			"embedPolicy": str, "embedAllowOrigins": str,
		}},
		"ConfigUpdateResponse": map[string]any{"type": "object", "properties": map[string]any{
			"success": boolSchema, "currentBaseURL": str,
			"appURL":           map[string]any{"$ref": "#/components/schemas/AppURLConfig"},
			"corsAllowOrigins": str, "embedPolicy": str, "embedAllowOrigins": str, "cooldownSeconds": intSchema,
			"limits": map[string]any{"$ref": "#/components/schemas/Limits"}, "anonymousPolicy": map[string]any{"$ref": "#/components/schemas/AnonymousPolicy"},
		}},
		"AdminSessionResponse": map[string]any{"type": "object", "properties": map[string]any{"success": boolSchema, "mode": str, "tokenId": str, "label": str, "userId": str, "username": str, "isAdmin": boolSchema}},
		"UserListItem": map[string]any{"type": "object", "properties": map[string]any{
			"id": str, "username": str, "email": str, "emailVerified": boolSchema,
			"isAdmin": boolSchema, "isActive": boolSchema,
			"deployLimit": intSchema, "deployCount": intSchema, "remaining": intSchema,
			"createdAt": timeSchema, "lastLoginAt": timeSchema,
		}},
		"UserListResponse": map[string]any{"type": "object", "properties": map[string]any{
			"success": boolSchema, "users": map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/UserListItem"}},
		}},
		"UserCreateRequest": map[string]any{"type": "object", "required": []string{"username", "password"}, "properties": map[string]any{
			"username": str, "email": str, "emailVerified": boolSchema, "password": str,
			"isAdmin": boolSchema, "deployLimit": intSchema,
		}},
		"UserCreateResponse": map[string]any{"type": "object", "properties": map[string]any{
			"success": boolSchema, "user": map[string]any{"$ref": "#/components/schemas/UserListItem"},
		}},
		"UserUpdateRequest": map[string]any{"type": "object", "properties": map[string]any{
			"username": str, "email": str, "emailVerified": boolSchema,
			"isAdmin": boolSchema, "isActive": boolSchema, "deployLimit": intSchema,
		}},
		"UserUpdateResponse": map[string]any{"type": "object", "properties": map[string]any{
			"success": boolSchema, "user": map[string]any{"$ref": "#/components/schemas/UserListItem"},
		}},
		"UserDeleteResponse": map[string]any{"type": "object", "properties": map[string]any{"success": boolSchema, "id": str}},
		"SiteListResponse":   map[string]any{"type": "object", "properties": map[string]any{"success": boolSchema, "sites": map[string]any{"type": "array", "items": map[string]any{"type": "object"}}}},
		"SitePinRequest":     map[string]any{"type": "object", "required": []string{"pinned"}, "properties": map[string]any{"pinned": boolSchema}},
		"SitePinResponse":    map[string]any{"type": "object", "properties": map[string]any{"success": boolSchema, "code": str, "isPinned": boolSchema, "pinnedAt": timeSchema}},
		"ScreenItem": map[string]any{"type": "object", "properties": map[string]any{
			"id": str, "ownerUserId": str, "ownerUsername": str, "name": str, "deviceName": str, "status": str,
			"currentSiteCode": str, "currentVersion": intSchema, "lastSeenAt": timeSchema, "appVersion": str,
			"runtime": str, "deviceInfo": objectSchema, "screenshotRequestedAt": timeSchema, "screenshotAt": timeSchema,
			"commandType": str, "commandRequestedAt": timeSchema, "commandCompletedAt": timeSchema,
			"createdAt": timeSchema, "updatedAt": timeSchema,
		}},
		"ScreenListResponse": map[string]any{"type": "object", "properties": map[string]any{
			"success": boolSchema, "screens": map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/ScreenItem"}},
		}},
		"ScreenBindRequest": map[string]any{"type": "object", "required": []string{"pairingCode"}, "properties": map[string]any{
			"pairingCode": str, "name": str,
		}},
		"ScreenBindResponse": map[string]any{"type": "object", "properties": map[string]any{
			"success": boolSchema, "screen": map[string]any{"$ref": "#/components/schemas/ScreenItem"},
		}},
		"ScreenPublishRequest": map[string]any{"type": "object", "required": []string{"code"}, "properties": map[string]any{
			"code": str, "versionNumber": intSchema,
		}},
		"ScreenPublishResponse": map[string]any{"type": "object", "properties": map[string]any{
			"success": boolSchema, "screen": map[string]any{"$ref": "#/components/schemas/ScreenItem"},
		}},
		"ScreenDeleteResponse": map[string]any{"type": "object", "properties": map[string]any{
			"success": boolSchema, "id": str,
		}},
		"ScreenCommandRequest": map[string]any{"type": "object", "required": []string{"type"}, "properties": map[string]any{
			"type": map[string]any{"type": "string", "enum": []string{"refresh", "sleep", "wake", "shutdown"}}, "payload": objectSchema,
		}},
		"ScreenCommandResponse": map[string]any{"type": "object", "properties": map[string]any{
			"success": boolSchema, "screen": map[string]any{"$ref": "#/components/schemas/ScreenItem"}, "command": map[string]any{"$ref": "#/components/schemas/ScreenDeviceCommand"},
		}},
		"ScreenScreenshotResponse": map[string]any{"type": "object", "properties": map[string]any{
			"success": boolSchema, "screen": map[string]any{"$ref": "#/components/schemas/ScreenItem"}, "screenshot": map[string]any{"$ref": "#/components/schemas/ScreenScreenshotCommand"},
		}},
		"DevicePairingStartRequest": map[string]any{"type": "object", "properties": map[string]any{
			"deviceName": str, "appVersion": str, "runtime": str, "deviceInfo": objectSchema,
		}},
		"DevicePairingStartResponse": map[string]any{"type": "object", "properties": map[string]any{
			"success": boolSchema, "screenId": str, "pairingId": str, "pairingCode": str, "pairingSecret": str, "expiresAt": timeSchema, "serverTime": timeSchema,
		}},
		"DevicePairingCompleteRequest": map[string]any{"type": "object", "required": []string{"pairingId", "pairingSecret"}, "properties": map[string]any{
			"pairingId": str, "pairingSecret": str,
		}},
		"DevicePairingCompleteResponse": map[string]any{"type": "object", "properties": map[string]any{
			"success": boolSchema, "paired": boolSchema, "deviceToken": str, "screen": map[string]any{"$ref": "#/components/schemas/ScreenItem"},
		}},
		"ScreenManifestResponse": map[string]any{"type": "object", "properties": map[string]any{
			"success": boolSchema, "screenId": str, "screenName": str, "ownerUserId": str, "ownerUsername": str,
			"mode": str, "baseUrl": str, "entryUrl": str, "siteCode": str, "version": intSchema, "mainEntry": str,
			"title": str, "description": str, "assets": map[string]any{"type": "array", "items": map[string]any{"type": "object"}},
			"accessCookie": map[string]any{"$ref": "#/components/schemas/ScreenAccessCookie"},
			"screenshot":   map[string]any{"$ref": "#/components/schemas/ScreenScreenshotCommand"},
			"command":      map[string]any{"$ref": "#/components/schemas/ScreenDeviceCommand"},
			"updatedAt":    timeSchema,
		}},
		"ScreenAccessCookie": map[string]any{"type": "object", "properties": map[string]any{
			"name": str, "value": str, "path": str, "maxAgeSeconds": intSchema, "expiresAt": timeSchema,
		}},
		"ScreenScreenshotCommand": map[string]any{"type": "object", "properties": map[string]any{
			"requestId": str, "requestedAt": timeSchema,
		}},
		"ScreenDeviceCommand": map[string]any{"type": "object", "properties": map[string]any{
			"requestId": str, "type": str, "payload": objectSchema, "requestedAt": timeSchema,
		}},
		"DeviceHeartbeatRequest": map[string]any{"type": "object", "properties": map[string]any{
			"appVersion": str, "runtime": str, "deviceInfo": objectSchema,
		}},
		"DeviceHeartbeatResponse": map[string]any{"type": "object", "properties": map[string]any{
			"success": boolSchema, "screen": map[string]any{"$ref": "#/components/schemas/ScreenItem"},
		}},
		"DeviceScreenshotRequest": map[string]any{"type": "object", "required": []string{"contentBase64", "requestId"}, "properties": map[string]any{
			"contentBase64": str, "mimeType": str, "width": intSchema, "height": intSchema, "requestId": str,
		}},
		"DeviceScreenshotResponse": map[string]any{"type": "object", "properties": map[string]any{
			"success": boolSchema, "screenId": str, "updatedAt": timeSchema,
		}},
		"DeviceCommandAckRequest": map[string]any{"type": "object", "required": []string{"requestId"}, "properties": map[string]any{
			"requestId": str, "type": str,
		}},
		"DeviceCommandAckResponse": map[string]any{"type": "object", "properties": map[string]any{
			"success": boolSchema, "screenId": str, "completedAt": timeSchema,
		}},
		"TokenCreateRequest":  map[string]any{"type": "object", "properties": map[string]any{"label": str, "ownerUserId": str, "isAdmin": boolSchema, "expiresAt": timeSchema, "ttlSeconds": intSchema}},
		"TokenCreateResponse": map[string]any{"type": "object", "properties": map[string]any{"success": boolSchema, "id": str, "token": str, "label": str, "ownerUserId": str, "isAdmin": boolSchema, "expiresAt": timeSchema, "createdAt": timeSchema}},
		"TokenListResponse":   map[string]any{"type": "object", "properties": map[string]any{"success": boolSchema, "tokens": map[string]any{"type": "array", "items": map[string]any{"type": "object"}}}},
	}
}
