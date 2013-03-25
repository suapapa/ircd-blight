package ircd

import (
	"reflect"
	"testing"
)

var portRangeTest = []struct {
	Range       string
	ExpectPorts []int
}{
	{
		Range:       "6667",
		ExpectPorts: []int{6667},
	},
	{
		Range:       "6666-6669",
		ExpectPorts: []int{6666, 6667, 6668, 6669},
	},
	{
		Range:       "6666-6669,6697",
		ExpectPorts: []int{6666, 6667, 6668, 6669, 6697},
	},
}

func TestPortRanges(t *testing.T) {
	for idx, test := range portRangeTest {
		directive := &Ports{
			PortString: test.Range,
		}
		ports, err := directive.GetPortList()
		if err != nil {
			t.Errorf("#%d: Error: %s", idx, err)
			continue
		}
		if got, want := len(ports), len(test.ExpectPorts); got != want {
			t.Errorf("#%d: len(ports) = %d, want %d", idx, got, want)
			continue
		}
		for i := 0; i < len(ports); i++ {
			if got, want := ports[i], test.ExpectPorts[i]; got != want {
				t.Errorf("#%d: ports[%d] = %d, want %d", idx, i, got, want)
			}
		}
	}
}
