package loadgen

import (
	"os"
)

const (
	GeneratorTmpl = "generator.%s"
)

type attackerFactory func(string) Attack

// CIRun default run mode for suite, with degradation checks
func CIRun(factory attackerFactory) {
	lm := SuiteFromHandles(factory)
	lm.RunSuite()
	lm.CheckDegradation()
	lm.StoreHandleReports()
	if lm.Degradation || lm.Failed {
		os.Exit(1)
	}
}

// FromHandles starts generators for all handles from config
func SuiteFromHandles(factory attackerFactory) *LoadManager {
	suiteCfg := LoadAttackProfileCfg()

	lm := NewLoadManager()
	for _, handleVal := range suiteCfg.Handles {
		lm.Groups = append(lm.Groups, NewRunner(
			handleVal.HandleName,
			lm,
			factory(handleVal.HandleName),
			handleVal),
		)
	}
	return lm
}
