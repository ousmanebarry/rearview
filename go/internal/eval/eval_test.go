package eval

import (
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 23, 17, 0, 0, 0, time.UTC)

func at(id int64, s int, typ string) Item {
	return Item{ID: id, TS: t0.Add(time.Duration(s) * time.Second), Type: typ}
}

func TestMatch(t *testing.T) {
	dets := []Item{at(1, 0, "hard_brake"), at(2, 10, "hard_brake"), at(3, 100, "hard_brake"), at(4, 50, "swerve")}
	labels := []Item{at(11, 2, "hard_brake"), at(12, 13, "hard_brake"), at(13, 14, "hard_brake"), at(14, 50, "hard_brake")}
	r := Match(dets, labels, 3*time.Second)
	if len(r.Matched) != 2 {
		t.Fatalf("matched %v", r.Matched)
	}
	// Detection 2 (t=10) is 3 s from label 12 and 4 s from 13: only 12 counts.
	if r.Matched[1][0].ID != 2 || r.Matched[1][1].ID != 12 {
		t.Errorf("second match %v", r.Matched[1])
	}
	if len(r.FalseAlarms) != 2 || len(r.Misses) != 2 {
		t.Errorf("false alarms %v, misses %v", r.FalseAlarms, r.Misses)
	}
	if r.Precision() != 0.5 || r.Recall() != 0.5 {
		t.Errorf("p=%v r=%v", r.Precision(), r.Recall())
	}
}

func TestOneToOne(t *testing.T) {
	// Two detections near one label: one match, one false alarm.
	r := Match([]Item{at(1, 0, "x"), at(2, 1, "x")}, []Item{at(9, 1, "x")}, 3*time.Second)
	if len(r.Matched) != 1 || r.Matched[0][0].ID != 2 || len(r.FalseAlarms) != 1 {
		t.Errorf("%+v", r)
	}
}

func TestEmpty(t *testing.T) {
	r := Match(nil, nil, time.Second)
	if r.Precision() != 1 || r.Recall() != 1 {
		t.Error("empty sets should score 1")
	}
}
