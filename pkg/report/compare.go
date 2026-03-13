package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	markdown "github.com/MichaelMure/go-term-markdown"
	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
)

type datasetMetadata struct {
	Fingerprint struct {
		Hash string `json:"hash"`
	} `json:"fingerprint"`
}

// Compare compares result and render plot.
func Compare(dirs ...string) error {
	if err := validateDatasetFingerprints(dirs...); err != nil {
		return err
	}

	// summaries for API
	apiSummaries := make(map[string][]*Summary)
	// summaries for pull/push
	pullSummaries := make(map[string][]*Summary)
	// aggregate
	aggregate := func(path string) error {
		files, err := filepath.Glob(fmt.Sprintf("%s/*.summary.json", path))
		if err != nil {
			return err
		}

		for _, f := range files {
			data, err := os.ReadFile(f)
			if err != nil {
				return err
			}

			var s Summary
			if err = json.Unmarshal(data, &s); err != nil {
				return err
			}

			if strings.Contains(f, "push-artifacts") || strings.Contains(f, "pull-artifacts") {
				pullSummaries[s.Description] = append(pullSummaries[s.Description], &s)
			} else {
				apiSummaries[s.Description] = append(apiSummaries[s.Description], &s)
			}
		}

		return nil
	}

	for _, dir := range dirs {
		if err := aggregate(dir); err != nil {
			return err
		}
	}

	// print comparison tables to terminal
	printComparisonTables(apiSummaries, dirs...)
	printComparisonTables(pullSummaries, dirs...)

	apiBar, err := createBar(apiSummaries, dirs...)
	if err != nil {
		return err
	}

	if err = renderChart(apiBar, filepath.Join(outputDir(), "api-comparison.html"), "API", apiSummaries, dirs...); err != nil {
		return err
	}

	pullBar, err := createBar(pullSummaries, dirs...)
	if err != nil {
		return err
	}

	if err = renderChart(pullBar, filepath.Join(outputDir(), "pull-push-comparison.html"), "PULL/PUSH", pullSummaries, dirs...); err != nil {
		return err
	}

	return nil
}

func validateDatasetFingerprints(paths ...string) error {
	fingerprints := make([]string, 0, len(paths))

	for _, path := range paths {
		data, err := os.ReadFile(filepath.Join(path, "dataset.json"))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}

		var metadata datasetMetadata
		if err := json.Unmarshal(data, &metadata); err != nil {
			return fmt.Errorf("parse dataset metadata for %s: %w", path, err)
		}
		if metadata.Fingerprint.Hash == "" {
			return fmt.Errorf("dataset fingerprint missing for %s", path)
		}
		fingerprints = append(fingerprints, metadata.Fingerprint.Hash)
	}

	if len(fingerprints) == 0 {
		return nil
	}
	if len(fingerprints) != len(paths) {
		return fmt.Errorf("cannot compare runs with incomplete dataset metadata")
	}

	expected := fingerprints[0]
	for i := 1; i < len(fingerprints); i++ {
		if fingerprints[i] != expected {
			return fmt.Errorf("dataset fingerprint mismatch: %s != %s", expected, fingerprints[i])
		}
	}

	return nil
}

func sortedKeys(m map[string][]*Summary) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// parseNumericValue parses a human-readable metric string to a float.
// Handles suffixes: "ms", "s", "/s", "%".
func parseNumericValue(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "/s") {
		return strconv.ParseFloat(strings.TrimSuffix(s, "/s"), 64)
	}
	if strings.HasSuffix(s, "%") {
		return strconv.ParseFloat(strings.TrimSuffix(s, "%"), 64)
	}
	if strings.HasSuffix(s, "ms") {
		return strconv.ParseFloat(strings.TrimSuffix(s, "ms"), 64)
	}
	if strings.HasSuffix(s, "s") {
		v, err := strconv.ParseFloat(strings.TrimSuffix(s, "s"), 64)
		if err != nil {
			return 0, err
		}
		return v * 1000, nil
	}
	return strconv.ParseFloat(s, 64)
}

// percentChange calculates the percentage change from a to b.
func percentChange(a, b string) string {
	aVal, errA := parseNumericValue(a)
	bVal, errB := parseNumericValue(b)
	if errA != nil || errB != nil || aVal == 0 {
		return "N/A"
	}
	pct := ((bVal - aVal) / math.Abs(aVal)) * 100
	sign := "+"
	if pct < 0 {
		sign = ""
	}
	return fmt.Sprintf("%s%.1f%%", sign, pct)
}

func printComparisonTables(summaries map[string][]*Summary, dirs ...string) {
	apis := sortedKeys(summaries)

	if len(dirs) < 2 {
		return
	}

	// print table for each run
	for i, dir := range dirs {
		var md strings.Builder
		md.WriteString(fmt.Sprintf("\n### %s\n\n", dir))
		md.WriteString("| API | Avg | Min | Med | Max | P(90) | P(95) | Success Rate | Iterations Rate |\n")
		md.WriteString("|-----|-----|-----|-----|-----|-------|-------|--------------|------------------|\n")

		for _, api := range apis {
			if i >= len(summaries[api]) {
				continue
			}
			s := summaries[api][i]
			md.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | %s | %s | %s | %s |\n",
				s.Description, s.Avg, s.Min, s.Med, s.Max, s.P90, s.P95, s.SuccessRate, s.IterationsRate))
		}

		fmt.Println(string(markdown.Render(md.String(), 160, 6)))
	}

	// print comparison table with % change
	var md strings.Builder
	md.WriteString(fmt.Sprintf("\n### Comparison: %s vs %s\n\n", dirs[0], dirs[1]))
	md.WriteString("| API | Avg | Min | Med | Max | P(90) | P(95) | Iterations Rate |\n")
	md.WriteString("|-----|-----|-----|-----|-----|-------|-------|-----------------|\n")

	for _, api := range apis {
		if len(summaries[api]) < 2 {
			continue
		}
		a := summaries[api][0]
		b := summaries[api][1]
		md.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | %s | %s | %s |\n",
			a.Description,
			percentChange(a.Avg, b.Avg),
			percentChange(a.Min, b.Min),
			percentChange(a.Med, b.Med),
			percentChange(a.Max, b.Max),
			percentChange(a.P90, b.P90),
			percentChange(a.P95, b.P95),
			percentChange(a.IterationsRate, b.IterationsRate),
		))
	}

	fmt.Println(string(markdown.Render(md.String(), 160, 6)))
}

func createBar(summaries map[string][]*Summary, dirs ...string) (*charts.Bar, error) {
	bar := charts.NewBar()
	xAxis := sortedKeys(summaries)

	bar.SetXAxis(xAxis)

	data := make([][]opts.BarData, len(dirs))
	for _, x := range xAxis {
		for i := range dirs {
			// parseNumericValue returns milliseconds; convert to seconds for chart
			ms, err := parseNumericValue(summaries[x][i].P95)
			if err != nil {
				return nil, err
			}
			p95 := math.Round(ms/10) / 100 // ms to seconds, 2 decimal places

			data[i] = append(data[i], opts.BarData{Value: p95})
		}
	}

	for i := range dirs {
		bar.AddSeries(dirs[i], data[i]).SetSeriesOptions(
			charts.WithLabelOpts(opts.Label{
				Show:     true,
				Position: "top",
			}),
		)
	}

	return bar, nil
}

func renderChart(bar *charts.Bar, filename, xAxisName string, summaries map[string][]*Summary, dirs ...string) error {
	bar.SetGlobalOptions(
		charts.WithTitleOpts(opts.Title{
			Title:    fmt.Sprintf("Harbor %s Performance", xAxisName),
			Subtitle: strings.Join(dirs, " → ") + "  ·  P95 latency (lower is better)",
			Right:    "45%",
			TitleStyle: &opts.TextStyle{
				Color:      "#e2e8f0",
				FontFamily: "DM Sans, -apple-system, sans-serif",
			},
			SubtitleStyle: &opts.TextStyle{
				Color:      "#64748b",
				FontFamily: "JetBrains Mono, monospace",
				FontSize:   11,
			},
		}),
		charts.WithXAxisOpts(opts.XAxis{
			Name:      xAxisName,
			AxisLabel: &opts.AxisLabel{Interval: "0", ShowMinLabel: true, ShowMaxLabel: true, FontSize: "12", Color: "#94a3b8", Rotate: 90},
			SplitLine: &opts.SplitLine{Show: false},
		}),
		charts.WithYAxisOpts(opts.YAxis{
			Name:      "P95 (seconds)",
			AxisLabel: &opts.AxisLabel{Color: "#64748b"},
			SplitLine: &opts.SplitLine{Show: true, LineStyle: &opts.LineStyle{Color: "#1a1f2e"}},
		}),
		charts.WithInitializationOpts(opts.Initialization{
			Width:  "1600px",
			Height: "800px",
		}),
		charts.WithLegendOpts(opts.Legend{
			Right: "80%", Data: dirs, Show: true,
			TextStyle: &opts.TextStyle{Color: "#94a3b8"},
		}),
		charts.WithToolboxOpts(opts.Toolbox{Show: true}),
	)

	// render chart to buffer so we can inject tables before </body>
	var buf bytes.Buffer
	if err := bar.Render(&buf); err != nil {
		return err
	}

	html := buf.String()
	if len(summaries) > 0 {
		tables := buildHTMLTables(xAxisName, summaries, dirs...)
		// inject before </body> so CSS applies correctly
		html = strings.Replace(html, "</body>", tables+"</body>", 1)
	}

	if err := os.WriteFile(filename, []byte(html), 0644); err != nil {
		return err
	}

	fmt.Printf("chart was rendered in: %s\n", filename)
	return nil
}

const tableStyle = `
<link href="https://fonts.googleapis.com/css2?family=JetBrains+Mono:wght@400;500;600;700&family=DM+Sans:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>
  /* ── Color Scheme ── */
  :root {
    color-scheme: light dark;

    /* OKLCH primitives */
    --lch-white: 100% 0 0;
    --lch-black: 0% 0 0;
    --lch-gray-50: 98% 0.003 247;
    --lch-gray-100: 96% 0.007 247;
    --lch-gray-200: 92% 0.013 255;
    --lch-gray-300: 87% 0.022 252;
    --lch-gray-400: 70% 0.04 256;
    --lch-gray-500: 55% 0.046 257;
    --lch-gray-600: 45% 0.043 257;
    --lch-gray-700: 37% 0.044 257;
    --lch-gray-800: 28% 0.041 260;
    --lch-gray-900: 21% 0.042 265;
    --lch-gray-950: 13% 0.042 264;
    --lch-cyan-400: 79% 0.13 210;
    --lch-cyan-500: 72% 0.13 215;
    --lch-amber-400: 83% 0.189 84;
    --lch-amber-500: 77% 0.188 70;
    --lch-green-400: 79% 0.209 151;
    --lch-green-500: 72% 0.219 149;
    --lch-green-600: 63% 0.194 149;
    --lch-green-700: 53% 0.154 150;
    --lch-red-400: 70% 0.191 22;
    --lch-red-500: 64% 0.237 25;
    --lch-red-600: 58% 0.245 27;
    --lch-red-700: 50% 0.213 27;

    /* Semantic tokens via light-dark() */
    --color-canvas: light-dark(oklch(var(--lch-gray-50)), oklch(var(--lch-gray-950)));
    --color-surface: light-dark(oklch(var(--lch-white)), oklch(var(--lch-gray-900)));
    --color-surface-raised: light-dark(oklch(var(--lch-white)), oklch(var(--lch-gray-800)));
    --color-text: light-dark(oklch(var(--lch-gray-900)), oklch(var(--lch-gray-100)));
    --color-text-secondary: light-dark(oklch(var(--lch-gray-600)), oklch(var(--lch-gray-400)));
    --color-text-muted: light-dark(oklch(var(--lch-gray-500)), oklch(var(--lch-gray-500)));
    --color-border: light-dark(oklch(var(--lch-gray-200)), oklch(var(--lch-gray-800)));
    --color-border-subtle: light-dark(oklch(var(--lch-gray-100)), oklch(var(--lch-gray-800) / 0.6));
    --color-accent: light-dark(oklch(var(--lch-cyan-500)), oklch(var(--lch-cyan-400)));
    --color-accent-subtle: light-dark(oklch(var(--lch-cyan-400) / 0.1), oklch(var(--lch-cyan-400) / 0.1));
    --color-accent-border: light-dark(oklch(var(--lch-cyan-400) / 0.25), oklch(var(--lch-cyan-400) / 0.2));
    --color-warn: light-dark(oklch(var(--lch-amber-500)), oklch(var(--lch-amber-400)));
    --color-warn-subtle: light-dark(oklch(var(--lch-amber-400) / 0.1), oklch(var(--lch-amber-400) / 0.1));
    --color-warn-border: light-dark(oklch(var(--lch-amber-400) / 0.25), oklch(var(--lch-amber-400) / 0.2));
    --color-positive: light-dark(oklch(var(--lch-green-600)), oklch(var(--lch-green-400)));
    --color-negative: light-dark(oklch(var(--lch-red-600)), oklch(var(--lch-red-400)));
    --color-positive-dot: light-dark(oklch(var(--lch-green-500)), oklch(var(--lch-green-400)));
    --color-negative-dot: light-dark(oklch(var(--lch-red-500)), oklch(var(--lch-red-400)));
    --color-hover: light-dark(oklch(var(--lch-cyan-400) / 0.04), oklch(var(--lch-cyan-400) / 0.03));
    --color-thead: light-dark(oklch(var(--lch-gray-50)), oklch(var(--lch-gray-900) / 0.8));
  }
  html[data-theme="light"] { color-scheme: light; }
  html[data-theme="dark"]  { color-scheme: dark; }

  /* ── Base ── */
  body {
    background: var(--color-canvas);
    color: var(--color-text);
    margin: 0;
    padding: 0;
    transition: background-color 0.2s ease, color 0.2s ease;
  }
  body > div:first-child {
    background: var(--color-canvas);
    width: 1600px;
    margin: 0 auto;
    padding: 20px 0 0;
  }

  /* ── Theme Toggle ── */
  .theme-toggle {
    position: fixed;
    top: 16px;
    right: 16px;
    z-index: 100;
    display: flex;
    align-items: center;
    gap: 6px;
    background: var(--color-surface);
    border: 1px solid var(--color-border);
    border-radius: 8px;
    padding: 8px 12px;
    cursor: pointer;
    font-family: 'JetBrains Mono', monospace;
    font-size: 11px;
    color: var(--color-text-secondary);
    transition: background 0.15s ease, border-color 0.15s ease;
    box-shadow: 0 2px 8px oklch(var(--lch-black) / 0.1);
  }
  .theme-toggle:hover {
    background: var(--color-surface-raised);
    border-color: var(--color-accent);
    color: var(--color-text);
  }
  .theme-toggle svg {
    width: 16px;
    height: 16px;
    fill: none;
    stroke: currentColor;
    stroke-width: 2;
    stroke-linecap: round;
    stroke-linejoin: round;
  }
  .theme-toggle__sun  { display: block; }
  .theme-toggle__moon { display: none; }
  html[data-theme="dark"] .theme-toggle__sun  { display: none; }
  html[data-theme="dark"] .theme-toggle__moon { display: block; }
  @media (prefers-color-scheme: dark) {
    html:not([data-theme="light"]) .theme-toggle__sun  { display: none; }
    html:not([data-theme="light"]) .theme-toggle__moon { display: block; }
  }

  /* ── Dashboard ── */
  .dash {
    font-family: 'DM Sans', -apple-system, BlinkMacSystemFont, sans-serif;
    background: var(--color-canvas);
    color: var(--color-text);
    width: 1600px;
    padding: 0 0 60px;
    margin: 0 auto;
  }
  .dash .section {
    margin-top: 48px;
  }
  .dash .section-header {
    display: flex;
    align-items: center;
    gap: 14px;
    margin-bottom: 20px;
    padding-bottom: 12px;
    border-bottom: 1px solid var(--color-accent-border);
  }
  .dash .section-header .badge {
    font-family: 'JetBrains Mono', monospace;
    font-size: 10px;
    font-weight: 600;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    padding: 4px 10px;
    border-radius: 4px;
    background: var(--color-accent-subtle);
    color: var(--color-accent);
    border: 1px solid var(--color-accent-border);
  }
  .dash .section-header h2 {
    font-size: 18px;
    font-weight: 600;
    color: var(--color-text);
    margin: 0;
    letter-spacing: -0.01em;
  }
  .dash .section-header.comparison .badge {
    background: var(--color-warn-subtle);
    color: var(--color-warn);
    border-color: var(--color-warn-border);
  }

  /* ── Tables ── */
  .dash table {
    width: 100%;
    border-collapse: separate;
    border-spacing: 0;
    font-size: 13px;
    background: var(--color-surface);
    border: 1px solid var(--color-border);
    border-radius: 8px;
    overflow: hidden;
    margin-bottom: 8px;
  }
  .dash thead th {
    font-family: 'JetBrains Mono', monospace;
    font-size: 10px;
    font-weight: 600;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--color-text-muted);
    padding: 14px 16px;
    text-align: right;
    background: var(--color-thead);
    border-bottom: 1px solid var(--color-border);
    position: sticky;
    top: 0;
    z-index: 1;
  }
  .dash thead th:first-child {
    text-align: left;
    color: var(--color-text-secondary);
  }
  .dash tbody td {
    font-family: 'JetBrains Mono', monospace;
    font-size: 12.5px;
    font-weight: 400;
    padding: 11px 16px;
    text-align: right;
    border-bottom: 1px solid var(--color-border-subtle);
    color: var(--color-text-secondary);
    transition: background 0.15s ease;
  }
  .dash tbody td:first-child {
    text-align: left;
    font-family: 'DM Sans', sans-serif;
    font-weight: 500;
    font-size: 13px;
    color: var(--color-text);
    white-space: nowrap;
  }
  .dash tbody tr:hover td {
    background: var(--color-hover);
  }
  .dash tbody tr:last-child td {
    border-bottom: none;
  }

  /* ── Status Colors ── */
  .dash .rate-ok { color: var(--color-positive); }
  .dash .improved {
    color: var(--color-positive);
    font-weight: 500;
  }
  .dash .improved::before {
    content: '▼ ';
    font-size: 8px;
    vertical-align: 1px;
  }
  .dash .regressed {
    color: var(--color-negative);
    font-weight: 500;
  }
  .dash .regressed::before {
    content: '▲ ';
    font-size: 8px;
    vertical-align: 1px;
  }
  .dash .neutral { color: var(--color-text-muted); }
  .dash .improved-up {
    color: var(--color-positive);
    font-weight: 500;
  }
  .dash .improved-up::before {
    content: '▲ ';
    font-size: 8px;
    vertical-align: 1px;
  }
  .dash .regressed-down {
    color: var(--color-negative);
    font-weight: 500;
  }
  .dash .regressed-down::before {
    content: '▼ ';
    font-size: 8px;
    vertical-align: 1px;
  }

  /* ── Summary Bar ── */
  .dash .summary-bar {
    display: flex;
    gap: 24px;
    margin-top: 16px;
    padding: 14px 20px;
    background: var(--color-surface);
    border: 1px solid var(--color-border);
    border-radius: 8px;
    font-family: 'JetBrains Mono', monospace;
    font-size: 12px;
  }
  .dash .summary-bar .stat {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .dash .summary-bar .stat-label {
    color: var(--color-text-muted);
    text-transform: uppercase;
    font-size: 10px;
    letter-spacing: 0.06em;
  }
  .dash .summary-bar .stat-value { font-weight: 600; }
  .dash .summary-bar .stat-value.green { color: var(--color-positive); }
  .dash .summary-bar .stat-value.red { color: var(--color-negative); }
  .dash .summary-bar .stat-value.neutral { color: var(--color-text-secondary); }

  /* ── Legend ── */
  .dash .legend {
    display: flex;
    gap: 24px;
    margin-bottom: 16px;
    font-family: 'JetBrains Mono', monospace;
    font-size: 11px;
  }
  .dash .legend-item {
    display: flex;
    align-items: center;
    gap: 6px;
  }
  .dash .legend-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
  }
  .dash .legend-dot.green { background: var(--color-positive-dot); }
  .dash .legend-dot.red { background: var(--color-negative-dot); }
  .dash .legend-text { color: var(--color-text-secondary); }

  /* ── Page Title ── */
  .dash .page-title {
    font-family: 'DM Sans', -apple-system, BlinkMacSystemFont, sans-serif;
    font-size: 28px;
    font-weight: 700;
    letter-spacing: -0.02em;
    color: var(--color-text);
    margin: 32px 0 4px;
    padding: 0;
  }
  .dash .page-subtitle {
    font-family: 'JetBrains Mono', monospace;
    font-size: 13px;
    color: var(--color-text-muted);
    margin: 0 0 8px;
  }
</style>
`

func buildHTMLTables(testType string, summaries map[string][]*Summary, dirs ...string) string {
	apis := sortedKeys(summaries)

	var sb strings.Builder
	sb.WriteString(tableStyle)

	// theme toggle button + JS
	sb.WriteString(`<button class="theme-toggle" onclick="toggleTheme()" aria-label="Toggle theme">`)
	sb.WriteString(`<svg class="theme-toggle__sun" viewBox="0 0 24 24"><circle cx="12" cy="12" r="5"/><line x1="12" y1="1" x2="12" y2="3"/><line x1="12" y1="21" x2="12" y2="23"/><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"/><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/><line x1="1" y1="12" x2="3" y2="12"/><line x1="21" y1="12" x2="23" y2="12"/><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/></svg>`)
	sb.WriteString(`<svg class="theme-toggle__moon" viewBox="0 0 24 24"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></svg>`)
	sb.WriteString(`<span>Theme</span></button>`)
	sb.WriteString(`
<script>
(function() {
  var saved = localStorage.getItem('perf-theme');
  if (saved) document.documentElement.setAttribute('data-theme', saved);

  // update echarts colors to match theme
  function applyChartTheme() {
    var isDark = saved === 'dark' || (!saved && window.matchMedia('(prefers-color-scheme: dark)').matches);
    var textColor = isDark ? '#94a3b8' : '#475569';
    var splitColor = isDark ? '#1a1f2e' : '#e2e8f0';
    var titleColor = isDark ? '#e2e8f0' : '#1e293b';
    var subColor = isDark ? '#64748b' : '#64748b';
    // find all echarts instances on the page
    var divs = document.querySelectorAll('[_echarts_instance_]');
    divs.forEach(function(el) {
      var chart = echarts.getInstanceByDom(el);
      if (!chart) return;
      chart.setOption({
        backgroundColor: 'transparent',
        title: { textStyle: { color: titleColor }, subtitleStyle: { color: subColor } },
        xAxis: { axisLabel: { color: textColor }, nameTextStyle: { color: subColor } },
        yAxis: { axisLabel: { color: textColor }, nameTextStyle: { color: subColor }, splitLine: { lineStyle: { color: splitColor } } },
        legend: { textStyle: { color: textColor } }
      });
    });
  }
  // run once on load
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', applyChartTheme);
  } else {
    setTimeout(applyChartTheme, 100);
  }

  window.toggleTheme = function() {
    var current = document.documentElement.getAttribute('data-theme');
    var isDarkNow = current === 'dark' || (!current && window.matchMedia('(prefers-color-scheme: dark)').matches);
    saved = isDarkNow ? 'light' : 'dark';
    document.documentElement.setAttribute('data-theme', saved);
    localStorage.setItem('perf-theme', saved);
    applyChartTheme();
  };
})();
</script>
`)

	sb.WriteString(`<div class="dash">`)
	sb.WriteString(fmt.Sprintf(`<h1 class="page-title">Harbor %s Performance Tests</h1>`, testType))
	sb.WriteString(fmt.Sprintf(`<p class="page-subtitle">%s</p>`, strings.Join(dirs, " → ")))

	// table for each run
	for i, dir := range dirs {
		sb.WriteString(`<div class="section">`)
		sb.WriteString(`<div class="section-header">`)
		sb.WriteString(fmt.Sprintf(`<span class="badge">Run %d</span>`, i+1))
		sb.WriteString(fmt.Sprintf(`<h2>%s</h2>`, dir))
		sb.WriteString("</div>\n<table>\n<thead>\n")
		sb.WriteString("<tr><th>Endpoint</th><th>Avg</th><th>Min</th><th>Med</th><th>Max</th><th>P90</th><th>P95</th><th>Success</th><th>Iter/s</th></tr>\n</thead>\n<tbody>\n")
		for _, api := range apis {
			if i >= len(summaries[api]) {
				continue
			}
			s := summaries[api][i]
			rateClass := ""
			if s.SuccessRate == "100%" {
				rateClass = ` class="rate-ok"`
			}
			sb.WriteString(fmt.Sprintf("<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td%s>%s</td><td>%s</td></tr>\n",
				s.Description, s.Avg, s.Min, s.Med, s.Max, s.P90, s.P95, rateClass, s.SuccessRate, s.IterationsRate))
		}
		sb.WriteString("</tbody>\n</table>\n</div>\n")
	}

	// comparison table with % change and summary
	if len(dirs) >= 2 {
		sb.WriteString(`<div class="section">`)
		sb.WriteString(`<div class="section-header comparison">`)
		sb.WriteString(`<span class="badge">Delta</span>`)
		sb.WriteString(fmt.Sprintf(`<h2>%s → %s</h2>`, dirs[0], dirs[1]))
		sb.WriteString("</div>\n")
		sb.WriteString(`<div class="legend">`)
		sb.WriteString(fmt.Sprintf(`<div class="legend-item"><span class="legend-dot green"></span><span class="legend-text">Faster in %s (latency ↓) or Higher throughput (iter/s ↑)</span></div>`, dirs[1]))
		sb.WriteString(fmt.Sprintf(`<div class="legend-item"><span class="legend-dot red"></span><span class="legend-text">Slower in %s (latency ↑) or Lower throughput (iter/s ↓)</span></div>`, dirs[1]))
		sb.WriteString("</div>\n<table>\n<thead>\n")
		sb.WriteString("<tr><th>Endpoint</th><th>Avg</th><th>Min</th><th>Med</th><th>Max</th><th>P90</th><th>P95</th><th>Iter/s</th></tr>\n</thead>\n<tbody>\n")

		improved := 0
		regressed := 0
		unchanged := 0
		for _, api := range apis {
			if len(summaries[api]) < 2 {
				continue
			}
			a := summaries[api][0]
			b := summaries[api][1]
			sb.WriteString(fmt.Sprintf("<tr><td>%s</td>%s%s%s%s%s%s%s</tr>\n",
				a.Description,
				percentChangeHTML(a.Avg, b.Avg, true),
				percentChangeHTML(a.Min, b.Min, true),
				percentChangeHTML(a.Med, b.Med, true),
				percentChangeHTML(a.Max, b.Max, true),
				percentChangeHTML(a.P90, b.P90, true),
				percentChangeHTML(a.P95, b.P95, true),
				percentChangeHTML(a.IterationsRate, b.IterationsRate, false),
			))
			// count for summary using P95
			p95Change := percentChange(a.P95, b.P95)
			if strings.HasPrefix(p95Change, "-") {
				improved++
			} else if strings.HasPrefix(p95Change, "+") {
				regressed++
			} else {
				unchanged++
			}
		}
		sb.WriteString("</tbody>\n</table>\n")

		// summary bar
		sb.WriteString(`<div class="summary-bar">`)
		sb.WriteString(fmt.Sprintf(`<div class="stat"><span class="stat-label">Improved (P95)</span><span class="stat-value green">%d</span></div>`, improved))
		sb.WriteString(fmt.Sprintf(`<div class="stat"><span class="stat-label">Regressed (P95)</span><span class="stat-value red">%d</span></div>`, regressed))
		sb.WriteString(fmt.Sprintf(`<div class="stat"><span class="stat-label">Unchanged</span><span class="stat-value neutral">%d</span></div>`, unchanged))
		sb.WriteString(fmt.Sprintf(`<div class="stat"><span class="stat-label">Total endpoints</span><span class="stat-value neutral">%d</span></div>`, improved+regressed+unchanged))
		sb.WriteString("</div>\n</div>\n")
	}

	sb.WriteString("</div>\n")
	return sb.String()
}

// percentChangeHTML returns a <td> with color coding.
// lowerIsBetter=true means negative change is good (latency metrics).
// lowerIsBetter=false means positive change is good (throughput metrics).
func percentChangeHTML(a, b string, lowerIsBetter bool) string {
	aVal, errA := parseNumericValue(a)
	bVal, errB := parseNumericValue(b)
	if errA != nil || errB != nil || aVal == 0 {
		return `<td class="neutral">N/A</td>`
	}

	diff := bVal - aVal
	pct := (diff / math.Abs(aVal)) * 100
	sign := "+"
	if pct < 0 {
		sign = ""
	}
	pctStr := fmt.Sprintf("%s%.1f%%", sign, pct)

	class := "neutral"
	if lowerIsBetter {
		if diff < 0 {
			class = "improved"
		} else if diff > 0 {
			class = "regressed"
		}
	} else {
		if diff > 0 {
			class = "improved-up"
		} else if diff < 0 {
			class = "regressed-down"
		}
	}

	return fmt.Sprintf(`<td class="%s">%s</td>`, class, pctStr)
}
