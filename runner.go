package loadgen

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"sync"
	"time"

	"go.uber.org/ratelimit"
)

// BeforeRunner can be implemented by an Attacker
// and its method is called before a test or Run.
type BeforeRunner interface {
	BeforeRun(c Config) error
}

// AfterRunner can be implemented by an Attacker
// and its method is called after a test or Run.
// The report is passed to compute the Failed field and/or store values in Output.
type AfterRunner interface {
	AfterRun(r *RunReport) error
}

type Runner struct {
	name            string
	ReadCsvName     string
	WriteCsvName    string
	RecycleData     bool
	sequence        int
	m               *LoadManager
	config          Config
	attackers       []Attack
	next, quit      chan bool
	results         chan result
	prototype       Attack
	metrics         map[string]*Metrics
	resultsPipeline func(r result) result
}

func NewRunner(name string, lm *LoadManager, a Attack, c Config) *Runner {
	r := new(Runner)
	r.name = name
	r.m = lm
	r.config = c
	r.prototype = a
	r.sequence = c.SequenceNum
	if c.Verbose {
		log.Printf("** [%s] bootstraping generator **\n", r.name)
		log.Printf("[%d] available logical CPUs\n", runtime.NumCPU())
	}

	// validate the configuration
	if msg := c.Validate(); len(msg) > 0 {
		for _, each := range msg {
			fmt.Println("a configuration error was found", each)
		}
		fmt.Println()
		flag.Usage()
		os.Exit(0)
	}

	// is the attacker interested in the Run lifecycle?
	if lifecycler, ok := a.(BeforeRunner); ok {
		if err := lifecycler.BeforeRun(c); err != nil {
			log.Fatalln("BeforeRun failed", err)
		}
	}

	// do a test if the flag says so
	if *oSample > 0 {
		r.test(*oSample)
		report := RunReport{}
		if lifecycler, ok := a.(AfterRunner); ok {
			if err := lifecycler.AfterRun(&report); err != nil {
				log.Fatalln("AfterRun failed", err)
			}
		}
		os.Exit(0)
		// unreachable
		return r
	}
	r.init()
	return r
}

func (r *Runner) init() {
	r.next = make(chan bool)
	r.quit = make(chan bool)
	r.results = make(chan result)
	r.attackers = []Attack{}
	r.metrics = make(map[string]*Metrics)
	r.resultsPipeline = r.addResult
}

func (r *Runner) spawnAttacker() {
	if r.config.Verbose {
		log.Printf("[%s] setup and spawn new attacker [%d]\n", r.name, len(r.attackers)+1)
	}
	attacker := r.prototype.Clone()
	if err := attacker.Setup(r.m, r.config); err != nil {
		log.Printf("[%s] attacker [%d] setup failed with [%v]\n", r.name, len(r.attackers)+1, err)
		return
	}
	r.attackers = append(r.attackers, attacker)
	go attack(attacker, r.next, r.quit, r.results, r.config.timeout())
}

// addResult is called from a dedicated goroutine.
func (r *Runner) addResult(s result) result {
	m, ok := r.metrics[s.doResult.RequestLabel]
	if !ok {
		m = new(Metrics)
		r.metrics[s.doResult.RequestLabel] = m
	}
	m.add(s)
	return s
}

// test uses the Attack to perform {count} calls and report its result
// it is intended for development of an Attack implementation.
func (r *Runner) test(count int) {
	probe := r.prototype.Clone()
	if err := probe.Setup(r.m, r.config); err != nil {
		log.Printf("test attack setup failed [%v]", err)
		return
	}
	defer probe.Teardown()
	for s := count; s > 0; s-- {
		now := time.Now()
		result := probe.Do(context.Background())
		log.Printf("test attack call [%s] took [%v] with status [%v] and error [%v]\n", result.RequestLabel, time.Now().Sub(now), result.StatusCode, result.Error)
	}
}

func (r *Runner) SetupHandleStore(m *LoadManager) {
	csvReadName := r.config.ReadFromCsvName
	recycleData := r.config.RecycleData
	if csvReadName != "" {
		log.Printf("creating read file: %s\n", csvReadName)
		f, err := os.Open(csvReadName)
		if err != nil {
			log.Fatalf("no csv read file found: %s", csvReadName)
		}
		m.CsvStore[csvReadName] = NewCSVData(f, recycleData)
	}
	csvWriteName := r.config.WriteToCsvName
	if csvWriteName != "" {
		log.Printf("creating write file: %s\n", csvWriteName)
		csvFile := createIfNotExists(csvWriteName)
		m.CsvStore[csvWriteName] = NewCSVData(csvFile, false)
	}
}

// Run offers the complete flow of a load test.
func (r *Runner) Run(wg *sync.WaitGroup, lm *LoadManager) {
	if wg != nil {
		defer wg.Done()
	}
	if lifecycler, ok := r.prototype.(BeforeRunner); ok {
		if err := lifecycler.BeforeRun(r.config); err != nil {
			log.Fatalln("BeforeRun failed", err)
		}
	}
	go r.collectResults()
	r.rampUp()
	r.fullAttack()
	r.quitAttackers()
	r.tearDownAttackers()
	report := RunReport{}
	if lifecycler, ok := r.prototype.(AfterRunner); ok {
		if err := lifecycler.AfterRun(&report); err != nil {
			log.Fatalln("AfterRun failed", err)
		}
	}
	lm.CsvMu.Lock()
	defer lm.CsvMu.Unlock()
	lm.Reports[r.name] = r.reportMetrics()
}

func (r *Runner) fullAttack() {
	// attack can only proceed when at least one attacker is waiting for rps tokens
	if len(r.attackers) == 0 {
		// rampup probably has failed too
		return
	}
	if r.config.Verbose {
		log.Printf("begin full attack of [%d] remaining seconds\n", r.config.AttackTimeSec-r.config.RampUpTimeSec)
	}
	fullAttackStartedAt = time.Now()
	limiter := ratelimit.New(r.config.RPS) // per second
	doneDeadline := time.Now().Add(time.Duration(r.config.AttackTimeSec-r.config.RampUpTimeSec) * time.Second)
	for time.Now().Before(doneDeadline) {
		limiter.Take()
		r.next <- true
	}
	if r.config.Verbose {
		log.Printf("end full attack")
	}
}

func (r *Runner) rampUp() {
	strategy := r.config.rampupStrategy()
	if r.config.Verbose {
		log.Printf("begin rampup of [%d] seconds to RPS [%d] within attack of [%d] seconds using strategy [%s]\n", r.config.RampUpTimeSec, r.config.RPS, r.config.AttackTimeSec, strategy)
	}
	switch strategy {
	case "linear":
		linearIncreasingGoroutinesAndRequestsPerSecondStrategy{}.execute(r)
	case "exp2":
		spawnAsWeNeedStrategy{}.execute(r)
	}
	// restore pipeline function incase it was changed by the rampup strategy
	r.resultsPipeline = r.addResult
	if r.config.Verbose {
		log.Printf("end rampup ending up with [%d] attackers\n", len(r.attackers))
	}
}

func (r *Runner) quitAttackers() {
	if r.config.Verbose {
		log.Printf("stopping attackers [%d]\n", len(r.attackers))
	}
	for range r.attackers {
		r.quit <- true
	}
}

func (r *Runner) tearDownAttackers() {
	if r.config.Verbose {
		log.Printf("tearing down attackers [%d]\n", len(r.attackers))
	}
	for i, each := range r.attackers {
		if err := each.Teardown(); err != nil {
			log.Printf("failed to teardown attacker [%d]:%v\n", i, err)
		}
	}
}

func (r *Runner) reportMetrics() *RunReport {
	for _, each := range r.metrics {
		each.updateLatencies()
	}
	return &RunReport{
		StartedAt:     fullAttackStartedAt,
		FinishedAt:    time.Now(),
		Configuration: r.config,
		Metrics:       r.metrics,
		Failed:        false, // must be overwritten by program
		Output:        map[string]interface{}{},
	}
}

func (r *Runner) collectResults() {
	for {
		r.resultsPipeline(<-r.results)
	}
}
