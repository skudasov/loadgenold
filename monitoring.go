package loadgen

import (
	"context"
	"github.com/spf13/viper"
	"log"
	"net"
	"sync"
	"time"

	graphite "github.com/cyberdelia/go-metrics-graphite"
	"github.com/rcrowley/go-metrics"
)

var (
	timers                      map[string]metrics.Timer
	errors                      map[string]metrics.Counter
	timerMutex                  sync.RWMutex
	errorMutext                 sync.RWMutex
	gauge                       metrics.Gauge
	pulseDiff                   metrics.Gauge
	observerTotalRecordsFetched metrics.Gauge
	goroutinesCount             int64 = 0
	monitorInit                 sync.Once
)

type Monitored struct {
	Attack
}

func WithMonitor(a Attack) Monitored {
	return Monitored{a}
}

func initMonitoring() {
	url := viper.GetString("graphite.url")
	flushDuration := time.Duration(viper.GetInt("graphite.flushDurationSec"))
	loadratorPrefix := viper.GetString("graphite.loadGeneratorPrefix")

	log.Println("[ grafana-monitoring ] setup graphite")
	log.Printf("[ grafana-monitoring ] url: %s\n", url)
	addr, err := net.ResolveTCPAddr("tcp", url)
	if err != nil {
		log.Fatalf("[grafana-monitoring] ResolveTCPAddr on [%s] failed error [%v] ", url, err)
	}
	go graphite.Graphite(
		metrics.DefaultRegistry,
		flushDuration*time.Second,
		loadratorPrefix,
		addr,
	)
	gauge = metrics.NewGauge()
	pulseDiff = metrics.NewGauge()
	observerTotalRecordsFetched = metrics.NewGauge()
	timers = map[string]metrics.Timer{}
	errors = map[string]metrics.Counter{}
	err = metrics.Register("goroutines-goroutinesCount", gauge)
	if err != nil {
		log.Fatal(err)
	}
	err = metrics.Register("pulseDiff", pulseDiff)
	if err != nil {
		log.Fatal(err)
	}
	err = metrics.Register("totalPulseFetched", observerTotalRecordsFetched)
	if err != nil {
		log.Fatal(err)
	}
}

func registerLabelTimings(label string) metrics.Timer {
	timerMutex.RLock()
	timer, ok := timers[label]
	timerMutex.RUnlock()
	if ok {
		return timer
	}
	timerMutex.Lock()
	defer timerMutex.Unlock()
	timer = metrics.NewTimer()
	timers[label] = timer
	err := metrics.Register(label+"-timer", timer)
	if err != nil {
		log.Println(err)
	}
	return timer
}

func registerErrCount(label string) metrics.Counter {
	errorMutext.RLock()
	cnt, ok := errors[label]
	errorMutext.RUnlock()
	if ok {
		return cnt
	}
	errorMutext.Lock()
	defer errorMutext.Unlock()
	cnt = metrics.NewCounter()
	errors[label] = cnt
	err := metrics.Register(label+"-err", cnt)
	if err != nil {
		log.Fatal(err)
	}
	return cnt
}

func (m Monitored) Do(ctx context.Context) DoResult {
	before := time.Now()
	result := m.Attack.Do(ctx)
	registerLabelTimings(result.RequestLabel).Update(time.Now().Sub(before))
	if result.Error != nil || result.StatusCode >= 400 {
		registerErrCount(result.RequestLabel).Inc(1)
	}
	return result
}

func (m Monitored) Setup(lm *LoadManager, c Config) error {
	if err := m.Attack.Setup(lm, c); err != nil {
		return err
	}
	monitorInit.Do(initMonitoring)
	return nil
}

func (m Monitored) Clone() Attack {
	goroutinesCount++
	monitorInit.Do(initMonitoring)
	gauge.Update(goroutinesCount)
	return Monitored{m.Attack.Clone()}
}

func (m Monitored) PutData(mo interface{}) error {
	if err := m.Attack.PutData(mo); err != nil {
		return err
	}
	return nil
}
