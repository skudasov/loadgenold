package loadgen

import (
	"github.com/spf13/viper"
	"math/rand"
	"sort"
	"time"
)

func timeNow() time.Time {
	return time.Now()
}

func timeHumanReadable(t time.Time) string {
	location, _ := time.LoadLocation(viper.GetString("timezone"))
	return t.In(location).String()
}

func epochNowMillis(t time.Time) int64 {
	return t.UnixNano() / 1000000
}

func SortGroupsBySequenceNum(groups []*Runner) {
	sort.Slice(groups[:], func(i, j int) bool {
		return groups[i].sequence < groups[j].sequence
	})
}

func RandInt() int {
	r1 := rand.New(rand.NewSource(time.Now().UnixNano()))
	return r1.Intn(999999999)
}
