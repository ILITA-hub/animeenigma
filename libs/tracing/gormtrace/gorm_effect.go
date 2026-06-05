package gormtrace

import (
	"reflect"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/tracing"
	"gorm.io/gorm"
)

// effectStartKey is the per-statement Settings key under which the Before
// callback stashes the operation start time. It rides db.Statement.Settings (a
// sync.Map scoped to the live statement), so each query gets its own start time
// without any shared mutable state across goroutines.
const effectStartKey = "gormtrace:effect_start"

// callback registration names (must be unique within each processor).
const (
	cbAfterCreate = "gormtrace:effect:after_create"
	cbAfterUpdate = "gormtrace:effect:after_update"
	cbAfterDelete = "gormtrace:effect:after_delete"
	cbAfterQuery  = "gormtrace:effect:after_query"
	cbBeforeAny   = "gormtrace:effect:before"
)

// RegisterEffectCallbacks wires the DB-effect recorder onto db. It registers a
// Before-callback (on Create/Update/Delete/Query) that stamps a per-statement
// start time, and After-callbacks that build activity-register effect rows:
//
//   - Create/Update/Delete ALWAYS emit one EffectKind="db_write" row carrying
//     table, op (resolved async), and RowsAffected (D-01).
//   - Query emits EffectKind="db_read" ONLY when gate.ShouldRecord(op, table,
//     durMS) is true — the P95-gated sparse read capture (D-02/D-04). The write
//     path is never gated.
//
// Every emitted Effect sets TargetKind="table", Target=<table>, Requests=1,
// UserID from the private ctx, and carries operation PCs captured cheaply &
// synchronously (resolved on the async Producer side — D-11).
//
// CRITICAL: the after-callbacks issue ZERO DB queries (they only read statement
// fields and call the non-blocking sink). A query inside an after-callback
// re-fires the Query callback AND zeroes db.Statement.RowsAffected (gorm #7044 /
// Pitfall 1 / T-03-06). Keep this function query-free.
//
// Call once after database.New() + InstrumentGORM, before serving traffic.
// RegisterEffectCallbacks MUST NEVER be wired into services/analytics (D-16 —
// the sink's own ingestion would self-amplify; enforced in the plan-06 boot
// wiring, documented here as the consumer constraint / T-03-09).
func RegisterEffectCallbacks(db *gorm.DB, sink tracing.EffectSink, gate ReadGate) error {
	if db == nil || sink == nil || gate == nil {
		return gorm.ErrInvalidValue
	}

	cb := db.Callback()

	// Before: stamp the start time on each statement (write + read).
	before := func(d *gorm.DB) {
		if d.Statement == nil {
			return
		}
		d.Statement.Settings.Store(effectStartKey, time.Now())
	}
	createProc := cb.Create()
	updateProc := cb.Update()
	deleteProc := cb.Delete()
	queryProc := cb.Query()

	if err := createProc.Before("gorm:create").Register(cbBeforeAny+":create", before); err != nil {
		return err
	}
	if err := updateProc.Before("gorm:update").Register(cbBeforeAny+":update", before); err != nil {
		return err
	}
	if err := deleteProc.Before("gorm:delete").Register(cbBeforeAny+":delete", before); err != nil {
		return err
	}
	if err := queryProc.Before("gorm:query").Register(cbBeforeAny+":query", before); err != nil {
		return err
	}

	// After (writes): always fact-row db_write.
	writeAfter := func(d *gorm.DB) { recordWrite(d, sink) }
	if err := createProc.After("gorm:create").Register(cbAfterCreate, writeAfter); err != nil {
		return err
	}
	if err := updateProc.After("gorm:update").Register(cbAfterUpdate, writeAfter); err != nil {
		return err
	}
	if err := deleteProc.After("gorm:delete").Register(cbAfterDelete, writeAfter); err != nil {
		return err
	}

	// After (reads): P95-gated db_read.
	if err := queryProc.After("gorm:query").Register(cbAfterQuery, func(d *gorm.DB) {
		recordRead(d, sink, gate)
	}); err != nil {
		return err
	}

	return nil
}

// recordWrite builds and emits a db_write effect from a completed
// Create/Update/Delete statement. It issues NO DB query (Pitfall 1).
func recordWrite(d *gorm.DB, sink tracing.EffectSink) {
	if d.Statement == nil {
		return
	}
	table := d.Statement.Table
	if table == "" {
		return
	}
	rows := int(d.Statement.RowsAffected)
	ctx := d.Statement.Context
	op := tracing.CaptureOperationPCs(ctx)
	// EffectKind is set explicitly to "db_write" (never defaulted) so analytics
	// does not fall back to "egress".
	e := tracing.Effect{
		EffectKind: "db_write",
		Target:     table,
		TargetKind: "table",
		Rows:       rows,
		Requests:   1,
		DurationMS: durationMS(d),
		UserID:     tracing.UserIDFromContext(ctx),
	}
	sink.Record(e.WithOperationPCs(op))
}

// recordRead builds and emits a db_read effect for a completed SELECT, but only
// when the read exceeds its per-(operation, table) P95 (the gate). It issues NO
// DB query (Pitfall 1).
func recordRead(d *gorm.DB, sink tracing.EffectSink, gate ReadGate) {
	if d.Statement == nil {
		return
	}
	table := d.Statement.Table
	if table == "" {
		return
	}

	durMS := durationMS(d)
	op := tracing.CaptureOperationPCs(d.Statement.Context)
	// Resolve a coarse op label for the gate lookup. The gate keys on the same
	// (operation, table) the analytics P95 producer aggregates by.
	operation := op.Resolve()
	if !gate.ShouldRecord(operation, table, durMS) {
		return
	}

	// A6: on a SELECT, RowsAffected may read 0 even though rows were scanned —
	// fall back to the reflect-len of Dest (the slice/struct being populated).
	rows := int(d.Statement.RowsAffected)
	if rows == 0 {
		rows = destLen(d.Statement.Dest)
	}

	e := tracing.Effect{
		EffectKind: "db_read",
		Target:     table,
		TargetKind: "table",
		Rows:       rows,
		Requests:   1,
		DurationMS: durMS,
		Operation:  operation,
		UserID:     tracing.UserIDFromContext(d.Statement.Context),
	}
	// Carry the captured PCs so the Producer can resolve the fine operation async
	// (it falls back to the coarse Operation we set above).
	e = e.WithOperationPCs(op)
	sink.Record(e)
}

// durationMS reads the start time stamped by the Before callback and returns the
// elapsed milliseconds. Returns 0 when no start time is present (e.g. a raw
// statement that bypassed the Before hook).
func durationMS(d *gorm.DB) int {
	if d.Statement == nil {
		return 0
	}
	if v, ok := d.Statement.Settings.Load(effectStartKey); ok {
		if start, ok := v.(time.Time); ok {
			return int(time.Since(start).Milliseconds())
		}
	}
	return 0
}

// destLen returns the number of result rows reflected in a SELECT Dest: the
// length of a slice/array Dest, or 1 for a single non-nil struct/pointer Dest.
// Used as the A6 fallback when RowsAffected reads 0 on a SELECT. It only
// inspects the already-populated Dest — it issues no DB query.
func destLen(dest interface{}) int {
	if dest == nil {
		return 0
	}
	v := reflect.ValueOf(dest)
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return 0
		}
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.Slice, reflect.Array:
		return v.Len()
	case reflect.Struct, reflect.Map:
		return 1
	default:
		return 0
	}
}
