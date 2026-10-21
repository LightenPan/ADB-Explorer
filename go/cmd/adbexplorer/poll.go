package main

import (
	"context"
	"sync"
	"time"
)

var pollOnce sync.Once

var pollStop = make(chan struct{})

func (a *app) startPoll() {
	pollOnce.Do(func() { go a.pollDevices() })
}

func stopPoll() {
	select {
	case <-pollStop:
	default:
		close(pollStop)
	}
}

// pollDevices refreshes the device list until the window is gone and the
// loop is stopped. The explorer refresh stays manual so a poll never cancels
// a listing.
func (a *app) pollDevices() {
	for {
		secs := a.pollSeconds()
		off := secs == 0
		if secs < 1 {
			secs = 3
		}
		if off {
			secs = 5
		}
		timer := time.NewTimer(time.Duration(secs) * time.Second)
		select {
		case <-pollStop:
			timer.Stop()
			return
		case <-timer.C:
		}
		if off {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		list, err := a.client().Devices(ctx)
		cancel()
		a.winUpdate(func() {
			if err != nil {
				a.connErr = err.Error()
				return
			}
			a.devices = list
			a.connErr = ""
			if !a.hasDevice(a.serial) {
				next := ""
				for _, d := range list {
					if d.Ready() {
						next = d.Serial
						break
					}
				}
				a.setSerial(next)
			}
		})
	}
}
