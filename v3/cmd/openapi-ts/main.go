package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type operation struct {
	OperationID string `yaml:"operationId"`
}

type specification struct {
	Paths map[string]map[string]yaml.Node `yaml:"paths"`
}

type generatedOperation struct {
	ID, Method, Path string
}

func main() {
	specPath := flag.String("spec", "api/openapi.yaml", "OpenAPI source")
	outPath := flag.String("out", "web/src/generated/client.ts", "generated TypeScript client")
	flag.Parse()
	body, err := os.ReadFile(*specPath)
	if err != nil {
		panic(err)
	}
	var spec specification
	if err = yaml.Unmarshal(body, &spec); err != nil {
		panic(err)
	}
	methods := map[string]bool{"get": true, "post": true, "put": true, "patch": true, "delete": true}
	var operations []generatedOperation
	seen := map[string]bool{}
	for path, entries := range spec.Paths {
		for method, node := range entries {
			method = strings.ToLower(method)
			if !methods[method] {
				continue
			}
			var item operation
			if err = node.Decode(&item); err != nil {
				panic(fmt.Sprintf("decode %s %s: %v", method, path, err))
			}
			if item.OperationID == "" {
				continue
			}
			if seen[item.OperationID] {
				panic("duplicate operationId: " + item.OperationID)
			}
			seen[item.OperationID] = true
			operations = append(operations, generatedOperation{ID: item.OperationID, Method: strings.ToUpper(method), Path: path})
		}
	}
	sort.Slice(operations, func(i, j int) bool { return operations[i].ID < operations[j].ID })
	if len(operations) == 0 {
		panic("OpenAPI contains no operations")
	}
	var out strings.Builder
	out.WriteString("// Code generated from api/openapi.yaml by cmd/openapi-ts; DO NOT EDIT.\n\n")
	out.WriteString("export const operations = {\n")
	for _, item := range operations {
		fmt.Fprintf(&out, "  %s: { method: %q, path: %q },\n", item.ID, item.Method, item.Path)
	}
	out.WriteString("} as const;\n\n")
	out.WriteString("export type OperationId = keyof typeof operations;\n")
	out.WriteString("export type RequestOptions = { path?: Record<string, string | number>; query?: Record<string, string | number | boolean | undefined | null>; body?: unknown; headers?: HeadersInit; signal?: AbortSignal };\n\n")
	out.WriteString(`export class GeneratedApiClient {
  constructor(private readonly baseURL = "/api/v3", private readonly csrfCookie = "sakura_v3_session_csrf") {}

  async call<T>(operationId: OperationId, options: RequestOptions = {}): Promise<T> {
    const operation = operations[operationId];
    let path: string = operation.path;
    const usesApiBase = path.startsWith("/api/v3");
    if (usesApiBase) path = path.slice("/api/v3".length) || "/";
    for (const [key, value] of Object.entries(options.path || {})) {
      path = path.replace("{" + key + "}", encodeURIComponent(String(value)));
    }
    if (/[{][^}]+[}]/.test(path)) throw new Error("Missing path parameter for " + operationId);
    const url = new URL((usesApiBase ? this.baseURL.replace(/\/$/, "") : "") + path, window.location.origin);
    for (const [key, value] of Object.entries(options.query || {})) {
      if (value !== undefined && value !== null && value !== "") url.searchParams.set(key, String(value));
    }
    const headers = new Headers(options.headers);
    if (options.body !== undefined) headers.set("Content-Type", "application/json");
    if (!["GET", "HEAD"].includes(operation.method)) {
      const cookies = document.cookie.split("; ");
      const csrfEntry = cookies.find((entry) => entry.startsWith(this.csrfCookie + "=")) || cookies.find((entry) => entry.split("=", 1)[0].endsWith("_csrf"));
      const csrf = csrfEntry?.split("=").slice(1).join("=");
      if (csrf) headers.set("X-CSRF-Token", decodeURIComponent(csrf));
    }
    const response = await fetch(url, { method: operation.method, headers, credentials: "include", body: options.body === undefined ? undefined : JSON.stringify(options.body), signal: options.signal });
    if (response.status === 204) return undefined as T;
    const contentType = response.headers.get("content-type") || "";
    const payload = contentType.includes("json") ? await response.json() : await response.text();
    if (!response.ok) {
      const message = typeof payload === "object" && payload && "message" in payload ? String(payload.message) : "Request failed (" + response.status + ")";
      throw new ApiError(message, response.status, payload);
    }
    return payload as T;
  }
}

export class ApiError extends Error {
  constructor(message: string, readonly status: number, readonly detail: unknown) { super(message); }
}
`)
	if err = os.MkdirAll(filepath.Dir(*outPath), 0o755); err != nil {
		panic(err)
	}
	if err = os.WriteFile(*outPath, []byte(out.String()), 0o644); err != nil {
		panic(err)
	}
}
