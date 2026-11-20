// Command testreport turns `go test -json` output into a single self-contained HTML report.
//
//	go test -json ./... > unit.json
//	go test -tags e2e -json ./e2e > e2e.json
//	go run ./cmd/testreport -o docs/report/index.html unit.json e2e.json
package main

import (
	"bufio"
	"cmp"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"
)

type event struct {
	Time    time.Time
	Action  string
	Package string
	Test    string
	Elapsed float64
	Output  string
}

type result struct {
	Suite   string
	Test    string // top-level test
	Case    string // subtest path below it, e.g. "blank/webkit"
	Browser string
	Status  string
	Elapsed float64
	Output  string
}

var browsers = []string{"chromium", "firefox", "webkit", "mobile-chrome"}

func main() {
	out := flag.String("o", "docs/report/index.html", "output HTML file")
	flag.Parse()

	var results []result
	var started, ended time.Time
	for _, f := range flag.Args() {
		rs, s, e := parse(f)
		results = append(results, rs...)
		if started.IsZero() || s.Before(started) {
			started = s
		}
		if e.After(ended) {
			ended = e
		}
	}
	slices.SortStableFunc(results, func(a, b result) int {
		return cmp.Or(cmp.Compare(a.Suite, b.Suite), cmp.Compare(a.Test, b.Test), cmp.Compare(a.Case, b.Case))
	})

	counts := map[string]int{}
	for _, r := range results {
		counts[r.Status]++
	}

	// Cross-browser matrix: one row per e2e scenario, one column per browser.
	type cell struct{ Status string }
	type row struct {
		Name  string
		Cells []cell
	}
	idx := map[string]int{}
	var matrix []row
	for _, r := range results {
		if r.Browser == "" {
			continue
		}
		name := strings.TrimSuffix(r.Test+" "+strings.TrimSuffix(r.Case, r.Browser), " ")
		name = strings.TrimSuffix(strings.ReplaceAll(name, "/", " / "), " / ")
		i, ok := idx[name]
		if !ok {
			i = len(matrix)
			idx[name] = i
			matrix = append(matrix, row{Name: name, Cells: make([]cell, len(browsers))})
		}
		for j, b := range browsers {
			if b == r.Browser {
				matrix[i].Cells[j] = cell{Status: r.Status}
			}
		}
	}

	data := map[string]any{
		"Results": results, "Counts": counts, "Total": len(results), "Matrix": matrix, "Browsers": browsers,
		"Started": started.Local().Format("02 Jan 2006 15:04 MST"), "Duration": ended.Sub(started).Round(time.Second),
		"GoVersion": runtime.Version(), "OS": runtime.GOOS + "/" + runtime.GOARCH,
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		log.Fatal(err)
	}
	f, err := os.Create(*out)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if err := page.Execute(f, data); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s: %d passed, %d failed, %d skipped\n", *out, counts["pass"], counts["fail"], counts["skip"])
	if counts["fail"] > 0 {
		os.Exit(1)
	}
}

// parse reads one go test -json stream and returns leaf test results (tests with no subtests).
func parse(path string) ([]result, time.Time, time.Time) {
	f, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	type key struct{ pkg, test string }
	outputs := map[key]*strings.Builder{}
	final := map[key]event{}
	hasChildren := map[key]bool{}
	var order []key
	var start, end time.Time

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		var e event
		if json.Unmarshal(sc.Bytes(), &e) != nil || e.Test == "" {
			continue
		}
		if start.IsZero() || e.Time.Before(start) {
			start = e.Time
		}
		if e.Time.After(end) {
			end = e.Time
		}
		k := key{e.Package, e.Test}
		if i := strings.LastIndex(e.Test, "/"); i > 0 {
			hasChildren[key{e.Package, e.Test[:i]}] = true
		}
		switch e.Action {
		case "output":
			if outputs[k] == nil {
				outputs[k] = &strings.Builder{}
			}
			outputs[k].WriteString(e.Output)
		case "pass", "fail", "skip":
			if _, seen := final[k]; !seen {
				order = append(order, k)
			}
			final[k] = e
		}
	}
	var rs []result
	for _, k := range order {
		if hasChildren[k] {
			continue
		}
		e := final[k]
		top, sub, _ := strings.Cut(k.test, "/")
		r := result{Suite: suiteName(k.pkg), Test: top, Case: sub, Status: e.Action, Elapsed: e.Elapsed}
		for _, b := range browsers {
			if sub == b || strings.HasSuffix(sub, "/"+b) {
				r.Browser = b
			}
		}
		if e.Action == "fail" && outputs[k] != nil {
			r.Output = outputs[k].String()
		}
		rs = append(rs, r)
	}
	return rs, start, end
}

func suiteName(pkg string) string {
	switch {
	case strings.HasSuffix(pkg, "/e2e"):
		return "Browser (e2e)"
	case strings.HasSuffix(pkg, "/checkout"):
		return "Unit: checkout rules"
	case strings.HasSuffix(pkg, "/web"):
		return "Unit: HTTP API"
	}
	return pkg
}

var page = template.Must(template.New("r").Funcs(template.FuncMap{
	"secs": func(s float64) string { return fmt.Sprintf("%.2fs", s) },
}).Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Test report — Mini Checkout</title>
<style>
body{font:15px/1.5 system-ui,-apple-system,Segoe UI,Roboto,sans-serif;margin:0;background:#f6f7f9;color:#1d2433}
main{max-width:1100px;margin:0 auto;padding:28px 18px 60px}
h1{margin:0 0 4px}h2{margin:32px 0 10px;font-size:1.15rem}
.meta{color:#5b6475;margin:0 0 18px}
.tiles{display:flex;gap:12px;flex-wrap:wrap}
.tile{background:#fff;border:1px solid #dde1e7;border-radius:10px;padding:14px 18px;min-width:140px}
.tile b{display:block;font-size:1.8rem;line-height:1.2}
table{width:100%;border-collapse:collapse;background:#fff;border:1px solid #dde1e7;border-radius:10px;overflow:hidden}
th,td{padding:8px 10px;border-bottom:1px solid #eef0f3;text-align:left;vertical-align:top}
th{background:#f0f2f5;font-weight:600}
.pass{color:#067647;font-weight:600}.fail{color:#b42318;font-weight:700}.skip{color:#8a6d00}
td.c{text-align:center}
pre{white-space:pre-wrap;background:#1d2433;color:#f6f7f9;padding:10px;border-radius:6px;font-size:12px;max-height:320px;overflow:auto}
code{font-size:13px}
</style></head><body><main>
<h1>Test report — Mini Checkout</h1>
<p class="meta">Run {{.Started}} · took {{.Duration}} · {{.GoVersion}} on {{.OS}} · retries: 0</p>
<div class="tiles">
<div class="tile"><b>{{.Total}}</b>tests</div>
<div class="tile"><b class="pass">{{index .Counts "pass"}}</b>passed</div>
<div class="tile"><b class="{{if index .Counts "fail"}}fail{{end}}">{{or (index .Counts "fail") 0}}</b>failed</div>
</div>

<h2>Cross-browser matrix (browser tests)</h2>
<table><thead><tr><th>Scenario</th>{{range .Browsers}}<th class="c">{{.}}</th>{{end}}</tr></thead><tbody>
{{range .Matrix}}<tr><td><code>{{.Name}}</code></td>{{range .Cells}}<td class="c {{.Status}}">{{if eq .Status "pass"}}✔{{else if eq .Status "fail"}}✘{{else}}–{{end}}</td>{{end}}</tr>{{end}}
</tbody></table>

<h2>All tests</h2>
<table><thead><tr><th>Suite</th><th>Test</th><th>Case</th><th>Result</th><th>Time</th></tr></thead><tbody>
{{range .Results}}<tr><td>{{.Suite}}</td><td><code>{{.Test}}</code></td><td><code>{{.Case}}</code></td><td class="{{.Status}}">{{.Status}}</td><td>{{secs .Elapsed}}</td></tr>
{{if .Output}}<tr><td colspan="5"><pre>{{.Output}}</pre></td></tr>{{end}}{{end}}
</tbody></table>
</main></body></html>`))
