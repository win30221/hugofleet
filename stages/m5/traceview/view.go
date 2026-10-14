// Package traceview renders safe OTel records collected by the local operator.
// It does not connect to a Worker API or expose filesystem access through Hub.
package traceview

import (
	"bufio"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"regexp"
	"sort"

	"github.com/win30221/hugofleet/stages/m5/observability"
)

var hex32 = regexp.MustCompile(`^[0-9a-f]{32}$`)
var hex16 = regexp.MustCompile(`^[0-9a-f]{16}$`)
var uuid = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var slug = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
var code = regexp.MustCompile(`^[A-Za-z0-9_]{1,64}$`)

func Read(input io.Reader, limit int) ([]observability.Record, error) {
	if limit < 1 || limit > 20000 {
		return nil, fmt.Errorf("trace record limit must be 1..20000")
	}
	scanner := bufio.NewScanner(io.LimitReader(input, 128<<20))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	items := []observability.Record{}
	for scanner.Scan() {
		var v struct {
			Kind string `json:"kind"`
			observability.Record
		}
		if json.Unmarshal(scanner.Bytes(), &v) != nil || v.Kind != "trace_span" {
			continue
		}
		if !hex32.MatchString(v.TraceID) || !hex16.MatchString(v.SpanID) || (v.ParentID != "" && !hex16.MatchString(v.ParentID)) || v.Start < 0 || v.DurationMicros < 0 {
			continue
		}
		switch v.Service {
		case "hub", "agent", "operator":
		default:
			continue
		}
		switch v.Name {
		case "hub.http", "hub.operation", "hub.command", "agent.command", "operator.reconcile":
		default:
			continue
		}
		attrs := map[string]string{}
		for k, s := range v.Attributes {
			switch k {
			case "request_id", "operation_id", "app_id", "command_id", "channel_id":
				if uuid.MatchString(s) {
					attrs[k] = s
				}
			case "tenant_id", "cluster_id":
				if slug.MatchString(s) {
					attrs[k] = s
				}
			case "operation_type", "result", "error_code":
				if code.MatchString(s) {
					attrs[k] = s
				}
			}
		}
		v.Attributes = attrs
		items = append(items, v.Record)
		if len(items) >= limit {
			break
		}
	}
	return items, scanner.Err()
}

func Render(output io.Writer, records []observability.Record) error {
	if len(records) > 20000 {
		return fmt.Errorf("too many trace records")
	}
	items := append([]observability.Record(nil), records...)
	sort.Slice(items, func(i, j int) bool {
		if items[i].TraceID == items[j].TraceID {
			return items[i].Start < items[j].Start
		}
		return items[i].TraceID < items[j].TraceID
	})
	return view.Execute(output, items)
}

var view = template.Must(template.New("traces").Parse(`<!doctype html><html lang="zh-Hant"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>HugoFleet · 實際部署追蹤</title><style>body{font:14px/1.6 system-ui;background:#f3f6f4;color:#183332;padding:30px;margin:auto;max-width:1500px}h1{margin-bottom:8px}p{color:#657773}table{width:100%;border-collapse:collapse;background:white}th,td{text-align:left;padding:12px;border-bottom:1px solid #dce6e2;vertical-align:top}th{background:#e8f1ed}code{font:11px monospace;overflow-wrap:anywhere}small{display:block;color:#657773}.hub{color:#11695d}.agent{color:#386f9c}.operator{color:#93602c}</style><h1>實際部署追蹤</h1><p>從 Hub／Agent／Operator 的已收集 JSON spans 產生，依 trace 與開始時間排列；parent ID 表示跨程序關聯。此檔是歷史取樣，不是即時畫面。</p><table><thead><tr><th>Trace／Span／Parent</th><th>來源／名稱</th><th>Correlation</th><th>開始時間／耗時</th></tr></thead><tbody>{{range .}}<tr><td><code>{{.TraceID}}</code><small>span <code>{{.SpanID}}</code></small><small>parent <code>{{.ParentID}}</code></small></td><td class="{{.Service}}">{{.Service}}<small>{{.Name}}</small></td><td>{{range $k,$v:=.Attributes}}<small>{{$k}} <code>{{$v}}</code></small>{{end}}</td><td>{{.Start}} ms<small>{{.DurationMicros}} µs</small></td></tr>{{else}}<tr><td colspan="4">尚無實際 span records。</td></tr>{{end}}</tbody></table></html>`))
