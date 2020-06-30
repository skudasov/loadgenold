package loadgen

import (
	"encoding/json"
	"flag"
	"github.com/spf13/viper"
	"log"
	"os"
	"time"
)

const (
	fRPS            = "rps"
	fAttackTime     = "attack"
	fRampupTime     = "ramp"
	fMaxAttackers   = "max"
	fOutput         = "o"
	fVerbose        = "verbose"
	fSample         = "t"
	fRampupStrategy = "s"
	fDoTimeout      = "timeout"
)

var (
	oRPS            = flag.Int(fRPS, 1, "target number of requests per second, must be greater than zero")
	oAttackTime     = flag.Int(fAttackTime, 60, "duration of the attack in seconds")
	oRampupTime     = flag.Int(fRampupTime, 10, "ramp up time in seconds")
	oMaxAttackers   = flag.Int(fMaxAttackers, 10, "maximum concurrent attackers")
	oOutput         = flag.String(fOutput, "", "output file to write the metrics per sample request index (use stdout if empty)")
	oVerbose        = flag.Bool(fVerbose, false, "produce more verbose logging")
	oSample         = flag.Int(fSample, 0, "test your attack implementation with a number of sample calls. Your program exits after this")
	oRampupStrategy = flag.String(fRampupStrategy, defaultRampupStrategy, "set the rampup strategy, possible values are {linear,exp2}")
	oDoTimeout      = flag.Int(fDoTimeout, 5, "timeout in seconds for each attack call")
)

var fullAttackStartedAt time.Time

type SuiteConfig struct {
	RootKeys      string   `mapstructure:"rootkeys"`
	RootRef       string   `mapstructure:"rootref"`
	DumpTransport string   `mapstructure:"dumptransport"`
	HttpTimeout   int      `mapstructure:"http_timeout"`
	Handles       []Config `mapstructure:"handles"`
	ExecutionMode string   `mapstructure:"execution_mode"`
}

// Config holds settings for a Runner.
type Config struct {
	HandleName      string            `mapstructure:"name"`
	RPS             int               `mapstructure:"rps"`
	AttackTimeSec   int               `mapstructure:"attack_time_sec"`
	RampUpTimeSec   int               `mapstructure:"ramp_up_sec"`
	RampUpStrategy  string            `mapstructure:"ramp_up_strategy"`
	MaxAttackers    int               `mapstructure:"max_attackers"`
	OutputFilename  string            `mapstructure:"outputFilename,omitempty"`
	Verbose         bool              `mapstructure:"verbose"`
	Metadata        map[string]string `mapstructure:"metadata,omitempty"`
	DoTimeoutSec    int               `mapstructure:"do_timeout_sec"`
	StoreData       bool              `mapstructure:"store_data"`
	RecycleData     bool              `mapstructure:"recycle_data"`
	ReadFromCsvName string            `mapstructure:"csv_read"`
	WriteToCsvName  string            `mapstructure:"csv_write"`
	HandleParams    map[string]string `mapstructure:"handle_params"`
	SequenceNum     int               `mapstructure:"sequence_num"`
}

// Validate checks all settings and returns a list of strings with problems.
func (c Config) Validate() (list []string) {
	if c.RPS <= 0 {
		list = append(list, "please set the RPS to a positive number of seconds")
	}
	if c.AttackTimeSec < 2 {
		list = append(list, "please set the attack time to a positive number of seconds > 1")
	}
	if c.RampUpTimeSec < 1 {
		list = append(list, "please set the attack time to a positive number of seconds > 0")
	}
	if c.MaxAttackers <= 0 {
		list = append(list, "please set a positive maximum number of attackers")
	}
	if c.DoTimeoutSec <= 0 {
		list = append(list, "please set the Do() timeout to a positive maximum number of seconds")
	}
	return
}

// timeout is in seconds
func (c Config) timeout() time.Duration {
	return time.Duration(c.DoTimeoutSec) * time.Second
}

func (c Config) rampupStrategy() string {
	if len(c.RampUpStrategy) == 0 {
		return defaultRampupStrategy
	}
	return c.RampUpStrategy
}

// ConfigFromFlags creates a Config for use in a Runner.
func ConfigFromFlags() Config {
	flag.Parse()
	return Config{
		RPS:            *oRPS,
		AttackTimeSec:  *oAttackTime,
		RampUpTimeSec:  *oRampupTime,
		RampUpStrategy: *oRampupStrategy,
		Verbose:        *oVerbose,
		MaxAttackers:   *oMaxAttackers,
		OutputFilename: *oOutput,
		Metadata:       map[string]string{},
		DoTimeoutSec:   *oDoTimeout,
	}
}

// ConfigFromFile loads a Config for use in a Runner.
func ConfigFromFile(named string) Config {
	c := ConfigFromFlags() // always parse flags
	f, err := os.Open(named)
	defer f.Close()
	if err != nil {
		log.Fatal("unable to read configuration", err)
	}
	err = json.NewDecoder(f).Decode(&c)
	if err != nil {
		log.Fatal("unable to decode configuration", err)
	}
	applyFlagOverrides(&c)
	return c
}

// override with any flag set
func applyFlagOverrides(c *Config) {
	flag.Visit(func(each *flag.Flag) {
		switch each.Name {
		case fRPS:
			c.RPS = *oRPS
		case fAttackTime:
			c.AttackTimeSec = *oAttackTime
		case fRampupTime:
			c.RampUpTimeSec = *oRampupTime
		case fVerbose:
			c.Verbose = *oVerbose
		case fMaxAttackers:
			c.MaxAttackers = *oMaxAttackers
		case fOutput:
			c.OutputFilename = *oOutput
		case fDoTimeout:
			c.DoTimeoutSec = *oDoTimeout
		}
	})
}

// LoadAttackProfileCfg loads yaml load profile config
func LoadAttackProfileCfg() *SuiteConfig {
	cfgPath := flag.String("config", "", "load attack profile config filepath")
	flag.Parse()
	viper.SetConfigFile(*cfgPath)
	err := viper.ReadInConfig()
	if err != nil {
		log.Fatalf("Failed to readIn viper: %s\n", err)
	}
	var suiteCfg *SuiteConfig
	if err := viper.Unmarshal(&suiteCfg); err != nil {
		log.Fatalf("failed to unmarshal suite config: %s\n", err)
	}
	return suiteCfg
}
