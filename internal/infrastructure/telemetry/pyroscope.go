package telemetry

import (
	"fmt"

	"github.com/grafana/pyroscope-go"
	log "github.com/sirupsen/logrus"
)

const fulmine = "fulmine"

// InitPyroscope initializes the Pyroscope profiler for continuous profiling.
// It returns a shutdown function that should be called on application exit.
// If pyroscopeServerURL is empty, this function does nothing and returns a no-op shutdown function.
func InitPyroscope(pyroscopeServerURL string) (func(), error) {
	if pyroscopeServerURL == "" {
		return nil, nil
	}

	profiler, err := pyroscope.Start(pyroscope.Config{
		ApplicationName: fulmine,
		ServerAddress:   pyroscopeServerURL,
		Logger:          nil,
		ProfileTypes: []pyroscope.ProfileType{
			pyroscope.ProfileCPU,
			pyroscope.ProfileInuseObjects,
			pyroscope.ProfileAllocObjects,
			pyroscope.ProfileInuseSpace,
			pyroscope.ProfileAllocSpace,
			pyroscope.ProfileGoroutines,
			pyroscope.ProfileMutexCount,
			pyroscope.ProfileMutexDuration,
			pyroscope.ProfileBlockCount,
			pyroscope.ProfileBlockDuration,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start pyroscope profiler: %s", err)
	}

	log.WithFields(log.Fields{
		"server":  pyroscopeServerURL,
		"service": fulmine,
	}).Info("pyroscope profiler started successfully")

	shutdown := func() {
		if err := profiler.Stop(); err != nil {
			log.WithError(err).Warn("failed to stop pyroscope profiler")
			return
		}
		log.Debug("pyroscope profiler shutdown")
	}

	log.Debug("pyroscope profiler initialized")

	return shutdown, nil
}
