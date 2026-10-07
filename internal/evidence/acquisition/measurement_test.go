package acquisition

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/alexei-led/archfit/internal/extract/registry"
	"github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/toolrun"
)

func TestMeasurementProfileIgnoresCheckoutAndCounts(t *testing.T) {
	s := &Service{Runner: &toolrun.RunnerMock{RunFunc: func(context.Context, toolrun.ToolCmd) (toolrun.Output, error) {
		return toolrun.Output{Stdout: []byte(`{"GOOS":"linux","GOARCH":"amd64","CGO_ENABLED":"0","GOFLAGS":"","GOEXPERIMENT":"","GO111MODULE":"","GOTOOLCHAIN":"auto","GO386":"","GOAMD64":"v1","GOARM":"","GOARM64":"","GOMIPS":"","GOMIPS64":"","GOPPC64":"","GORISCV64":"","GOWASM":""}`)}, nil
	}}}
	rows := []evidence.Coverage{{Tool: registry.ToolGoPackages, Version: goToolVersion, Status: evidence.StatusOK, FilesSeen: 5}, {Tool: "scip", Status: evidence.StatusDisabled}}
	a := s.measurementProfile(context.Background(), scope.Scope{Root: "/checkout/head"}, rows, nil, nil, nil)
	rows[0].FilesSeen = 40
	rows[0].Reason = "/tmp/base local details"
	rows[0], rows[1] = rows[1], rows[0]
	b := s.measurementProfile(context.Background(), scope.Scope{Root: "/tmp/base"}, rows, nil, nil, nil)
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("checkout/counters changed profile: %+v vs %+v", a, b)
	}
	encoded, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) == 0 || len(a.Unknowns) != 0 {
		t.Fatalf("profile is incomplete: %+v", a)
	}
}

func TestMeasurementProfileBuildEnvironmentUnknown(t *testing.T) {
	s := &Service{Runner: &toolrun.RunnerMock{RunFunc: func(context.Context, toolrun.ToolCmd) (toolrun.Output, error) {
		return toolrun.Output{ExitCode: 1}, nil
	}}}
	p := s.measurementProfile(context.Background(), scope.Scope{Root: t.TempDir()}, []evidence.Coverage{{Tool: registry.ToolGoPackages, Version: goToolVersion, Status: evidence.StatusOK}}, nil, nil, nil)
	if len(p.Unknowns) != 1 {
		t.Fatalf("unknown environment was accepted: %+v", p)
	}
}

func TestMeasurementProfileHistoryWindow(t *testing.T) {
	s := &Service{Runner: &toolrun.RunnerMock{RunFunc: func(context.Context, toolrun.ToolCmd) (toolrun.Output, error) {
		return toolrun.Output{Stdout: []byte("git version 2.49.0")}, nil
	}}}
	h := &evidence.VolatilityCorroboration{Status: evidence.StatusOK, CommitWindow: 500, CommitsScanned: 4}
	a := s.measurementProfile(context.Background(), scope.Scope{}, nil, nil, nil, h)
	h.CommitsScanned = 10
	b := s.measurementProfile(context.Background(), scope.Scope{}, nil, nil, nil, h)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("observed commit count changed measurement identity")
	}
	h.CommitWindow = 0
	h.FullHistory = true
	c := s.measurementProfile(context.Background(), scope.Scope{}, nil, nil, nil, h)
	if !reflect.DeepEqual(a, c) {
		t.Fatal("data-dependent history fallback changed measurement identity")
	}
	d := s.measurementProfile(context.Background(), scope.Scope{}, nil, nil, nil, nil)
	if reflect.DeepEqual(c.Producers, d.Producers) {
		t.Fatal("unavailable history lost producer availability")
	}
}
