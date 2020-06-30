package loadgen

import (
	"encoding/json"
	"fmt"
	"github.com/insolar/x-crypto/ecdsa"
	"github.com/spf13/viper"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"syscall"
	"time"
)

const (
	ReportFileTmpl = "%s-%d.json"
)

var (
	handleReportRe = regexp.MustCompile(`-(\d+).json`)
	sigs           = make(chan os.Signal, 1)
)

// LoadManager manages data and finish criteria
type LoadManager struct {
	RootMemberPrivateKey *ecdsa.PrivateKey
	RootMemberPublicKey  *ecdsa.PublicKey
	CsvMu                *sync.Mutex
	// Groups runner objects that fires .Do()
	Groups []*Runner
	// AttackerConfigs attacker configs
	AttackerConfigs map[string]Config
	// Reports run reports for every handle
	Reports map[string]*RunReport
	// CsvStore stores data for all attackers
	CsvStore  map[string]*CSVData
	ReportDir string
	// When degradation threshold is reached for any handle, see default config
	Degradation bool
	// When there are errors in any handle
	Failed bool
}

// NewLoadManager create load manager with data files
func NewLoadManager() *LoadManager {
	var err error
	lm := &LoadManager{
		CsvMu:       &sync.Mutex{},
		Reports:     make(map[string]*RunReport),
		CsvStore:    make(map[string]*CSVData),
		Degradation: false,
	}
	if lm.ReportDir, err = filepath.Abs(filepath.Join("load", "reports")); err != nil {
		log.Fatal(err)
	}
	return lm
}

func (m *LoadManager) SetupHandleStore(handle Config) {
	csvReadName := handle.ReadFromCsvName
	recycleData := handle.RecycleData
	if csvReadName != "" {
		log.Printf("creating read file: %s\n", csvReadName)
		f, err := os.Open(csvReadName)
		if err != nil {
			log.Fatalf("no csv read file found: %s", csvReadName)
		}
		m.CsvStore[csvReadName] = NewCSVData(f, recycleData)
	}
	csvWriteName := handle.WriteToCsvName
	if csvWriteName != "" {
		log.Printf("creating write file: %s\n", csvWriteName)
		csvFile := createIfNotExists(csvWriteName)
		m.CsvStore[csvWriteName] = NewCSVData(csvFile, false)
	}
}

func (m *LoadManager) HandleShutdownSignal() {
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigs
		fmt.Println("exit signal received, exiting")
		m.Shutdown()
		os.Exit(1)
	}()
}

func (m *LoadManager) Shutdown() {
	for _, s := range m.CsvStore {
		s.Flush()
		s.f.Close()
	}
}

// RunSuite starts suite and wait for all generator to shutdown
func (m *LoadManager) RunSuite() {
	m.HandleShutdownSignal()

	t := timeNow()
	startTime := epochNowMillis(t)
	hrStartTime := timeHumanReadable(t)
	mode := viper.GetString("execution_mode")
	if mode == "parallel" {
		var wg sync.WaitGroup
		wg.Add(len(m.Groups))

		for _, r := range m.Groups {
			r.SetupHandleStore(m)
			go r.Run(&wg, m)
		}
		wg.Wait()
	} else if mode == "sequence" {
		// Used to prepare data by sequence of tests
		SortGroupsBySequenceNum(m.Groups)
		for _, r := range m.Groups {
			r.SetupHandleStore(m)
			r.Run(nil, m)
		}
	}

	t = timeNow()
	finishTime := epochNowMillis(t)
	hrFinishTime := timeHumanReadable(t)

	TimerangeUrl(startTime, finishTime)
	HumanReadableTestInterval(hrStartTime, hrFinishTime)

	m.Shutdown()
}

func (m *LoadManager) CsvForHandle(name string) *CSVData {
	s, ok := m.CsvStore[name]
	if !ok {
		log.Fatalf("no csv storage file found for: %s", name)
	}
	return s
}

// StoreHandleReports stores report for every handle in suite
func (m *LoadManager) StoreHandleReports() {
	ts := time.Now().Unix()
	for handleName, r := range m.Reports {
		b, err := json.MarshalIndent(r, "", "    ")
		if err != nil {
			log.Fatal(err)
		}
		repPath := filepath.Join(m.ReportDir, fmt.Sprintf(ReportFileTmpl, handleName, ts))
		log.Printf("writing report for handle [%s] in %s", handleName, repPath)
		if err := ioutil.WriteFile(repPath, b, 0777); err != nil {
			log.Fatal(err)
		}
		if !m.Degradation {
			m.WriteLastSuccess(handleName, ts)
		}
	}
}

// WriteLastSuccess writes ts of last successful run for handle
func (m *LoadManager) WriteLastSuccess(handleName string, ts int64) {
	lastSuccessFile := filepath.Join(m.ReportDir, handleName+"_last")
	createIfNotExists(lastSuccessFile)
	err := ioutil.WriteFile(lastSuccessFile, []byte(strconv.Itoa(int(ts))), 0777)
	if err != nil {
		log.Fatal(err)
	}
}

// CheckErrors check errors logic
func (m *LoadManager) CheckErrors() {
	for handleName, currentReport := range m.Reports {
		if len(currentReport.Metrics[handleName].Errors) > 0 {
			m.Failed = true
		}
	}
}

// CheckDegradation checks handle performance degradation to last successful run stored in *handle_name*_last file
func (m *LoadManager) CheckDegradation() {
	handleThreshold := viper.GetFloat64("checks.handle_threshold_percent")
	for handleName, currentReport := range m.Reports {
		lastReport, err := m.LastSuccessReportForHandle(handleName)
		if os.IsNotExist(err) {
			log.Printf("nothing to compare for %s handle, no reports in %s", handleName, m.ReportDir)
			continue
		}
		if _, ok := lastReport.Metrics[handleName]; !ok {
			log.Fatalf("no last report for handle %s found in last report", handleName)
		}
		if _, ok := currentReport.Metrics[handleName]; !ok {
			log.Fatalf("no last report for handle %s found in current report", handleName)
		}
		currentMean := currentReport.Metrics[handleName].Latencies.P50 / time.Millisecond
		lastMean := lastReport.Metrics[handleName].Latencies.P50 / time.Millisecond
		fmt.Printf("[ %s ] current: %dms, last: %dms\n", handleName, currentMean, lastMean)
		fmt.Printf("ratio: %f\n", float64(currentMean)/float64(lastMean))
		if float64(currentMean)/float64(lastMean) >= handleThreshold {
			log.Printf("p50 degradation of %s handle: %d > %d", handleName, currentMean, lastMean)
			m.Degradation = true
			continue
		}
	}
}

// LastSuccessReportForHandle gets last successful report for a handle
func (m *LoadManager) LastSuccessReportForHandle(handleName string) (*RunReport, error) {
	f, err := os.Open(filepath.Join(m.ReportDir, handleName+"_last"))
	defer f.Close()
	if err != nil {
		return nil, err
	}
	lastTs, err := ioutil.ReadAll(f)
	if err != nil {
		log.Fatal(err)
	}
	lf, err := os.Open(filepath.Join(m.ReportDir, fmt.Sprintf("%s-%s.json", handleName, string(lastTs))))
	defer lf.Close()
	if err != nil {
		log.Fatal(err)
	}
	data, err := ioutil.ReadAll(lf)
	if err != nil {
		log.Fatal(err)
	}
	var runReport RunReport
	if err := json.Unmarshal(data, &runReport); err != nil {
		log.Fatal(err)
	}
	return &runReport, nil
}

// createIfNotExists creates file if not exists, used to not override csv data
func createIfNotExists(fname string) *os.File {
	var file *os.File
	fpath, _ := filepath.Abs(fname)
	_, err := os.Stat(fpath)
	if err != nil {
		file, err = os.Create(fname)
	} else {
		log.Fatalf("file %s already exists, please rename write_csv or read_csv file name in config", fname)
	}
	if err != nil {
		log.Fatal(err)
	}
	return file
}
