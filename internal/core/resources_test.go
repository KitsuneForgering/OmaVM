package core

import "testing"

func TestHalfMachineResources(t *testing.T) {
	for _, test := range []struct{ hostCPUs, hostRAM, wantCPUs, wantRAM int }{
		{8, 16384, 4, 8192},
		{1, 1024, 1, 512},
		{2, 256, 1, 256},
		{192, 524288, 64, 262144},
	} {
		cpus, ram := halfMachineResources(test.hostCPUs, test.hostRAM)
		if cpus != test.wantCPUs || ram != test.wantRAM {
			t.Errorf("halfMachineResources(%d, %d) = %d, %d; want %d, %d", test.hostCPUs, test.hostRAM, cpus, ram, test.wantCPUs, test.wantRAM)
		}
	}
}
