#### Load generation library
Created for load tests that use (generated) clients in Go to communicate to services (in any supported language). By providing the Attack interface, any client and protocol could potentially be tested with this package.

Compared to existing HTTP load testing tools (e.g. tsenart/vegeta) that can send raw HTTP requests, this package requires the use of client code to send the requests and receive the response.

This tool is heavily based on https://github.com/emicklei/hazana, with added functionality:
- [x] multiple generators in one runtime
- [x] generate grafana dashboard for all attackers
- [x] load and store data for attackers
- [x] performance degradation checks
- [x] dump transport for debug
- [ ] automatic generation of load profile from logs
```
go get github.com/insolar/loadgen
```

Implement interface for attacker
```go
// Attack must be implemented by a service client.
type Attack interface {
	// GetManager gets load manager with all required data files/readers/writers
	GetManager() *LoadManager
	// Setup should establish the connection to the service
	// It may want to access the config of the Runner.
	Setup(lm *LoadManager, c Config) error
	// Do performs one request and is executed in a separate goroutine.
	// The context is used to cancel the request on timeout.
	Do(ctx context.Context) DoResult
	// Teardown can be used to close the connection to the service
	Teardown() error
	// Clone should return a fresh new Attack
	// Make sure the new Attack has values for shared struct fields initialized at Setup.
	Clone() Attack
	// StoreData should return if this scenario will save data, that gonna be needed for another scenario or verification
	StoreData() bool
	// PutData writes object representation to handle file
	PutData(mo interface{}) error
	// GetData reads object from handle file
	GetData() (interface{}, error)
}
```
Add new attacker type to factory
```go
func AttackerFromName(name string) loadgen.Attack {
	switch name {
	case "new_attacker":
		return loadgen.WithMonitor(new(NewAttack))
	default:
		log.Fatalf("unknown attacker type: %s", name)
	}
	return nil
}
```
Create labels.go file, label constants used to generate grafana dashboard
```go
package observer_load

const (
	MemberCreateLabel          = "member_create"
	MemberTransferLabel        = "member_transfer"
	FeeLabel                   = "fee"
	MarketStatsLabel           = "market-stats"
	MemberGetLabel             = "get_member"
	GetClosedTransactionsLabel = "get_closed_transactions"
	GetMemberTransactionsLabel = "get_member_transactions"
	GetTransactionLabel        = "get_transaction"
	GetTransactionsLabel       = "get_transactions"
	MemberGetBalanceLabel      = "get_member_balance"
	MigrateDepositLabel        = "deposit_migration"
	StaticLabel                = "static"
)
```
If you are writing prepare test, you can use file to put data (specify csv_write and store_data)
```yaml
dumptransport: true
http_timeout: 120
handles:
  migration_member_xns_create:
    rps: 2
    rampUpSec: 1
    attackTimeSec: 2
    maxAttackers: 2
    csv_write: member-refs.csv
    store_data: true
```
And later use this file in another test (specify csv_read, and recycle_data flag)
```yaml
dumptransport: true
http_timeout: 20
handles:
  transfer:
    rps: 30
    rampUpSec: 1800
    attackTimeSec: 7200
    maxAttackers: 2000
    csv_read: member-refs.csv
    recycle_data: true
    csv_write: tx-refs.csv
    store_data: true
```

#### Metrics
Graphite and Prometheus default configs can be specified in run config
```yaml
graphite:
  url: 0.0.0.0:2003
  flushDurationSec: 1
  loadGeneratorPrefix: observer
prometheus:
  # dev prometheus
  url: http://192.168.85.254:9090/
  env_label: stage
  namespace: stage
  pulse_diff_check_interval_sec: 5
  pulse_lag_threshold: 70
  opened_requests_threshold: 20
```

Or set default values before suite run
```yaml
func Defaults() {
	viper.SetDefault("generator.target", "https://wallet-api.mainnet.insolar.io")
	viper.SetDefault("generator.responseTimeoutSec", 20)
	viper.SetDefault("generator.rampUpStrategy", "linear")
	viper.SetDefault("generator.verbose", true)

	viper.SetDefault("graphite.url", "0.0.0.0:2003")
	viper.SetDefault("graphite.flushDurationSec", 1)
	viper.SetDefault("graphite.loadGeneratorPrefix", "observer")

	viper.SetDefault("prometheus.url", "http://192.168.85.254:9090/")
	viper.SetDefault("prometheus.env_label", "stage")
	viper.SetDefault("prometheus.namespace", "stage-wallet")
	viper.SetDefault("prometheus.pulse_lag_threshold", 70)
	viper.SetDefault("prometheus.opened_requests_threshold", 20)

	viper.SetDefault("checks.handle_threshold_percent", 1.20)

	viper.SetDefault("dashboard_dir", "dashboard")
	viper.SetDefault("load_scripts_dir", "load")
}
```

Create suite and run it
```go
func main() {
	// you can override viper default config values if you have a lot of common load profiles in separate yamls
	config.Defaults()
	loadgen.CIRun(observer_load.AttackerFromName)
}
```

```go
go run load/cmd/load/main.go -config load/run-configs/prod-min.yaml
```

For more examples see [this](https://github.com/insolar/go-autotests) repo

#### CI Run
If handle threshold percent is reached (default is 20% of p50 for any handle), or there is errors in any handle, pipeline will fail.
```yaml
checks:
    handle_threshold_percent: 1.2
```
All reports for handle is stored in reports dir

#### Debug
Bootstrap local kamon for debugging metrics, export dashboard from dir
```
docker run -d -p 8181:80 -p 8125:8125/udp -p 8126:8126 --publish=2003:2003 --name kamon-grafana-dashboard kamon/grafana_graphite
```
Turn on dumptransport in run config:
```yaml
dumptransport: true
```