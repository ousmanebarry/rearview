package trips

import (
	"encoding/xml"
	"io"
	"time"

	"rearview/internal/store"
)

type gpx struct {
	XMLName xml.Name `xml:"gpx"`
	Version string   `xml:"version,attr"`
	Creator string   `xml:"creator,attr"`
	NS      string   `xml:"xmlns,attr"`
	Trk     gpxTrk   `xml:"trk"`
}

type gpxTrk struct {
	Name string   `xml:"name"`
	Seg  []gpxSeg `xml:"trkseg"`
}

type gpxSeg struct {
	Pts []gpxPt `xml:"trkpt"`
}

type gpxPt struct {
	Lat    float64 `xml:"lat,attr"`
	Lon    float64 `xml:"lon,attr"`
	Time   string  `xml:"time"`
	Speed  float64 `xml:"speed"`  // m/s, GPX 1.0 style; most viewers read it
	Course float64 `xml:"course"` // degrees
}

// WriteGPX writes a track as GPX 1.1. A gap longer than maxGap starts a new
// track segment so viewers don't draw a straight line across it.
func WriteGPX(w io.Writer, name string, track []store.GPSPoint, maxGap time.Duration) error {
	g := gpx{Version: "1.1", Creator: "rearview", NS: "http://www.topografix.com/GPX/1/1", Trk: gpxTrk{Name: name}}
	var cur gpxSeg
	for i, p := range track {
		if i > 0 && p.TS.Sub(track[i-1].TS) > maxGap && len(cur.Pts) > 0 {
			g.Trk.Seg = append(g.Trk.Seg, cur)
			cur = gpxSeg{}
		}
		cur.Pts = append(cur.Pts, gpxPt{Lat: p.Lat, Lon: p.Lon, Time: store.FormatTime(p.TS), Speed: p.SpeedKmh / 3.6, Course: p.HeadingDeg})
	}
	if len(cur.Pts) > 0 {
		g.Trk.Seg = append(g.Trk.Seg, cur)
	}
	if _, err := io.WriteString(w, xml.Header); err != nil {
		return err
	}
	enc := xml.NewEncoder(w)
	enc.Indent("", " ")
	if err := enc.Encode(g); err != nil {
		return err
	}
	_, err := io.WriteString(w, "\n")
	return err
}
