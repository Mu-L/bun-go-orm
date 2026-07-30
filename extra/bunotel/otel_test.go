package bunotel

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect"
	"github.com/uptrace/bun/dialect/feature"
	"github.com/uptrace/bun/schema"
)

// otelsqlInstrumName is the instrumentation name otelsql uses for the meter it
// creates in ReportDBStatsMetrics.
const otelsqlInstrumName = "github.com/uptrace/opentelemetry-go-extra/otelsql"

// TestQueryHookInitUsesConfiguredMeterProvider ensures that DB stats metrics are
// reported to the meter provider passed to WithMeterProvider instead of the
// global one. See https://github.com/uptrace/bun/issues/1270.
func TestQueryHookInitUsesConfiguredMeterProvider(t *testing.T) {
	global := setGlobalMeterProvider(t)

	configured := new(recordingMeterProvider)
	NewQueryHook(WithMeterProvider(configured)).Init(newTestDB(t))

	if !configured.hasMeter(otelsqlInstrumName) {
		t.Error("DB stats metrics were not reported to the configured meter provider")
	}
	if global.hasMeter(otelsqlInstrumName) {
		t.Error("DB stats metrics were reported to the global meter provider")
	}
}

// TestQueryHookInitFallsBackToGlobalMeterProvider ensures that hooks created
// without WithMeterProvider keep reporting to the global meter provider.
func TestQueryHookInitFallsBackToGlobalMeterProvider(t *testing.T) {
	global := setGlobalMeterProvider(t)

	NewQueryHook().Init(newTestDB(t))

	if !global.hasMeter(otelsqlInstrumName) {
		t.Error("DB stats metrics were not reported to the global meter provider")
	}
}

// setGlobalMeterProvider installs a recording meter provider as the global one
// and restores the previous provider when the test finishes.
func setGlobalMeterProvider(t *testing.T) *recordingMeterProvider {
	t.Helper()

	prev := otel.GetMeterProvider()
	t.Cleanup(func() { otel.SetMeterProvider(prev) })

	mp := new(recordingMeterProvider)
	otel.SetMeterProvider(mp)
	return mp
}

// recordingMeterProvider is a no-op meter provider that remembers which
// instrumentation names asked it for a meter.
type recordingMeterProvider struct {
	noop.MeterProvider

	names []string
}

func (p *recordingMeterProvider) Meter(name string, opts ...metric.MeterOption) metric.Meter {
	p.names = append(p.names, name)
	return p.MeterProvider.Meter(name, opts...)
}

func (p *recordingMeterProvider) hasMeter(name string) bool {
	for _, n := range p.names {
		if n == name {
			return true
		}
	}
	return false
}

// newTestDB returns a bun.DB that never connects to a database: reporting DB
// stats only requires sql.DB.Stats.
func newTestDB(t *testing.T) *bun.DB {
	t.Helper()

	db := bun.NewDB(sql.OpenDB(nopConnector{}), newNopDialect())
	t.Cleanup(func() { _ = db.Close() })
	return db
}

var errNotImplemented = errors.New("bunotel: not implemented")

type nopConnector struct{}

func (nopConnector) Connect(context.Context) (driver.Conn, error) { return nil, errNotImplemented }

func (c nopConnector) Driver() driver.Driver { return nopDriver{} }

type nopDriver struct{}

func (nopDriver) Open(string) (driver.Conn, error) { return nil, errNotImplemented }

type nopDialect struct {
	schema.BaseDialect

	tables *schema.Tables
}

var _ schema.Dialect = (*nopDialect)(nil)

func newNopDialect() *nopDialect {
	d := new(nopDialect)
	d.tables = schema.NewTables(d)
	return d
}

func (*nopDialect) Init(*sql.DB) {}

func (*nopDialect) Name() dialect.Name { return dialect.SQLite }

func (*nopDialect) Features() feature.Feature { return 0 }

func (d *nopDialect) Tables() *schema.Tables { return d.tables }

func (*nopDialect) OnTable(*schema.Table) {}

func (*nopDialect) IdentQuote() byte { return '"' }

func (*nopDialect) AppendSequence(b []byte, _ *schema.Table, _ *schema.Field) []byte { return b }

func (*nopDialect) DefaultVarcharLen() int { return 0 }

func (*nopDialect) DefaultSchema() string { return "" }
