package loadgen

import (
	"encoding/json"
	"fmt"
	"github.com/spf13/viper"
	"go/ast"
	"go/parser"
	"go/token"
	"io/ioutil"
	"log"
	"os"
	"strings"
)

const (
	dashboardPath = "%s/%s/auto.json"
	labelsPath    = "%s/labels.go"
)

var (
	percentiles              = []string{"50", "95", "99"}
	rpsLabelSuffixes         = []string{"timer", "err"}
	projectMetricPrefix      = "observer"
	percentilesScaleFactor   = "0.000001"
	alias                    = "%s-%s"
	percentileTargetTemplate = "alias(scale(%s.%s-timer.%s-percentile, %s), '%s')"
	rpsTargetTemplate        = "alias(perSecond(%s.%s-%s.count_ps), '%s')"
	goroutinesTotalTemplate  = "%s.goroutines-goroutinesCount.value"
	PanelID                  = 0
)

type Inputs []struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Type        string `json:"type"`
	PluginID    string `json:"pluginId"`
	PluginName  string `json:"pluginName"`
}

type Requires []struct {
	Type    string `json:"type"`
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version"`
}

type TimePicker struct {
	RefreshIntervals []string `json:"refresh_intervals"`
	TimeOptions      []string `json:"time_options"`
}

type Time struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type Templating struct {
	List []interface{} `json:"list"`
}

type Annotations struct {
	List []interface{} `json:"list"`
}

// Rows data
type Yaxe struct {
	Format  string      `json:"format"`
	Label   interface{} `json:"label"`
	LogBase int         `json:"logBase"`
	Max     interface{} `json:"max"`
	Min     interface{} `json:"min"`
	Show    bool        `json:"show"`
}

type Xaxe struct {
	Buckets interface{}   `json:"buckets"`
	Mode    string        `json:"mode"`
	Name    interface{}   `json:"name"`
	Show    bool          `json:"show"`
	Values  []interface{} `json:"values"`
}

type Tooltip struct {
	Shared    bool   `json:"shared"`
	Sort      int    `json:"sort"`
	ValueType string `json:"value_type"`
}

type Target struct {
	RefID  string `json:"refId"`
	Target string `json:"target"`
}

type Legend struct {
	Avg     bool `json:"avg"`
	Current bool `json:"current"`
	Max     bool `json:"max"`
	Min     bool `json:"min"`
	Show    bool `json:"show"`
	Total   bool `json:"total"`
	Values  bool `json:"values"`
}

type Panel struct {
	AliasColors     struct{}      `json:"aliasColors"`
	Bars            bool          `json:"bars"`
	DashLength      int           `json:"dashLength"`
	Dashes          bool          `json:"dashes"`
	Datasource      string        `json:"datasource"`
	Fill            int           `json:"fill"`
	ID              int           `json:"id"`
	Legend          Legend        `json:"legend"`
	Lines           bool          `json:"lines"`
	Linewidth       int           `json:"linewidth"`
	Links           []interface{} `json:"links"`
	NullPointMode   string        `json:"nullPointMode"`
	Percentage      bool          `json:"percentage"`
	Pointradius     int           `json:"pointradius"`
	Points          bool          `json:"points"`
	Renderer        string        `json:"renderer"`
	SeriesOverrides []interface{} `json:"seriesOverrides"`
	SpaceLength     int           `json:"spaceLength"`
	Span            int           `json:"span"`
	Stack           bool          `json:"stack"`
	SteppedLine     bool          `json:"steppedLine"`
	Targets         []Target      `json:"targets"`
	Thresholds      []interface{} `json:"thresholds"`
	TimeFrom        interface{}   `json:"timeFrom"`
	TimeShift       interface{}   `json:"timeShift"`
	Title           string        `json:"title"`
	Tooltip         Tooltip       `json:"tooltip"`
	Type            string        `json:"type"`
	Xaxis           Xaxe          `json:"xaxis"`
	Yaxes           []Yaxe        `json:"yaxes"`
}

type Row struct {
	Collapse        bool        `json:"collapse"`
	Height          int         `json:"height"`
	Panels          []Panel     `json:"panels"`
	Repeat          interface{} `json:"repeat"`
	RepeatIteration interface{} `json:"repeatIteration"`
	RepeatRowID     interface{} `json:"repeatRowId"`
	ShowTitle       bool        `json:"showTitle"`
	Title           string      `json:"title"`
	TitleSize       string      `json:"titleSize"`
}

type Dashboard struct {
	Inputs        Inputs        `json:"__inputs"`
	Requires      Requires      `json:"__requires"`
	Annotations   Annotations   `json:"annotations"`
	Editable      bool          `json:"editable"`
	GnetID        interface{}   `json:"gnetId"`
	GraphTooltip  int           `json:"graphTooltip"`
	HideControls  bool          `json:"hideControls"`
	ID            interface{}   `json:"id"`
	Links         []interface{} `json:"links"`
	Refresh       string        `json:"refresh"`
	Rows          []Row         `json:"rows"`
	SchemaVersion int           `json:"schemaVersion"`
	Style         string        `json:"style"`
	Tags          []interface{} `json:"tags"`
	Templating    Templating    `json:"templating"`
	Time          Time          `json:"time"`
	Timepicker    TimePicker    `json:"timepicker"`
	Timezone      string        `json:"timezone"`
	Title         string        `json:"title"`
	Version       int           `json:"version"`
}

func GenerateGoroutinesTotalTarget() []Target {
	return []Target{
		{
			Target: fmt.Sprintf(goroutinesTotalTemplate, projectMetricPrefix),
		},
	}
}

func GeneratePercentileTargets(labels []string) []Target {
	targets := make([]Target, 0)
	for _, label := range labels {
		for _, percentile := range percentiles {
			title := fmt.Sprintf(alias, label, percentile)
			targetRequest := fmt.Sprintf(
				percentileTargetTemplate,
				projectMetricPrefix,
				label,
				percentile,
				percentilesScaleFactor,
				title,
			)
			targets = append(targets, Target{
				Target: targetRequest,
			})
		}
	}
	return targets
}

func GenerateRPSTargets(labels []string) []Target {
	targets := make([]Target, 0)
	for _, label := range labels {
		for _, suffix := range rpsLabelSuffixes {
			title := fmt.Sprintf(alias, label, suffix)
			targetRequest := fmt.Sprintf(
				rpsTargetTemplate,
				projectMetricPrefix,
				label,
				suffix,
				title,
			)
			targets = append(targets, Target{
				Target: targetRequest,
			})
		}
	}
	return targets
}

func GenerateRPSPanel(title string, targets []Target) Panel {
	PanelID += 1
	return Panel{
		AliasColors: struct{}{},
		Bars:        false,
		DashLength:  10,
		Dashes:      false,
		Datasource:  "${DS_LOCAL_GRAPHITE}",
		Fill:        1,
		ID:          RandInt(),
		Legend: Legend{
			Show: true,
		},
		Lines:           true,
		Linewidth:       1,
		Links:           []interface{}{},
		NullPointMode:   "null",
		Percentage:      false,
		Pointradius:     5,
		Points:          false,
		Renderer:        "flot",
		SeriesOverrides: []interface{}{},
		SpaceLength:     10,
		Span:            12,
		Stack:           false,
		SteppedLine:     false,
		Targets:         targets,
		Thresholds:      []interface{}{},
		TimeFrom:        nil,
		TimeShift:       nil,
		Title:           title,
		Tooltip: Tooltip{
			ValueType: "individual",
			Shared:    true,
		},
		Type: "graph",
		Xaxis: Xaxe{
			Values: []interface{}{},
			Mode:   "time",
			Show:   true,
		},
		Yaxes: []Yaxe{
			{
				Format:  "short",
				LogBase: 1,
				Show:    true,
			},
			{
				Format:  "short",
				LogBase: 1,
				Show:    true,
			},
		},
	}
}

func GenerateInfoPanel(title string, targets []Target) Panel {
	return Panel{
		AliasColors: struct{}{},
		Bars:        false,
		DashLength:  10,
		Dashes:      false,
		Datasource:  "${DS_LOCAL_GRAPHITE}",
		Fill:        1,
		ID:          RandInt(),
		Legend: Legend{
			Show: true,
		},
		Lines:           true,
		Linewidth:       1,
		Links:           []interface{}{},
		NullPointMode:   "null",
		Percentage:      false,
		Pointradius:     5,
		Points:          false,
		Renderer:        "flot",
		SeriesOverrides: []interface{}{},
		SpaceLength:     10,
		Span:            12,
		Stack:           false,
		SteppedLine:     false,
		Targets:         targets,
		Thresholds:      []interface{}{},
		TimeFrom:        nil,
		TimeShift:       nil,
		Title:           title,
		Tooltip: Tooltip{
			ValueType: "individual",
			Shared:    true,
		},
		Type: "graph",
		Xaxis: Xaxe{
			Values: []interface{}{},
			Mode:   "time",
			Show:   true,
		},
		Yaxes: []Yaxe{
			{
				Format:  "short",
				LogBase: 1,
				Show:    true,
			},
			{
				Format:  "short",
				LogBase: 1,
				Show:    true,
			},
		},
	}
}

func GeneratePercentilePanel(title string, targets []Target) Panel {
	return Panel{
		AliasColors: struct{}{},
		Bars:        false,
		DashLength:  10,
		Dashes:      false,
		Datasource:  "${DS_LOCAL_GRAPHITE}",
		Fill:        1,
		ID:          RandInt(),
		Legend: Legend{
			Show: true,
		},
		Lines:           true,
		Linewidth:       1,
		Links:           []interface{}{},
		NullPointMode:   "null",
		Percentage:      false,
		Pointradius:     5,
		Points:          false,
		Renderer:        "flot",
		SeriesOverrides: []interface{}{},
		SpaceLength:     10,
		Span:            12,
		Stack:           false,
		SteppedLine:     false,
		Targets:         targets,
		Thresholds:      []interface{}{},
		TimeFrom:        nil,
		TimeShift:       nil,
		Title:           title,
		Tooltip: Tooltip{
			ValueType: "individual",
			Shared:    true,
		},
		Type: "graph",
		Xaxis: Xaxe{
			Values: []interface{}{},
			Mode:   "time",
			Show:   true,
		},
		Yaxes: []Yaxe{
			{
				Format:  "ms",
				LogBase: 1,
				Show:    true,
			},
			{
				Format:  "short",
				LogBase: 1,
				Show:    true,
			},
		},
	}
}

func GenerateRow(panel Panel) Row {
	return Row{
		Collapse:  false,
		Height:    360,
		Title:     "Dashboard row",
		TitleSize: "h6",
		Panels:    []Panel{panel},
	}
}

func GenerateRows(labels []string) []Row {
	infoTargets := GenerateGoroutinesTotalTarget()
	percTargets := GeneratePercentileTargets(labels)
	rpsTargets := GenerateRPSTargets(labels)
	infoPanel := GenerateInfoPanel("Generator Debug Info", infoTargets)
	percPanel := GeneratePercentilePanel("Percentiles (90,95,99)", percTargets)
	rpsPanel := GenerateRPSPanel("RPS (Total+Errors)", rpsTargets)
	infoRow := GenerateRow(infoPanel)
	percRow := GenerateRow(percPanel)
	rpsRow := GenerateRow(rpsPanel)

	rows := make([]Row, 0)
	rows = append(rows, percRow, rpsRow, infoRow)
	return rows
}

func GrafanaDashboard(title string, labels []string) Dashboard {
	rows := GenerateRows(labels)
	return Dashboard{
		Inputs: Inputs{
			{
				Name:        "DS_LOCAL_GRAPHITE",
				Label:       "Local Graphite",
				Description: "",
				Type:        "datasource",
				PluginID:    "graphite",
				PluginName:  "Graphite",
			},
		},
		Requires: Requires{
			{
				Type:    "grafana",
				ID:      "grafana",
				Name:    "Grafana",
				Version: "4.4.3",
			},
			{
				Type:    "panel",
				ID:      "graph",
				Name:    "Graph",
				Version: "",
			},
			{
				Type:    "datasource",
				ID:      "graphite",
				Name:    "Graphite",
				Version: "1.0.0",
			},
		},
		Annotations: Annotations{
			List: []interface{}{},
		},
		Editable:      true,
		GnetID:        nil,
		GraphTooltip:  0,
		HideControls:  false,
		ID:            nil,
		Links:         []interface{}{},
		Refresh:       "5s",
		Rows:          rows,
		SchemaVersion: 14,
		Style:         "dark",
		Tags:          []interface{}{},
		Templating: Templating{
			List: []interface{}{},
		},
		Time: Time{
			From: "now-5m",
			To:   "now",
		},
		Timepicker: TimePicker{
			RefreshIntervals: []string{"5s",
				"10s",
				"30s",
				"1m",
				"5m",
				"15m",
				"30m",
				"1h",
				"2h",
				"1d",
			},
			TimeOptions: []string{"5m",
				"15m",
				"1h",
				"6h",
				"12h",
				"24h",
				"2d",
				"7d",
				"30d",
			},
		},
		Timezone: "",
		Title:    title,
		Version:  1,
	}
}

func GenerateGrafanaDashboard() {
	title := viper.GetString("graphite.loadGeneratorPrefix")
	dashboard, err := json.Marshal(GrafanaDashboard(title, ParseLabels()))
	if err != nil {
		log.Fatal(err)
	}
	fpath := fmt.Sprintf(dashboardPath, viper.GetString("load_scripts_dir"), viper.GetString("dashboard_dir"))
	if err := ioutil.WriteFile(fpath, dashboard, os.ModePerm); err != nil {
		log.Fatal(err)
	}
}

func ParseLabels() []string {
	fset := token.NewFileSet()
	fpath := fmt.Sprintf(labelsPath, viper.GetString("load_scripts_dir"))
	node, err := parser.ParseFile(
		fset,
		fpath,
		nil,
		parser.ParseComments,
	)
	if err != nil {
		log.Fatalf("load scripts must have %s: %s", fpath, err)
	}
	declarations := make([]token.Pos, 0)
	labels := make([]string, 0)
	ast.Inspect(node, func(n ast.Node) bool {
		var s string
		switch x := n.(type) {
		case *ast.BasicLit:
			labels = append(labels, strings.Replace(x.Value, "\"", "", -1))
		case *ast.GenDecl:
			declarations = append(declarations, x.Pos())
		}
		if s != "" {
			fmt.Printf("%s:\t%s\n", fset.Position(n.Pos()), s)
		}
		return true
	})
	if len(declarations) > 1 {
		log.Fatal("labels.go file must contains only one const declaration, used to generate dashboards")
	}
	return labels
}
