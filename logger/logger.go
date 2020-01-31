package logger

import (
	"context"

	"github.com/mixpanel/obs"
	"go.uber.org/zap"
)

type Logger interface {
	Info(v ...interface{})
	Warn(v ...interface{})
	ScopeName(name string) Logger
}

type defaultLogger struct {
	*zap.SugaredLogger
}

func (d defaultLogger) ScopeName(name string) Logger {
	d.SugaredLogger = d.SugaredLogger.Named(name)
	return d
}

var DefaultLogger Logger

func init() {
	l, err := zap.NewProduction()
	if err != nil {
		panic(err)
	}
	DefaultLogger = defaultLogger{
		SugaredLogger: l.Sugar(),
	}
}

type NullLogger struct {
}

func (n NullLogger) Info(v ...interface{}) {
}

func (n NullLogger) Warn(v ...interface{}) {
}

func (n NullLogger) ScopeName(name string) Logger {
	return n
}

type ObsLogger struct {
	fr obs.FlightRecorder
	fs obs.FlightSpan
}

func NewObsLogger(fr obs.FlightRecorder) *ObsLogger {
	return &ObsLogger{
		fr: fr,
		fs: fr.WithSpan(context.Background()),
	}
}

func obsVals(v ...interface{}) obs.Vals {
	vals := obs.Vals{}
	if len(v)%2 != 0 {
		return vals
	}
	for i := 0; i < len(v); i += 2 {
		val, ok := v[i].(string)
		if !ok {
			continue
		}
		vals[val] = v[i+1]
	}
	return vals
}

func (o *ObsLogger) Info(v ...interface{}) {
	if len(v) == 0 {
		return
	}
	_, ok := v[0].(string)
	if !ok {
		o.fs.Info("", obs.Vals{"log_vals": v})
		return
	}
	o.fs.Info(v[0].(string), obsVals(v[1:]...))
}

func (o *ObsLogger) Warn(v ...interface{}) {
	if len(v) == 0 {
		return
	}
	_, ok := v[0].(string)
	if !ok {
		o.fs.Info("", obs.Vals{"log_vals": v})
		return
	}
	o.fs.Warn("", v[0].(string), obsVals(v[1:]...))
}

func (o *ObsLogger) ScopeName(name string) Logger {
	return NewObsLogger(o.fr.ScopeName(name))
}
