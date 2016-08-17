/*
Waituntil is a command that waits (sleeps) until some specific time,
or technically until it is that time or later. It makes some attempt
to deal with the system time shifting out from underneath it, but it
doesn't try too hard here.

usage: waituntil [-v] <WHEN>

-v reports the target time waituntil will (try to) wait for.

<WHEN> has two forms. The simple form is HH:MM[:SS], with HH in 24
hour time. If HH:MM is in the past, waituntil assumes that you mean
that time tomorrow.

The full form is YYYY-MM-DD HH:MM[:SS]. You can omit YYYY and MM to
mean the current year and month, and you can omit the time of day (in
which case it's taken as midnight).  If this time is in the past,
waituntil exits immediately.

Author: Chris Siebenmann
https://github.com/siebenmann/waituntil

Copyright: GPL v3
*/
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

// What our time specifications are missing, if anything.
// We don't mention missing hours, minutes, or seconds, because
// we take time.ParseInLocation()'s default zero values for those
// when parsing full time specifications.
//
// (Note that this means we can't use this code to parse 'HH:MM'
// alone, since that's specified to wrap into tomorrow. That would
// require a new marker.)
//
// The order matters here; things later imply everything before them.
const (
	full = iota
	noyear
	nomonth
)

type tSpec struct {
	spec  string
	lacks int
}

// The various time specifications that our full parsing accepts.
var specs = []tSpec{
	{"2006-01-02 15:04", full},
	{"2006-01-02 15:04:05", full},
	{"2006-01-02", full},
	{"01-02 15:04", noyear},
	{"01-02 15:04:05", noyear},
	{"01-02", noyear},
	{"02 15:04", nomonth},
	{"02 15:04:05", nomonth},
	{"02", nomonth},
}

// Parse the simple HH:MM[:SS] time specification. This implements
// rolling over a time in the past into tomorrow.
func parseHHMM(tspec string) (time.Time, error) {
	var hr, min, secs int
	var tgt time.Time
	_, e := fmt.Sscanf(tspec, "%d:%d:%d", &hr, &min, &secs)
	if e != nil {
		secs = 0
		_, e = fmt.Sscanf(tspec, "%d:%d", &hr, &min)
		if e != nil {
			return tgt, e
		}
	}
	// Get the current year, month, day, and location, and create a new
	// time from it using our hours, minutes, and seconds. There is
	// probably an easier way to do this.
	now := time.Now()
	tgt = time.Date(now.Year(), now.Month(), now.Day(), hr, min, secs, 0, now.Location())

	// Before we do anything else: if our target time is right now,
	// we're done. We accept times that are this minute and with the
	// target seconds being before now, too, so that '17:01' is
	// still considered 'right now' at 17:01:33. (And in general
	// if you say '17:01:30' and hit return at 17:01:35, you
	// probably don't mean tomorrow. This is arguable.)
	if now.Hour() == hr && now.Minute() == min && now.Second() >= secs {
		return tgt, nil
	}

	// If the target time we've determined is before now, it's actually
	// tomorrow. Push it forward.
	if tgt.Before(now) {
		tgt = tgt.Add(time.Hour * 24)
	}
	return tgt, nil
}

// Parse time specification. First we try the simple HH:MM[:SS] parse,
// and then we fall back to the big hammer.
func parseTime(tspec string) (time.Time, error) {
	t, e := parseHHMM(tspec)
	if e == nil {
		return t, e
	}

	// This isn't HH:MM[:SS], so we run it through our collection of
	// time specifications in the hope that something will hit.
	now := time.Now()
	for _, spec := range specs {
		t, e = time.ParseInLocation(spec.spec, tspec, time.Local)
		if e != nil {
			continue
		}
		if spec.lacks >= noyear {
			t = t.AddDate(now.Year(), 0, 0)
		}
		if spec.lacks >= nomonth {
			t = t.AddDate(0, int(now.Month())-1, 0)
		}
		return t, e
	}
	return t, errors.New("cannot parse time argument")
}

func main() {
	var tstr string
	var verbose = flag.Bool("v", false, "be verbose about when we're waiting for")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "waituntil [-v] HH:MM[:SS]\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() == 0 {
		fmt.Fprintf(os.Stderr, "%s: no target time given\n", os.Args[0])
		return
	}

	tstr = strings.Join(flag.Args(), " ")
	tgt, e := parseTime(tstr)
	if e != nil {
		fmt.Fprintf(os.Stderr, "%s: cannot parse target time\n", os.Args[0])
		return
	}

	if *verbose {
		fmt.Printf("until %s\n", tgt)
	}

	// Now we start looping looking to sleep until our target time.
	// We don't wait for the full duration all at once, because that
	// could go badly wrong if the clock is adjusted. Instead we only
	// wait for so long at a time, and re-check things every time we
	// come out of a sleep.
	for {
		now := time.Now()
		if now.After(tgt) {
			return
		}
		dur := tgt.Sub(now)

		// we have a one-second granularity; if we're closer
		// than that, we're done.
		if dur < time.Second {
			return
		}

		// If our target time is within a minute, we sleep for
		// exactly that long on the assumption that clock changes
		// over that short a time are unimportant. Otherwise, we
		// sleep for half the time or an hour, whichever is
		// smaller.
		switch {
		case dur > (2 * time.Hour):
			dur = time.Hour
		case dur > time.Minute:
			dur = dur / 2
		}

		time.Sleep(dur)
	}
}
