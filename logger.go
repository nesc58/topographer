package main

import (
	"net"
	"os"

	"github.com/rs/zerolog"
)

type logger struct {
	lookupLog  zerolog.Logger
	updateLog  zerolog.Logger
	startupLog zerolog.Logger
}

func (l *logger) LookupError(ip net.IP, name string, err error) {
	l.lookupLog.Error().Str("provider", name).Stringer("ip", ip).Err(err).Msg("")
}

func (l *logger) UpdateInfo(name string) {
	l.updateLog.Info().Str("provider", name).Msg("Database was updated")
}

func (l *logger) UpdateError(name string, err error) {
	l.updateLog.Error().Str("provider", name).Err(err).Msg("")
}

func (l *logger) UpdateProgress(name, step string, details map[string]string) {
	event := l.updateLog.Info().Str("provider", name).Str("step", step)
	for key, value := range details {
		event = event.Str(key, value)
	}

	event.Msg("Database update progress")
}

func (l *logger) ServerStarted(address string) {
	l.startupLog.Info().Str("address", address).Msg("Server started successfully")
}

func newLogger() *logger {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	return &logger{
		lookupLog:  zerolog.New(os.Stderr).With().Timestamp().Stack().Str("event_name", "lookup").Logger(),
		updateLog:  zerolog.New(os.Stderr).With().Timestamp().Stack().Str("event_name", "update").Logger(),
		startupLog: zerolog.New(os.Stderr).With().Timestamp().Str("event_name", "startup").Logger(),
	}
}
