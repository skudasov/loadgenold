package loadgen

import (
	"bytes"
	"encoding/json"
	"github.com/spf13/viper"
	"io/ioutil"
	"log"
	"net/http"
)

var (
	orgId             = 1
	timerangeTemplate = "Grafana test data: %s/dashboard/db/observer?orgId=%d&from=%d&to=%d"
)

func TimerangeUrl(fromEpoch int64, toEpoch int64) {
	url := viper.GetString("grafana.url")
	log.Printf(timerangeTemplate, url, orgId, fromEpoch, toEpoch)
}

func HumanReadableTestInterval(from string, to string) {
	log.Printf("Test time: %s - %s", from, to)
}

type UploadInput struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	PluginID string `json:"pluginId"`
	Value    string `json:"value"`
}

type ImportPayload struct {
	Dashboard Dashboard     `json:"dashboard"`
	Overwrite bool          `json:"overwrite"`
	Inputs    []UploadInput `json:"inputs"`
}

func UploadGrafanaDashboard() {
	title := viper.GetString("graphite.loadGeneratorPrefix")
	url := viper.GetString("grafana.url") + "/api/dashboards/import"
	log.Printf("importing grafana dashboard to %s", url)
	dashboard := GrafanaDashboard(title, ParseLabels())
	payload := ImportPayload{
		Dashboard: dashboard,
		Overwrite: true,
		Inputs: []UploadInput{
			{
				Name:     "DS_LOCAL_GRAPHITE",
				Type:     "datasource",
				PluginID: "graphite",
				Value:    "Local Graphite",
			},
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		log.Fatal(err)
	}
	dataBuf := bytes.NewBuffer(data)
	resp, err := http.Post(url, "application/json", dataBuf)
	if err != nil {
		log.Fatal(err)
	}
	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	log.Printf("import result: %s", respBody)
}
