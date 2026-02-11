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
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const apiBase = "https://api.supermodeltools.com"
const supermodelEndpoint = apiBase + "/v1/graphs/supermodel"
const impactEndpoint = apiBase + "/v1/analysis/impact"
const testCoverageEndpoint = apiBase + "/v1/analysis/test-coverage-map"
const circularDepsEndpoint = apiBase + "/v1/analysis/circular-dependencies"

const pollTimeout = 15 * time.Minute
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
    - name: "Impact Analysis"
      header: "Impact Analysis"
      type: "unordered_list"
    - name: "Test Coverage"
      header: "Test Coverage"
      type: "unordered_list"
    - name: "Circular Dependencies"
      header: "Circular Dependencies"
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

  - name: "test_coverage"
    label: "Test Coverage"
    label_singular: "Coverage"
    field: "test_coverage"
    multi_value: false
    min_entities: 1
    index_description: "Browse by test coverage status"

  - name: "impact_level"
    label: "Impact Level"
    label_singular: "Impact Level"
    field: "impact_level"
    multi_value: false
    min_entities: 1
    index_description: "Browse by change impact level"

  - name: "dependency_health"
    label: "Dependency Health"
    label_singular: "Dependency Health"
    field: "dependency_health"
    multi_value: false
    min_entities: 1
    index_description: "Browse by dependency health status"

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

	// Step 4 & 5: Call Supermodel APIs in parallel
	logGroup("Calling Supermodel APIs")

	type apiResult struct {
		name string
		data []byte
		err  error
	}

	results := make(chan apiResult, 4)
	var wg sync.WaitGroup

	// Launch all 4 endpoints concurrently
	for _, ep := range []struct{ name, url string }{
		{"supermodel", supermodelEndpoint},
		{"impact", impactEndpoint},
		{"test-coverage", testCoverageEndpoint},
		{"circular-deps", circularDepsEndpoint},
	} {
		wg.Add(1)
		go func(name, url string) {
			defer wg.Done()
			fmt.Printf("Calling %s endpoint...\n", name)
			data, err := callEndpoint(url, apiKey, zipPath)
			results <- apiResult{name: name, data: data, err: err}
		}(ep.name, ep.url)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var graphJSON []byte
	var impactJSON []byte
	var testCoverageJSON []byte
	var circularDepsJSON []byte

	for r := range results {
		if r.err != nil {
			if r.name == "supermodel" {
				fatal("Supermodel API failed: %v", r.err)
			}
			fmt.Printf("::warning::%s endpoint failed: %v\n", r.name, r.err)
			continue
		}
		fmt.Printf("%s: received %d bytes\n", r.name, len(r.data))
		switch r.name {
		case "supermodel":
			graphJSON = r.data
		case "impact":
			impactJSON = r.data
		case "test-coverage":
			testCoverageJSON = r.data
		case "circular-deps":
			circularDepsJSON = r.data
		}
	}
	logGroupEnd()

	// Step 6: Save JSON results
	logGroup("Saving analysis data")
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

	if impactJSON != nil {
		p := filepath.Join(tmpDir, "impact.json")
		os.WriteFile(p, impactJSON, 0644)
		fmt.Printf("Impact analysis saved to %s\n", p)
	}
	if testCoverageJSON != nil {
		p := filepath.Join(tmpDir, "test-coverage.json")
		os.WriteFile(p, testCoverageJSON, 0644)
		fmt.Printf("Test coverage saved to %s\n", p)
	}
	if circularDepsJSON != nil {
		p := filepath.Join(tmpDir, "circular-deps.json")
		os.WriteFile(p, circularDepsJSON, 0644)
		fmt.Printf("Circular dependencies saved to %s\n", p)
	}
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

	// Step 7b: Enrich markdown with analysis data
	if impactJSON != nil || testCoverageJSON != nil || circularDepsJSON != nil {
		logGroup("Enriching entities with analysis data")
		enriched := enrichMarkdown(contentDir, impactJSON, testCoverageJSON, circularDepsJSON)
		fmt.Printf("Enriched %d entity files with analysis data\n", enriched)
		logGroupEnd()
	}

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

// callEndpoint sends the zip to a Supermodel API endpoint and polls for completion.
func callEndpoint(endpointURL, apiKey, zipPath string) ([]byte, error) {
	idempotencyKey := generateUUID()

	// Initial POST
	respBody, resp, err := postWithZip(endpointURL, apiKey, zipPath, idempotencyKey)
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

		respBody, resp, err = postWithZip(endpointURL, apiKey, zipPath, idempotencyKey)
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
func postWithZip(endpointURL, apiKey, zipPath, idempotencyKey string) ([]byte, *http.Response, error) {
	body, contentType, err := createMultipartBody(zipPath)
	if err != nil {
		return nil, nil, err
	}

	req, err := http.NewRequest("POST", endpointURL, body)
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

// --- Analysis response types ---

type ImpactResponse struct {
	Impacts []ImpactEntry `json:"impacts"`
}

type ImpactEntry struct {
	Target struct {
		File string `json:"file"`
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"target"`
	BlastRadius struct {
		DirectDependents     int     `json:"directDependents"`
		TransitiveDependents int     `json:"transitiveDependents"`
		AffectedFiles        int     `json:"affectedFiles"`
		RiskScore            float64 `json:"riskScore"`
	} `json:"blastRadius"`
	AffectedFunctions []struct {
		File         string `json:"file"`
		Name         string `json:"name"`
		Distance     int    `json:"distance"`
		Relationship string `json:"relationship"`
	} `json:"affectedFunctions"`
	EntryPointsAffected []struct {
		File string `json:"file"`
		Name string `json:"name"`
	} `json:"entryPointsAffected"`
}

type TestCoverageResponse struct {
	Metadata struct {
		CoveragePercentage float64 `json:"coveragePercentage"`
		TestedFunctions    int     `json:"testedFunctions"`
		UntestedFunctions  int     `json:"untestedFunctions"`
	} `json:"metadata"`
	UntestedFunctions []struct {
		File       string `json:"file"`
		Name       string `json:"name"`
		Line       int    `json:"line"`
		Type       string `json:"type"`
		Confidence string `json:"confidence"`
		Reason     string `json:"reason"`
	} `json:"untestedFunctions"`
	TestedFunctions []struct {
		File      string   `json:"file"`
		Name      string   `json:"name"`
		Line      int      `json:"line"`
		TestFiles []string `json:"testFiles"`
	} `json:"testedFunctions"`
	CoverageByFile []struct {
		File               string  `json:"file"`
		CoveragePercentage float64 `json:"coveragePercentage"`
	} `json:"coverageByFile"`
}

type CircularDepsResponse struct {
	Cycles []struct {
		ID    string   `json:"id"`
		Files []string `json:"files"`
		Edges []struct {
			Source          string   `json:"source"`
			Target          string   `json:"target"`
			ImportedSymbols []string `json:"importedSymbols"`
		} `json:"edges"`
		Severity          string `json:"severity"`
		BreakingSuggestion string `json:"breakingSuggestion"`
	} `json:"cycles"`
	Summary struct {
		TotalCycles       int `json:"totalCycles"`
		HighSeverityCount int `json:"highSeverityCount"`
	} `json:"summary"`
}

// enrichMarkdown reads analysis JSON and injects data into generated markdown frontmatter.
func enrichMarkdown(contentDir string, impactJSON, testCoverageJSON, circularDepsJSON []byte) int {
	// Parse analysis data
	impactByFile := map[string]ImpactEntry{}
	if impactJSON != nil {
		var impact ImpactResponse
		if err := json.Unmarshal(impactJSON, &impact); err == nil {
			for _, entry := range impact.Impacts {
				impactByFile[entry.Target.File] = entry
			}
		}
	}

	testedFuncs := map[string][]string{}      // "file:name" -> test files
	untestedFuncs := map[string]string{}       // "file:name" -> reason
	coverageByFile := map[string]float64{}
	testedCountByFile := map[string]int{}
	totalCountByFile := map[string]int{}
	testedNamesInFile := map[string][]string{}
	untestedNamesInFile := map[string][]string{}
	if testCoverageJSON != nil {
		var coverage TestCoverageResponse
		if err := json.Unmarshal(testCoverageJSON, &coverage); err == nil {
			for _, f := range coverage.TestedFunctions {
				key := f.File + ":" + f.Name
				testedFuncs[key] = f.TestFiles
				testedCountByFile[f.File]++
				totalCountByFile[f.File]++
				testedNamesInFile[f.File] = append(testedNamesInFile[f.File], f.Name)
			}
			for _, f := range coverage.UntestedFunctions {
				key := f.File + ":" + f.Name
				untestedFuncs[key] = f.Reason
				totalCountByFile[f.File]++
				untestedNamesInFile[f.File] = append(untestedNamesInFile[f.File], f.Name)
			}
			for _, f := range coverage.CoverageByFile {
				coverageByFile[f.File] = f.CoveragePercentage
			}
		}
	}

	filesInCycles := map[string][]string{} // file -> cycle IDs
	cycleDetails := map[string]string{}    // cycle ID -> severity
	cycleSuggestions := map[string]string{} // cycle ID -> suggestion
	if circularDepsJSON != nil {
		var circular CircularDepsResponse
		if err := json.Unmarshal(circularDepsJSON, &circular); err == nil {
			for _, cycle := range circular.Cycles {
				cycleDetails[cycle.ID] = cycle.Severity
				cycleSuggestions[cycle.ID] = cycle.BreakingSuggestion
				for _, f := range cycle.Files {
					filesInCycles[f] = append(filesInCycles[f], cycle.ID)
				}
			}
		}
	}

	enriched := 0
	filepath.Walk(contentDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		content := string(data)

		// Find frontmatter boundaries
		if !strings.HasPrefix(content, "---\n") {
			return nil
		}
		endIdx := strings.Index(content[4:], "\n---\n")
		if endIdx < 0 {
			return nil
		}
		endIdx += 4

		frontmatter := content[:endIdx]
		body := content[endIdx+5:] // skip "\n---\n"

		// Extract file_path from frontmatter
		filePath := extractFrontmatterValue(frontmatter, "file_path")
		funcName := extractFrontmatterValue(frontmatter, "function_name")
		nodeType := extractFrontmatterValue(frontmatter, "node_type")

		var additions []string
		var bodySections []string
		modified := false

		// Impact analysis enrichment
		if filePath != "" {
			if impact, ok := impactByFile[filePath]; ok {
				level := "Low"
				if impact.BlastRadius.RiskScore >= 30 {
					level = "High"
				} else if impact.BlastRadius.RiskScore >= 10 {
					level = "Medium"
				}
				if impact.BlastRadius.RiskScore > 100 {
					level = "Critical"
				}
				additions = append(additions,
					fmt.Sprintf("impact_level: \"%s\"", level),
					fmt.Sprintf("impact_risk_score: %.1f", impact.BlastRadius.RiskScore),
					fmt.Sprintf("impact_direct_dependents: %d", impact.BlastRadius.DirectDependents),
					fmt.Sprintf("impact_transitive_dependents: %d", impact.BlastRadius.TransitiveDependents),
					fmt.Sprintf("impact_affected_files: %d", impact.BlastRadius.AffectedFiles),
				)

				var impactLines []string
				impactLines = append(impactLines, fmt.Sprintf("- Risk Score: %.1f (%s)", impact.BlastRadius.RiskScore, level))
				impactLines = append(impactLines, fmt.Sprintf("- Direct Dependents: %d", impact.BlastRadius.DirectDependents))
				impactLines = append(impactLines, fmt.Sprintf("- Transitive Dependents: %d", impact.BlastRadius.TransitiveDependents))
				impactLines = append(impactLines, fmt.Sprintf("- Affected Files: %d", impact.BlastRadius.AffectedFiles))
				if len(impact.EntryPointsAffected) > 0 {
					impactLines = append(impactLines, fmt.Sprintf("- Entry Points Affected: %d", len(impact.EntryPointsAffected)))
					for _, ep := range impact.EntryPointsAffected {
						if len(impactLines) > 10 {
							break
						}
						impactLines = append(impactLines, fmt.Sprintf("  - %s (%s)", ep.Name, ep.File))
					}
				}
				bodySections = append(bodySections, "## Impact Analysis\n\n"+strings.Join(impactLines, "\n"))
				modified = true
			}
		}

		// Test coverage enrichment
		if funcName != "" && filePath != "" && (nodeType == "Function" || nodeType == "Method") {
			key := filePath + ":" + funcName
			if testFiles, ok := testedFuncs[key]; ok {
				additions = append(additions, `test_coverage: "Tested"`)
				var lines []string
				lines = append(lines, fmt.Sprintf("- Status: Tested by %d test file(s)", len(testFiles)))
				for _, tf := range testFiles {
					lines = append(lines, fmt.Sprintf("  - %s", tf))
				}
				bodySections = append(bodySections, "## Test Coverage\n\n"+strings.Join(lines, "\n"))
				modified = true
			} else if reason, ok := untestedFuncs[key]; ok {
				additions = append(additions, `test_coverage: "Untested"`)
				bodySections = append(bodySections, "## Test Coverage\n\n- Status: Untested\n- Reason: "+reason)
				modified = true
			}
		}
		if nodeType == "File" && filePath != "" {
			if cov, ok := coverageByFile[filePath]; ok {
				covStatus := "Tested"
				if cov == 0 {
					covStatus = "Untested"
				}
				tc := testedCountByFile[filePath]
				tot := totalCountByFile[filePath]
				additions = append(additions,
					fmt.Sprintf("test_coverage: \"%s\"", covStatus),
					fmt.Sprintf("test_coverage_pct: %.1f", cov),
				)
				var covLines []string
				covLines = append(covLines, coverageBarHTML(filePath, cov, tc, tot))
				for _, name := range testedNamesInFile[filePath] {
					covLines = append(covLines, fmt.Sprintf(`<span class="cov-func"><span class="cov-check">✓</span> %s</span>`, name))
				}
				for _, name := range untestedNamesInFile[filePath] {
					covLines = append(covLines, fmt.Sprintf(`<span class="cov-func"><span class="cov-x">✗</span> %s</span>`, name))
				}
				bodySections = append(bodySections, "## Test Coverage\n\n"+joinCoverageItems(covLines))
				modified = true
			}
		}

		// Directory/Module/Package entities: show child file coverage bars
		if (nodeType == "Directory" || nodeType == "Module" || nodeType == "Package" || nodeType == "Namespace") && filePath != "" && len(coverageByFile) > 0 {
			prefix := filePath
			if !strings.HasSuffix(prefix, "/") {
				prefix += "/"
			}
			type fileCov struct {
				file   string
				pct    float64
				tested int
				total  int
			}
			var childFiles []fileCov
			for f, pct := range coverageByFile {
				if strings.HasPrefix(f, prefix) {
					childFiles = append(childFiles, fileCov{f, pct, testedCountByFile[f], totalCountByFile[f]})
				}
			}
			if len(childFiles) > 0 {
				sort.Slice(childFiles, func(i, j int) bool {
					return childFiles[i].pct < childFiles[j].pct
				})
				var covLines []string
				for _, cf := range childFiles {
					covLines = append(covLines, coverageBarHTML(cf.file, cf.pct, cf.tested, cf.total))
				}
				bodySections = append(bodySections, "## Test Coverage\n\n"+joinCoverageItems(covLines))
				totalTested := 0
				totalAll := 0
				for _, cf := range childFiles {
					totalTested += cf.tested
					totalAll += cf.total
				}
				overallPct := 0.0
				if totalAll > 0 {
					overallPct = float64(totalTested) / float64(totalAll) * 100
				}
				covStatus := "Tested"
				if overallPct == 0 {
					covStatus = "Untested"
				}
				additions = append(additions,
					fmt.Sprintf("test_coverage: \"%s\"", covStatus),
					fmt.Sprintf("test_coverage_pct: %.1f", overallPct),
				)
				modified = true
			}
		}

		// Circular dependency enrichment
		if filePath != "" {
			if cycleIDs, ok := filesInCycles[filePath]; ok {
				additions = append(additions, `dependency_health: "In Cycle"`)
				var lines []string
				for _, id := range cycleIDs {
					sev := cycleDetails[id]
					lines = append(lines, fmt.Sprintf("- %s (severity: %s)", id, sev))
					if suggestion := cycleSuggestions[id]; suggestion != "" {
						lines = append(lines, fmt.Sprintf("  - Suggestion: %s", suggestion))
					}
				}
				bodySections = append(bodySections, "## Circular Dependencies\n\n"+strings.Join(lines, "\n"))
				modified = true
			} else if nodeType == "File" {
				additions = append(additions, `dependency_health: "Clean"`)
				modified = true
			}
		}

		if !modified {
			return nil
		}

		// Rebuild file: insert new frontmatter fields before closing ---
		newFrontmatter := frontmatter + "\n" + strings.Join(additions, "\n")
		newBody := body
		if len(bodySections) > 0 {
			newBody = strings.Join(bodySections, "\n\n") + "\n\n" + body
		}

		newContent := newFrontmatter + "\n---\n" + newBody
		os.WriteFile(path, []byte(newContent), info.Mode())
		enriched++
		return nil
	})

	return enriched
}

// extractFrontmatterValue extracts a simple string value from YAML frontmatter.
func extractFrontmatterValue(frontmatter, key string) string {
	for _, line := range strings.Split(frontmatter, "\n") {
		line = strings.TrimSpace(line)
		prefix := key + ":"
		if strings.HasPrefix(line, prefix) {
			val := strings.TrimSpace(line[len(prefix):])
			val = strings.Trim(val, `"'`)
			return val
		}
	}
	return ""
}

// coverageBarHTML generates an HTML coverage bar row.
func coverageBarHTML(label string, pct float64, tested, total int) string {
	color := "var(--red)"
	if pct >= 80 {
		color = "var(--green)"
	} else if pct > 0 {
		color = "var(--orange)"
	}
	return fmt.Sprintf(
		`<div class="cov-row">`+
			`<div class="cov-bar"><div class="cov-fill" style="width:%.1f%%;background:%s"></div></div>`+
			`<span class="cov-pct">%.1f%%</span>`+
			`<span class="cov-label">%s</span>`+
			`<span class="cov-ratio">(%d/%d)</span>`+
			`</div>`,
		pct, color, pct, label, tested, total,
	)
}

// joinCoverageItems wraps HTML strings as markdown list items.
func joinCoverageItems(items []string) string {
	var lines []string
	for _, item := range items {
		lines = append(lines, "- "+item)
	}
	return strings.Join(lines, "\n")
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
