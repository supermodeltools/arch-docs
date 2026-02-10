package main

import (
	"archive/zip"
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const apiBaseURL = "https://api.supermodeltools.com/v1/graphs/supermodel"
const pollTimeout = 45 * time.Minute
const defaultPollInterval = 10 * time.Second
const maxFileSize = 10 * 1024 * 1024 // 10MB

// APIResponse matches the Supermodel API response structure.
type APIResponse struct {
	Status string          `json:"status"`
	JobID  string          `json:"jobId"`
	Error  json.RawMessage `json:"error"`
	Result json.RawMessage `json:"result"`
}

// pssgConfigTemplate is the pssg.yaml configuration template.
const pssgConfigTemplate = `site:
  name: "%s"
  base_url: "%s"
  repo_url: "%s"
  description: "Architecture documentation for the %s codebase. Explore files, functions, classes, domains, and dependencies."
  author: "Supermodel"
  language: "en"

paths:
  data: "%s"
  templates: "%s"
  output: "%s"
  source_dir: "%s"

data:
  format: "markdown"
  entity_type: "entity"
  entity_slug:
    source: "filename"
  body_sections:
    - name: "Functions"
      header: "Functions"
      type: "unordered_list"
    - name: "Classes"
      header: "Classes"
      type: "unordered_list"
    - name: "Types"
      header: "Types"
      type: "unordered_list"
    - name: "Dependencies"
      header: "Dependencies"
      type: "unordered_list"
    - name: "Imported By"
      header: "Imported By"
      type: "unordered_list"
    - name: "Calls"
      header: "Calls"
      type: "unordered_list"
    - name: "Called By"
      header: "Called By"
      type: "unordered_list"
    - name: "Source Files"
      header: "Source Files"
      type: "unordered_list"
    - name: "Subdirectories"
      header: "Subdirectories"
      type: "unordered_list"
    - name: "Files"
      header: "Files"
      type: "unordered_list"
    - name: "Source"
      header: "Source"
      type: "unordered_list"
    - name: "Extends"
      header: "Extends"
      type: "unordered_list"
    - name: "Defined In"
      header: "Defined In"
      type: "unordered_list"
    - name: "Subdomains"
      header: "Subdomains"
      type: "unordered_list"
    - name: "Domain"
      header: "Domain"
      type: "unordered_list"
    - name: "faqs"
      header: "FAQs"
      type: "faq"

taxonomies:
  - name: "node_type"
    label: "Node Types"
    label_singular: "Node Type"
    field: "node_type"
    multi_value: false
    min_entities: 1
    index_description: "Browse by entity type"

  - name: "language"
    label: "Languages"
    label_singular: "Language"
    field: "language"
    multi_value: false
    min_entities: 1
    index_description: "Browse by programming language"

  - name: "domain"
    label: "Domains"
    label_singular: "Domain"
    field: "domain"
    multi_value: false
    min_entities: 1
    index_description: "Browse by architectural domain"

  - name: "subdomain"
    label: "Subdomains"
    label_singular: "Subdomain"
    field: "subdomain"
    multi_value: false
    min_entities: 1
    index_description: "Browse by architectural subdomain"

  - name: "top_directory"
    label: "Top Directories"
    label_singular: "Directory"
    field: "top_directory"
    multi_value: false
    min_entities: 1
    index_description: "Browse by top-level directory"

  - name: "extension"
    label: "File Extensions"
    label_singular: "Extension"
    field: "extension"
    multi_value: false
    min_entities: 1
    index_description: "Browse by file extension"

  - name: "tags"
    label: "Tags"
    label_singular: "Tag"
    field: "tags"
    multi_value: true
    min_entities: 1
    index_description: "Browse by tag"

pagination:
  per_page: 48
  url_pattern: "/{taxonomy}/{entry}/{page}"

structured_data:
  entity_type: "SoftwareSourceCode"
  field_mappings:
    name: "title"
    description: "description"
    programmingLanguage: "language"
    codeRepository: "repo_url"

sitemap:
  enabled: true
  max_urls_per_file: 50000
  priorities:
    homepage: 1.0
    entity: 0.8
    taxonomy_index: 0.7
    hub_page_1: 0.6
    hub_page_n: 0.4

rss:
  enabled: true
  title: "%s"
  description: "Architecture documentation for the %s codebase"

robots:
  enabled: true

llms_txt:
  enabled: true
  title: "%s"
  description: "Architecture documentation for the %s codebase"

search:
  enabled: true

templates:
  entity: "entity.html"
  homepage: "index.html"
  hub: "hub.html"
  taxonomy_index: "taxonomy_index.html"
  all_entities: "all_entities.html"

output:
  clean_before_build: true
  extract_css: "styles.css"
  extract_js: "main.js"

extra:
  cta:
    enabled: true
    heading: "Analyze Your Own Codebase"
    description: "Get architecture documentation, dependency graphs, and domain analysis for your codebase in minutes."
    button_text: "Try Supermodel Free"
    button_url: "https://dashboard.supermodeltools.com/billing/"
`

func main() {
	// Step 1: Read inputs
	apiKey := getInput("supermodel-api-key")
	if apiKey == "" {
		fatal("supermodel-api-key input is required")
	}

	siteName := getInput("site-name")
	baseURL := getInput("base-url")
	outputDir := getInput("output-dir")
	templatesDir := getInput("templates-dir")

	if outputDir == "" {
		outputDir = "./arch-docs-output"
	}

	// Step 2: Derive repo info
	ghRepo := os.Getenv("GITHUB_REPOSITORY") // e.g. "owner/repo"
	repoName := ""
	repoURL := ""
	if ghRepo != "" {
		parts := strings.SplitN(ghRepo, "/", 2)
		if len(parts) == 2 {
			repoName = parts[1]
		} else {
			repoName = ghRepo
		}
		repoURL = "https://github.com/" + ghRepo
	}

	if siteName == "" {
		if repoName != "" {
			siteName = repoName + " Architecture Docs"
		} else {
			siteName = "Architecture Docs"
		}
	}

	if baseURL == "" {
		// Default to GitHub Pages URL for the repo
		if ghRepo != "" {
			parts := strings.SplitN(ghRepo, "/", 2)
			if len(parts) == 2 {
				baseURL = "https://" + parts[0] + ".github.io/" + parts[1]
			} else {
				baseURL = repoURL
			}
		} else {
			baseURL = "https://example.com"
		}
	}

	workspaceDir := os.Getenv("GITHUB_WORKSPACE")
	if workspaceDir == "" {
		workspaceDir = "."
	}

	// Resolve output dir
	if !filepath.IsAbs(outputDir) {
		outputDir = filepath.Join(workspaceDir, outputDir)
	}

	fmt.Printf("::group::Configuration\n")
	fmt.Printf("Site name: %s\n", siteName)
	fmt.Printf("Base URL: %s\n", baseURL)
	fmt.Printf("Output dir: %s\n", outputDir)
	fmt.Printf("Repo: %s\n", ghRepo)
	fmt.Printf("Workspace: %s\n", workspaceDir)
	logGroupEnd()

	// Step 3: Zip the repo
	logGroup("Creating repository archive")
	zipPath, err := createRepoZip(workspaceDir)
	if err != nil {
		fatal("Failed to create repo zip: %v", err)
	}
	defer os.Remove(zipPath)

	info, _ := os.Stat(zipPath)
	fmt.Printf("Archive created: %s (%.2f MB)\n", zipPath, float64(info.Size())/(1024*1024))
	logGroupEnd()

	// Step 4 & 5: Call Supermodel API and poll
	logGroup("Calling Supermodel API")
	graphJSON, err := callSupermodelAPI(apiKey, zipPath)
	if err != nil {
		fatal("API call failed: %v", err)
	}
	fmt.Printf("Graph data received (%d bytes)\n", len(graphJSON))
	logGroupEnd()

	// Step 6: Save graph JSON
	logGroup("Saving graph data")
	tmpDir, err := os.MkdirTemp("", "arch-docs-*")
	if err != nil {
		fatal("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	graphPath := filepath.Join(tmpDir, "graph.json")
	if err := os.WriteFile(graphPath, graphJSON, 0644); err != nil {
		fatal("Failed to write graph JSON: %v", err)
	}
	fmt.Printf("Graph saved to %s\n", graphPath)
	logGroupEnd()

	// Step 7: Run graph2md
	logGroup("Generating markdown from graph")
	contentDir := filepath.Join(tmpDir, "content")
	if err := os.MkdirAll(contentDir, 0755); err != nil {
		fatal("Failed to create content dir: %v", err)
	}

	graph2mdArgs := []string{
		"-input", graphPath,
		"-output", contentDir,
	}
	if repoName != "" {
		graph2mdArgs = append(graph2mdArgs, "-repo", repoName)
	}
	if repoURL != "" {
		graph2mdArgs = append(graph2mdArgs, "-repo-url", repoURL)
	}

	if err := runCommand("graph2md", graph2mdArgs...); err != nil {
		fatal("graph2md failed: %v", err)
	}

	entityCount := countFiles(contentDir, ".md")
	fmt.Printf("Generated %d markdown files\n", entityCount)
	logGroupEnd()

	// Step 8: Generate pssg.yaml and run pssg build
	logGroup("Building static site")

	// Determine templates path
	tplDir := templatesDir
	if tplDir == "" {
		// Use bundled templates
		tplDir = "/app/templates"
		// If running outside Docker (e.g., local testing), fall back
		if _, err := os.Stat(tplDir); os.IsNotExist(err) {
			// Try relative to binary
			execPath, _ := os.Executable()
			tplDir = filepath.Join(filepath.Dir(execPath), "templates")
			if _, err := os.Stat(tplDir); os.IsNotExist(err) {
				tplDir = "templates"
			}
		}
	} else if !filepath.IsAbs(tplDir) {
		tplDir = filepath.Join(workspaceDir, tplDir)
	}

	configPath := filepath.Join(tmpDir, "pssg.yaml")
	if err := generateConfig(configPath, siteName, baseURL, repoURL, repoName, contentDir, tplDir, outputDir, workspaceDir); err != nil {
		fatal("Failed to generate pssg config: %v", err)
	}

	if err := runCommand("pssg", "build", "--config", configPath); err != nil {
		fatal("pssg build failed: %v", err)
	}

	pageCount := countFiles(outputDir, ".html")
	fmt.Printf("Built %d HTML pages\n", pageCount)
	logGroupEnd()

	// Step 8b: Rewrite paths if base URL has a path prefix (e.g. GitHub Pages subdirectory)
	pathPrefix := extractPathPrefix(baseURL)
	if pathPrefix != "" {
		logGroup("Rewriting paths for subdirectory deployment")
		fmt.Printf("Path prefix: %s\n", pathPrefix)
		if err := rewritePathPrefix(outputDir, pathPrefix); err != nil {
			fatal("Failed to rewrite paths: %v", err)
		}
		logGroupEnd()
	}

	// Step 9: Set outputs
	logGroup("Setting outputs")
	absOutput, _ := filepath.Abs(outputDir)
	setOutput("site-path", absOutput)
	setOutput("entity-count", strconv.Itoa(entityCount))
	setOutput("page-count", strconv.Itoa(pageCount))
	fmt.Printf("site-path=%s\n", absOutput)
	fmt.Printf("entity-count=%d\n", entityCount)
	fmt.Printf("page-count=%d\n", pageCount)
	logGroupEnd()

	fmt.Println("Architecture docs generated successfully!")
}

// getInput reads a GitHub Actions input from the environment.
func getInput(name string) string {
	// Docker actions receive env vars with hyphens preserved: INPUT_SUPERMODEL-API-KEY
	envKey := "INPUT_" + strings.ToUpper(name)
	val := os.Getenv(envKey)
	if val == "" {
		// Also check with hyphens replaced by underscores (for local testing)
		envKey = "INPUT_" + strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
		val = os.Getenv(envKey)
	}
	return strings.TrimSpace(val)
}

// setOutput writes a GitHub Actions output value.
func setOutput(name, value string) {
	outputFile := os.Getenv("GITHUB_OUTPUT")
	if outputFile == "" {
		fmt.Printf("::set-output name=%s::%s\n", name, value)
		return
	}
	f, err := os.OpenFile(outputFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("::warning::Failed to write output %s: %v\n", name, err)
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s=%s\n", name, value)
}

// logGroup starts a GitHub Actions log group.
func logGroup(name string) {
	fmt.Printf("::group::%s\n", name)
}

// logGroupEnd ends a GitHub Actions log group.
func logGroupEnd() {
	fmt.Println("::endgroup::")
}

// fatal prints an error and exits.
func fatal(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Printf("::error::%s\n", msg)
	os.Exit(1)
}

// createRepoZip walks the workspace directory and creates a zip archive.
// It skips .git/, node_modules/, binary files, and files > 10MB.
func createRepoZip(workspaceDir string) (string, error) {
	tmpFile, err := os.CreateTemp("", "repo-*.zip")
	if err != nil {
		return "", fmt.Errorf("creating temp file: %w", err)
	}
	defer tmpFile.Close()

	zw := zip.NewWriter(tmpFile)
	defer zw.Close()

	skipDirs := map[string]bool{
		".git":         true,
		"node_modules": true,
		".next":        true,
		"dist":         true,
		"build":        true,
		"vendor":       true,
		"__pycache__":  true,
		".venv":        true,
	}

	binaryExts := map[string]bool{
		".exe": true, ".dll": true, ".so": true, ".dylib": true,
		".bin": true, ".obj": true, ".o": true, ".a": true,
		".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
		".ico": true, ".svg": true, ".webp": true,
		".mp3": true, ".mp4": true, ".avi": true, ".mov": true,
		".zip": true, ".tar": true, ".gz": true, ".bz2": true,
		".rar": true, ".7z": true,
		".woff": true, ".woff2": true, ".ttf": true, ".eot": true,
		".pdf": true, ".doc": true, ".docx": true,
	}

	fileCount := 0
	err = filepath.Walk(workspaceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}

		relPath, err := filepath.Rel(workspaceDir, path)
		if err != nil {
			return nil
		}

		// Skip root
		if relPath == "." {
			return nil
		}

		// Skip hidden dirs and known large dirs
		baseName := filepath.Base(path)
		if info.IsDir() {
			if skipDirs[baseName] || strings.HasPrefix(baseName, ".") {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip binary files
		ext := strings.ToLower(filepath.Ext(path))
		if binaryExts[ext] {
			return nil
		}

		// Skip files > 10MB
		if info.Size() > maxFileSize {
			return nil
		}

		// Skip hidden files
		if strings.HasPrefix(baseName, ".") {
			return nil
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return nil
		}
		header.Name = relPath
		header.Method = zip.Deflate

		writer, err := zw.CreateHeader(header)
		if err != nil {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer file.Close()

		_, err = io.Copy(writer, file)
		if err != nil {
			return nil
		}

		fileCount++
		return nil
	})

	if err != nil {
		return "", fmt.Errorf("walking workspace: %w", err)
	}

	fmt.Printf("Archived %d files\n", fileCount)
	return tmpFile.Name(), nil
}

// callSupermodelAPI sends the zip to the Supermodel API and polls for completion.
func callSupermodelAPI(apiKey, zipPath string) ([]byte, error) {
	idempotencyKey := generateUUID()

	// Initial POST
	respBody, resp, err := postWithZip(apiKey, zipPath, idempotencyKey)
	if err != nil {
		return nil, fmt.Errorf("initial request: %w", err)
	}

	var apiResp APIResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("parsing response: %w (body: %s)", err, string(respBody))
	}

	if apiResp.Status == "completed" {
		return apiResp.Result, nil
	}

	if apiResp.Status == "failed" {
		return nil, fmt.Errorf("API returned failure: %s", string(apiResp.Error))
	}

	// Poll loop
	deadline := time.Now().Add(pollTimeout)
	for time.Now().Before(deadline) {
		interval := getPollInterval(resp, defaultPollInterval)
		fmt.Printf("Status: %s (job: %s), polling in %s...\n", apiResp.Status, apiResp.JobID, interval)
		time.Sleep(interval)

		respBody, resp, err = postWithZip(apiKey, zipPath, idempotencyKey)
		if err != nil {
			fmt.Printf("::warning::Poll request failed: %v, retrying...\n", err)
			continue
		}

		if err := json.Unmarshal(respBody, &apiResp); err != nil {
			return nil, fmt.Errorf("parsing poll response: %w", err)
		}

		if apiResp.Status == "completed" {
			return apiResp.Result, nil
		}

		if apiResp.Status == "failed" {
			return nil, fmt.Errorf("API returned failure: %s", string(apiResp.Error))
		}
	}

	return nil, fmt.Errorf("timeout waiting for API response after %s", pollTimeout)
}

// postWithZip sends a multipart POST request with the zip file.
func postWithZip(apiKey, zipPath, idempotencyKey string) ([]byte, *http.Response, error) {
	body, contentType, err := createMultipartBody(zipPath)
	if err != nil {
		return nil, nil, err
	}

	req, err := http.NewRequest("POST", apiBaseURL, body)
	if err != nil {
		return nil, nil, err
	}

	req.Header.Set("Content-Type", contentType)
	req.Header.Set("X-Api-Key", apiKey)
	req.Header.Set("Idempotency-Key", idempotencyKey)

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, resp, nil
}

// createMultipartBody builds the multipart form body with the zip file.
func createMultipartBody(zipPath string) (*bytes.Buffer, string, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	file, err := os.Open(zipPath)
	if err != nil {
		return nil, "", fmt.Errorf("opening zip: %w", err)
	}
	defer file.Close()

	part, err := writer.CreateFormFile("file", filepath.Base(zipPath))
	if err != nil {
		return nil, "", fmt.Errorf("creating form file: %w", err)
	}

	if _, err := io.Copy(part, file); err != nil {
		return nil, "", fmt.Errorf("copying zip data: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, "", fmt.Errorf("closing multipart writer: %w", err)
	}

	return body, writer.FormDataContentType(), nil
}

// generateUUID generates a UUID v4.
func generateUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// getPollInterval reads the Retry-After header or returns the default.
func getPollInterval(resp *http.Response, defaultInterval time.Duration) time.Duration {
	if resp == nil {
		return defaultInterval
	}
	retryAfter := resp.Header.Get("Retry-After")
	if retryAfter == "" {
		return defaultInterval
	}
	seconds, err := strconv.Atoi(retryAfter)
	if err != nil {
		return defaultInterval
	}
	if seconds < 1 {
		return defaultInterval
	}
	if seconds > 120 {
		seconds = 120
	}
	return time.Duration(seconds) * time.Second
}

// generateConfig writes a pssg.yaml config file.
func generateConfig(configPath, siteName, baseURL, repoURL, repoName, contentDir, tplDir, outputDir, sourceDir string) error {
	config := fmt.Sprintf(pssgConfigTemplate,
		siteName,       // site.name
		baseURL,        // site.base_url
		repoURL,        // site.repo_url
		repoName,       // site.description
		contentDir,     // paths.data
		tplDir,         // paths.templates
		outputDir,      // paths.output
		sourceDir,      // paths.source_dir
		siteName,       // rss.title
		repoName,       // rss.description
		siteName,       // llms_txt.title
		repoName,       // llms_txt.description
	)
	return os.WriteFile(configPath, []byte(config), 0644)
}

// runCommand executes an external command with stdout/stderr forwarding.
func runCommand(name string, args ...string) error {
	fmt.Printf("Running: %s %s\n", name, strings.Join(args, " "))
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// extractPathPrefix returns the path component of a URL (e.g. "/graph2md" from
// "https://supermodeltools.github.io/graph2md"). Returns "" if no prefix.
func extractPathPrefix(baseURL string) string {
	u, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	p := strings.TrimRight(u.Path, "/")
	if p == "" || p == "/" {
		return ""
	}
	return p
}

// rewritePathPrefix rewrites all root-relative paths in HTML and JS files to
// include the given prefix. This is needed when deploying to a subdirectory
// (e.g. GitHub Pages project sites at username.github.io/repo/).
func rewritePathPrefix(dir, prefix string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".html" && ext != ".js" {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		content := string(data)
		original := content

		// Rewrite href="/..." to href="/prefix/..."
		content = strings.ReplaceAll(content, `href="/`, `href="`+prefix+`/`)
		// Rewrite src="/..." to src="/prefix/..."
		content = strings.ReplaceAll(content, `src="/`, `src="`+prefix+`/`)
		// Rewrite fetch("/..." to fetch("/prefix/..."
		content = strings.ReplaceAll(content, `fetch("/`, `fetch("`+prefix+`/`)
		// Rewrite JS navigation: window.location.href = "/" + ...
		content = strings.ReplaceAll(content, `window.location.href = "/"`, `window.location.href = "`+prefix+`/"`)
		content = strings.ReplaceAll(content, `window.location.href = "/" + `, `window.location.href = "`+prefix+`/" + `)

		if content != original {
			if err := os.WriteFile(path, []byte(content), info.Mode()); err != nil {
				return fmt.Errorf("writing %s: %w", path, err)
			}
		}

		return nil
	})
}

// countFiles counts files with the given extension in a directory tree.
func countFiles(dir, ext string) int {
	count := 0
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && strings.HasSuffix(path, ext) {
			count++
		}
		return nil
	})
	return count
}
