package graph2md

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- helpers ---

func makeDomainNode(id, name string) Node {
	return Node{
		ID:     id,
		Labels: []string{"Domain"},
		Properties: map[string]interface{}{
			"name": name,
		},
	}
}

func makeSubdomainNode(id, name string) Node {
	return Node{
		ID:     id,
		Labels: []string{"Subdomain"},
		Properties: map[string]interface{}{
			"name": name,
		},
	}
}

func makeFileNode(id, name, path string) Node {
	return Node{
		ID:     id,
		Labels: []string{"File"},
		Properties: map[string]interface{}{
			"name": name,
			"path": path,
		},
	}
}

func makeRel(id, relType, startNode, endNode string) Relationship {
	return Relationship{
		ID:        id,
		Type:      relType,
		StartNode: startNode,
		EndNode:   endNode,
	}
}

// domainRenderCtx builds a minimal renderContext for a Domain node with
// the provided related-domain edges. nodeLookup and slugLookup are
// populated for all supplied nodes.
func domainRenderCtx(center Node, allNodes []Node, outEdges, inEdges []domainEdge) *renderContext {
	nodeLookup := make(map[string]*Node)
	slugLookup := make(map[string]string)
	for i := range allNodes {
		n := &allNodes[i]
		nodeLookup[n.ID] = n
		slugLookup[n.ID] = toSlug(getStr(n.Properties, "name"))
	}
	nodeLookup[center.ID] = &center
	slugLookup[center.ID] = toSlug(getStr(center.Properties, "name"))

	return &renderContext{
		node:                &center,
		label:               "Domain",
		slug:                slugLookup[center.ID],
		repoName:            "test-repo",
		repoURL:             "https://github.com/test/repo",
		nodeLookup:          nodeLookup,
		slugLookup:          slugLookup,
		imports:             make(map[string][]string),
		importedBy:          make(map[string][]string),
		calls:               make(map[string][]string),
		calledBy:            make(map[string][]string),
		containsFile:        make(map[string][]string),
		definesFunc:         make(map[string][]string),
		declaresClass:       make(map[string][]string),
		definesType:         make(map[string][]string),
		childDir:            make(map[string][]string),
		extendsRel:          make(map[string][]string),
		belongsToDomain:     make(map[string]string),
		belongsToSubdomain:  make(map[string]string),
		partOfDomain:        make(map[string]string),
		domainFiles:         make(map[string][]string),
		subdomainFiles:      make(map[string][]string),
		fileOfFunc:          make(map[string]string),
		fileOfClass:         make(map[string]string),
		fileOfType:          make(map[string]string),
		domainNodeByName:    make(map[string]string),
		subdomainNodeByName: make(map[string]string),
		domainSubdomains:    make(map[string][]string),
		subdomainFuncs:      make(map[string][]string),
		subdomainClasses:    make(map[string][]string),
		domainRelatesOut:    map[string][]domainEdge{center.ID: outEdges},
		domainRelatesIn:     map[string][]domainEdge{center.ID: inEdges},
	}
}

// --- Tests ---

func TestDomainRelationshipIndexing(t *testing.T) {
	// Two domains connected by DOMAIN_RELATES, plus a non-domain node that
	// should NOT be indexed.
	domA := makeDomainNode("d1", "Auth")
	domB := makeDomainNode("d2", "Payments")
	file := makeFileNode("f1", "app.go", "src/app.go")

	nodes := []Node{domA, domB, file}
	rels := []Relationship{
		makeRel("r1", "DOMAIN_RELATES", "d1", "d2"),
		makeRel("r2", "dependsOn", "d2", "d1"),
		makeRel("r3", "IMPORTS", "f1", "d1"), // file->domain: NOT domain-to-domain
	}

	nodeLookup := make(map[string]*Node)
	for i := range nodes {
		nodeLookup[nodes[i].ID] = &nodes[i]
	}

	domainRelatesOut := make(map[string][]domainEdge)
	domainRelatesIn := make(map[string][]domainEdge)

	// Simulate the indexing logic from Run()
	for _, rel := range rels {
		switch rel.Type {
		case "IMPORTS", "calls", "CONTAINS_FILE", "DEFINES_FUNCTION",
			"DECLARES_CLASS", "DEFINES", "CHILD_DIRECTORY", "EXTENDS",
			"belongsTo", "partOf":
			// handled by other indexes (not tested here)
		default:
			startNode := nodeLookup[rel.StartNode]
			endNode := nodeLookup[rel.EndNode]
			if startNode != nil && endNode != nil && hasLabel(startNode, "Domain") && hasLabel(endNode, "Domain") {
				domainRelatesOut[rel.StartNode] = append(domainRelatesOut[rel.StartNode], domainEdge{nodeID: rel.EndNode, relType: rel.Type})
				domainRelatesIn[rel.EndNode] = append(domainRelatesIn[rel.EndNode], domainEdge{nodeID: rel.StartNode, relType: rel.Type})
			}
		}
	}

	// d1 -> d2 via DOMAIN_RELATES
	if got := len(domainRelatesOut["d1"]); got != 1 {
		t.Fatalf("expected 1 outgoing edge from d1, got %d", got)
	}
	if domainRelatesOut["d1"][0].nodeID != "d2" || domainRelatesOut["d1"][0].relType != "DOMAIN_RELATES" {
		t.Fatalf("unexpected edge: %+v", domainRelatesOut["d1"][0])
	}

	// d2 -> d1 via dependsOn
	if got := len(domainRelatesOut["d2"]); got != 1 {
		t.Fatalf("expected 1 outgoing edge from d2, got %d", got)
	}
	if domainRelatesOut["d2"][0].nodeID != "d1" || domainRelatesOut["d2"][0].relType != "dependsOn" {
		t.Fatalf("unexpected edge: %+v", domainRelatesOut["d2"][0])
	}

	// incoming: d2 should have 1 incoming from d1 via DOMAIN_RELATES
	if got := len(domainRelatesIn["d2"]); got != 1 {
		t.Fatalf("expected 1 incoming edge to d2, got %d", got)
	}

	// f1 -> d1 (IMPORTS) should NOT produce a domain edge
	if got := len(domainRelatesOut["f1"]); got != 0 {
		t.Fatalf("expected 0 domain edges from file node, got %d", got)
	}
}

func TestWriteGraphData_Domain_IncludesRelatedDomains(t *testing.T) {
	domA := makeDomainNode("d1", "Auth")
	domB := makeDomainNode("d2", "Payments")

	ctx := domainRenderCtx(domA, []Node{domB}, []domainEdge{
		{nodeID: "d2", relType: "DOMAIN_RELATES"},
	}, nil)

	var sb strings.Builder
	ctx.writeGraphData(&sb)
	output := sb.String()

	if !strings.Contains(output, "graph_data:") {
		t.Fatal("expected graph_data frontmatter")
	}

	// Extract JSON from frontmatter
	gdJSON := extractGraphDataJSON(t, output)
	var gd graphData
	if err := json.Unmarshal([]byte(gdJSON), &gd); err != nil {
		t.Fatalf("failed to parse graph_data JSON: %v", err)
	}

	// Should have the center node (d1) and the related domain (d2)
	if len(gd.Nodes) < 2 {
		t.Fatalf("expected at least 2 nodes, got %d", len(gd.Nodes))
	}

	// Should have a relatesTo edge
	found := false
	for _, e := range gd.Edges {
		if e.Type == "relatesTo" && e.Source == "d1" && e.Target == "d2" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected relatesTo edge from d1 to d2, edges: %+v", gd.Edges)
	}
}

func TestWriteGraphData_Domain_Bidirectional(t *testing.T) {
	domA := makeDomainNode("d1", "Auth")
	domB := makeDomainNode("d2", "Payments")
	domC := makeDomainNode("d3", "Notifications")

	ctx := domainRenderCtx(domA, []Node{domB, domC},
		[]domainEdge{{nodeID: "d2", relType: "DOMAIN_RELATES"}},
		[]domainEdge{{nodeID: "d3", relType: "dependsOn"}},
	)

	var sb strings.Builder
	ctx.writeGraphData(&sb)
	gdJSON := extractGraphDataJSON(t, sb.String())

	var gd graphData
	if err := json.Unmarshal([]byte(gdJSON), &gd); err != nil {
		t.Fatalf("failed to parse graph_data JSON: %v", err)
	}

	// Outgoing: d1 -> d2
	foundOut := false
	// Incoming: d3 -> d1 (reverse=true means edge goes from target to source)
	foundIn := false
	for _, e := range gd.Edges {
		if e.Type == "relatesTo" && e.Source == "d1" && e.Target == "d2" {
			foundOut = true
		}
		if e.Type == "relatesTo" && e.Source == "d3" && e.Target == "d1" {
			foundIn = true
		}
	}
	if !foundOut {
		t.Errorf("missing outgoing relatesTo edge d1->d2, edges: %+v", gd.Edges)
	}
	if !foundIn {
		t.Errorf("missing incoming relatesTo edge d3->d1, edges: %+v", gd.Edges)
	}
}

func TestWriteMermaidDiagram_Domain_ShowsRelatedDomains(t *testing.T) {
	domA := makeDomainNode("d1", "Auth")
	domB := makeDomainNode("d2", "Payments")
	domC := makeDomainNode("d3", "Notifications")

	ctx := domainRenderCtx(domA, []Node{domB, domC},
		[]domainEdge{{nodeID: "d2", relType: "DOMAIN_RELATES"}},
		[]domainEdge{{nodeID: "d3", relType: "dependsOn"}},
	)

	var sb strings.Builder
	ctx.writeMermaidDiagram(&sb)
	output := sb.String()

	if !strings.Contains(output, "mermaid_diagram:") {
		t.Fatal("expected mermaid_diagram frontmatter")
	}

	// Outgoing arrow: center -->|DOMAIN_RELATES| Payments
	if !strings.Contains(output, "DOMAIN_RELATES") {
		t.Errorf("expected DOMAIN_RELATES label in mermaid output:\n%s", output)
	}

	// Incoming arrow: Notifications -->|dependsOn| center
	if !strings.Contains(output, "dependsOn") {
		t.Errorf("expected dependsOn label in mermaid output:\n%s", output)
	}

	// Both domain names should appear
	if !strings.Contains(output, "Payments") {
		t.Errorf("expected Payments in mermaid output:\n%s", output)
	}
	if !strings.Contains(output, "Notifications") {
		t.Errorf("expected Notifications in mermaid output:\n%s", output)
	}
}

func TestWriteDomainBody_RelatedDomainsSection(t *testing.T) {
	domA := makeDomainNode("d1", "Auth")
	domB := makeDomainNode("d2", "Payments")
	domC := makeDomainNode("d3", "Notifications")

	ctx := domainRenderCtx(domA, []Node{domB, domC},
		[]domainEdge{{nodeID: "d2", relType: "DOMAIN_RELATES"}},
		[]domainEdge{{nodeID: "d3", relType: "dependsOn"}},
	)

	var sb strings.Builder
	ctx.writeDomainBody(&sb)
	output := sb.String()

	if !strings.Contains(output, "## Related Domains") {
		t.Fatalf("expected '## Related Domains' section, got:\n%s", output)
	}

	// Outgoing: should have link to Payments with relType
	if !strings.Contains(output, "DOMAIN_RELATES") {
		t.Errorf("expected DOMAIN_RELATES in body:\n%s", output)
	}

	// Incoming: should have link to Notifications with "(incoming)"
	if !strings.Contains(output, "dependsOn") || !strings.Contains(output, "(incoming)") {
		t.Errorf("expected incoming dependsOn in body:\n%s", output)
	}
}

func TestWriteDomainBody_NoRelatedDomains_NoSection(t *testing.T) {
	domA := makeDomainNode("d1", "Auth")

	ctx := domainRenderCtx(domA, nil, nil, nil)

	var sb strings.Builder
	ctx.writeDomainBody(&sb)
	output := sb.String()

	if strings.Contains(output, "## Related Domains") {
		t.Fatalf("should NOT have Related Domains section when there are none, got:\n%s", output)
	}
}

func TestEndToEnd_DomainRelationships(t *testing.T) {
	// Build a minimal SupermodelIR JSON with two domains and a relationship.
	graph := Graph{
		Nodes: []Node{
			makeDomainNode("d1", "Auth"),
			makeDomainNode("d2", "Payments"),
		},
		Relationships: []Relationship{
			makeRel("r1", "processes_requests_from", "d1", "d2"),
		},
	}
	apiResp := APIResponse{
		Status: "completed",
		Result: &GraphResult{
			Graph: graph,
		},
	}

	data, err := json.Marshal(apiResp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "graph.json")
	if err := os.WriteFile(inputPath, data, 0644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	outputDir := filepath.Join(tmpDir, "output")
	if err := Run(inputPath, outputDir, "test-repo", "https://github.com/test/repo"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Read the Auth domain output file (slug is "domain-<name>")
	authSlug := toSlug("domain-Auth")
	authPath := filepath.Join(outputDir, authSlug+".md")
	authData, err := os.ReadFile(authPath)
	if err != nil {
		t.Fatalf("read auth markdown: %v", err)
	}
	authMD := string(authData)

	// Should contain Related Domains section
	if !strings.Contains(authMD, "## Related Domains") {
		t.Errorf("Auth domain should have Related Domains section:\n%s", authMD)
	}

	// Should contain reference to Payments with the relationship type
	if !strings.Contains(authMD, "processes_requests_from") {
		t.Errorf("Auth domain should reference processes_requests_from:\n%s", authMD)
	}

	// graph_data should contain relatesTo edge
	if !strings.Contains(authMD, "relatesTo") {
		t.Errorf("Auth domain graph_data should contain relatesTo edge:\n%s", authMD)
	}

	// Read the Payments domain output file (should have incoming)
	paymentsSlug := toSlug("domain-Payments")
	paymentsPath := filepath.Join(outputDir, paymentsSlug+".md")
	paymentsData, err := os.ReadFile(paymentsPath)
	if err != nil {
		t.Fatalf("read payments markdown: %v", err)
	}
	paymentsMD := string(paymentsData)

	if !strings.Contains(paymentsMD, "## Related Domains") {
		t.Errorf("Payments domain should have Related Domains section:\n%s", paymentsMD)
	}
	if !strings.Contains(paymentsMD, "(incoming)") {
		t.Errorf("Payments domain should show incoming relationship:\n%s", paymentsMD)
	}
}

// --- test utilities ---

// extractGraphDataJSON extracts the JSON string from a graph_data: "..." frontmatter line.
func extractGraphDataJSON(t *testing.T, frontmatter string) string {
	t.Helper()
	const prefix = `graph_data: "`
	idx := strings.Index(frontmatter, prefix)
	if idx == -1 {
		t.Fatalf("graph_data not found in:\n%s", frontmatter)
	}
	start := idx + len(prefix)
	// Find the closing quote — the value is a Go %q-escaped string
	rest := frontmatter[start:]
	// Unescape the Go-quoted string
	full := `"` + rest[:strings.Index(rest, "\"\n")] + `"`
	var unquoted string
	if err := json.Unmarshal([]byte(full), &unquoted); err != nil {
		t.Fatalf("failed to unquote graph_data: %v\nraw: %s", err, full)
	}
	return unquoted
}
