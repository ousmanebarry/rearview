// Package prefilter finds motion events (hard braking, swerves, G-spikes) in
// a trip's GPS track, before any video is analysed, and turns them into
// analysis windows.
package prefilter

import (
	"math"
	"sort"
	"time"

	"rearview/internal/config"
	"rearview/internal/store"
	"rearview/internal/trips"
)

// Detection is one motion event found in a track.
type Detection struct {
	Type  string // hard_brake | swerve | g_spike
	Start time.Time
	End   time.Time
	Peak  time.Time // the worst moment
	Score float64   // 0..1; 0.5 at the threshold, 1 at twice the threshold or more
	Value float64   // the peak measurement: km/h dropped, degrees turned
	Lat   float64
	Lon   float64
	// SpeedKmh is the speed just before the peak.
	SpeedKmh float64
}

// Detect runs every detector over a track sorted by time and merges
// detections of the same type closer than cfg.MergeS.
func Detect(track []store.GPSPoint, cfg config.Motion) []Detection {
	var real []store.GPSPoint
	for _, p := range track {
		if !p.Interpolated {
			real = append(real, p)
		}
	}
	real = dropDropouts(real)
	var all []Detection
	all = append(all, merge(hardBrakes(real, cfg), cfg.MergeS)...)
	all = append(all, merge(swerves(real, cfg), cfg.MergeS)...)
	sort.SliceStable(all, func(i, j int) bool { return all[i].Peak.Before(all[j].Peak) })
	return all
}

func score(value, threshold float64) float64 {
	return math.Min(1, value/(2*threshold))
}

// dropoutMinKmh is the speed above which a run of records with a heading of
// exactly 0° that ends back at speed is a receiver dropout.
const dropoutMinKmh = 10

// dropDropouts removes short runs of records where the receiver reported a
// heading of exactly 0° (usually with speed 0 or a few km/h) and then came
// back at about the same speed, seen at 130 km/h on the 401. Without this
// every dropout looks like a hard brake or a swerve. A run that stays slow is
// a real stop and is kept, except for records of exactly 0 km/h and 0°: the
// receiver keeps its last heading when the car is stopped, so those are
// dropouts too (seen braking 15 km/h -> "0" -> 6 km/h at a red light, which
// read as a 15 km/h/s brake).
func dropDropouts(pts []store.GPSPoint) []store.GPSPoint {
	zero := func(p store.GPSPoint) bool { return p.HeadingDeg == 0 }
	out := pts[:0:0]
	for i := 0; i < len(pts); i++ {
		if zero(pts[i]) && pts[i].SpeedKmh == 0 && i > 0 {
			continue
		}
		if !zero(pts[i]) || i == 0 {
			out = append(out, pts[i])
			continue
		}
		j := i
		for j < len(pts) && zero(pts[j]) {
			j++
		}
		before := pts[i-1].SpeedKmh
		if j < len(pts) && before > dropoutMinKmh && pts[j].SpeedKmh >= before/2 &&
			pts[j].TS.Sub(pts[i-1].TS).Seconds() <= 5 {
			i = j - 1 // skip the run
			continue
		}
		out = append(out, pts[i:j]...)
		i = j - 1
	}
	return out
}

// pairsWithin calls f for every pair of points (j < i) at most window apart.
func pairsWithin(pts []store.GPSPoint, window float64, f func(a, b store.GPSPoint)) {
	for i := 1; i < len(pts); i++ {
		for j := i - 1; j >= 0; j-- {
			dt := pts[i].TS.Sub(pts[j].TS).Seconds()
			if dt > window+1e-9 {
				break
			}
			if dt > 0 {
				f(pts[j], pts[i])
			}
		}
	}
}

func hardBrakes(pts []store.GPSPoint, cfg config.Motion) []Detection {
	var out []Detection
	pairsWithin(pts, cfg.HardBrakeWindowS, func(a, b store.GPSPoint) {
		drop := a.SpeedKmh - b.SpeedKmh
		if drop >= cfg.HardBrakeDropKmh {
			out = append(out, Detection{
				Type: "hard_brake", Start: a.TS, End: b.TS, Peak: b.TS,
				Score: score(drop, cfg.HardBrakeDropKmh), Value: drop, Lat: b.Lat, Lon: b.Lon, SpeedKmh: a.SpeedKmh,
			})
		}
	})
	return out
}

func swerves(pts []store.GPSPoint, cfg config.Motion) []Detection {
	var out []Detection
	pairsWithin(pts, cfg.SwerveWindowS, func(a, b store.GPSPoint) {
		if a.SpeedKmh < cfg.SwerveMinKmh || b.SpeedKmh < cfg.SwerveMinKmh {
			return
		}
		turn := math.Abs(trips.HeadingDelta(a.HeadingDeg, b.HeadingDeg))
		if turn >= cfg.SwerveDeg {
			out = append(out, Detection{
				Type: "swerve", Start: a.TS, End: b.TS, Peak: b.TS,
				Score: score(turn, cfg.SwerveDeg), Value: turn, Lat: b.Lat, Lon: b.Lon, SpeedKmh: a.SpeedKmh,
			})
		}
	})
	return out
}

// merge joins detections (sorted by start, one type) that start within gapS
// of the previous one's end; the merged event keeps the worst peak.
func merge(ds []Detection, gapS float64) []Detection {
	sort.SliceStable(ds, func(i, j int) bool { return ds[i].Start.Before(ds[j].Start) })
	var out []Detection
	for _, d := range ds {
		if n := len(out); n > 0 && d.Start.Sub(out[n-1].End).Seconds() < gapS {
			m := &out[n-1]
			if d.End.After(m.End) {
				m.End = d.End
			}
			if d.Score > m.Score || (d.Score == m.Score && d.Value > m.Value) {
				m.Peak, m.Score, m.Value, m.Lat, m.Lon, m.SpeedKmh = d.Peak, d.Score, d.Value, d.Lat, d.Lon, d.SpeedKmh
			}
			continue
		}
		out = append(out, d)
	}
	return out
}
