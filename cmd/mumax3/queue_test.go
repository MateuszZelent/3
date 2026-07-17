package main

import (
	"reflect"
	"testing"
)

func TestSelectQueueGPUs(t *testing.T) {
	tests := []struct {
		name        string
		deviceCount int
		explicitGPU bool
		gpu         int
		maxGPUs     int
		want        []int
		wantErr     bool
	}{
		{name: "all", deviceCount: 4, want: []int{0, 1, 2, 3}},
		{name: "limited", deviceCount: 4, maxGPUs: 2, want: []int{0, 1}},
		{name: "limit above available", deviceCount: 2, maxGPUs: 8, want: []int{0, 1}},
		{name: "explicit GPU", deviceCount: 4, explicitGPU: true, gpu: 3, maxGPUs: 2, want: []int{3}},
		{name: "negative limit", deviceCount: 4, maxGPUs: -1, wantErr: true},
		{name: "invalid explicit GPU", deviceCount: 2, explicitGPU: true, gpu: 2, wantErr: true},
		{name: "no GPUs", deviceCount: 0, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := selectQueueGPUs(tt.deviceCount, tt.explicitGPU, tt.gpu, tt.maxGPUs)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr = %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("GPU IDs = %v, want %v", got, tt.want)
			}
		})
	}
}
