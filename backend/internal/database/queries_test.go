package database

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestTenantQueriesReferenceOrganization is the isolation lint: a query on an
// organization-owned table (tenant_tables.txt) must mention organization_id,
// so a forgotten filter cannot leak rows across organizations. Queries that are
// global on purpose (token lookups, a user's own memberships, platform admin
// views) opt out with "-- tenant-lint: cross-org <reason>".
func TestTenantQueriesReferenceOrganization(t *testing.T) {
	tables := readTenantTables(t)
	files, err := filepath.Glob(filepath.Join("queries", "*.sql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no query files: %v", err)
	}
	nameRe := regexp.MustCompile(`^--\s*name:\s*(\w+)`)
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, q := range splitQueries(string(raw), nameRe) {
			if q.exempt {
				if q.reason == "" {
					t.Errorf("%s: %s: tenant-lint exemption needs a reason", file, q.name)
				}
				continue
			}
			body := strings.ToLower(stripComments(q.body))
			for _, table := range tables {
				if !regexp.MustCompile(`\b` + table + `\b`).MatchString(body) {
					continue
				}
				if !strings.Contains(body, "organization_id") {
					t.Errorf("%s: %s touches %s but never references organization_id", file, q.name, table)
				}
			}
		}
	}
}

type namedQuery struct {
	name   string
	body   string
	exempt bool
	reason string
}

func splitQueries(src string, nameRe *regexp.Regexp) []namedQuery {
	var out []namedQuery
	var cur *namedQuery
	var pendingExempt *string
	sc := bufio.NewScanner(strings.NewReader(src))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "-- tenant-lint: cross-org") {
			r := strings.TrimSpace(strings.TrimPrefix(trimmed, "-- tenant-lint: cross-org"))
			pendingExempt = &r
			continue
		}
		if m := nameRe.FindStringSubmatch(trimmed); m != nil {
			if cur != nil {
				out = append(out, *cur)
			}
			cur = &namedQuery{name: m[1]}
			if pendingExempt != nil {
				cur.exempt, cur.reason = true, *pendingExempt
				pendingExempt = nil
			}
			continue
		}
		if cur != nil {
			cur.body += line + "\n"
		}
	}
	if cur != nil {
		out = append(out, *cur)
	}
	return out
}

func stripComments(sql string) string {
	var b strings.Builder
	for _, line := range strings.Split(sql, "\n") {
		if i := strings.Index(line, "--"); i >= 0 {
			line = line[:i]
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

func readTenantTables(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile("tenant_tables.txt")
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	if len(out) == 0 {
		t.Fatal("tenant_tables.txt is empty")
	}
	return out
}
