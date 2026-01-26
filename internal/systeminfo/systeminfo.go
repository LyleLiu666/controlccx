package systeminfo

import (
	"os"
	"runtime"
	"time"
)

type Info struct {
	Hostname  string    `json:"hostname"`
	OS        string    `json:"os"`
	Arch      string    `json:"arch"`
	GoVersion string    `json:"go_version"`
	Now       time.Time `json:"now"`
}

func Snapshot() Info {
	host, _ := os.Hostname()
	return Info{
		Hostname:  host,
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		GoVersion: runtime.Version(),
		Now:       time.Now().UTC(),
	}
}

