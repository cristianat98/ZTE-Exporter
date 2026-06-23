package main

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestBuildInfoCollector(t *testing.T) {
	c := buildInfoCollector()
	if c == nil {
		t.Fatal("expected a non-nil collector")
	}

	descCh := make(chan *prometheus.Desc, 1)
	c.Describe(descCh)
	close(descCh)

	count := 0
	for range descCh {
		count++
	}
	if count != 1 {
		t.Errorf("expected 1 described metric, got %d", count)
	}
}
