package flow

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"yingce/backend/internal/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type commandEngineFunc func() (int, []byte, error)

func (f commandEngineFunc) Do(context.Context, Scope, Actor, string, string, string, []byte) (int, []byte, error) {
	return f()
}

func enqueueTestCommand(t *testing.T, d CommandDispatcher) model.FlowCommandOutbox {
	t.Helper()
	scope, err := ResolveScope(d.DB, "u_alice", "")
	if err != nil {
		t.Fatal(err)
	}
	fund(t, Billing{DB: d.DB}, "u_alice", 50_000)
	row, _, err := d.Enqueue(context.Background(), CommandInput{
		Scope: scope, Actor: Actor{Type: "user", ID: "u_alice"},
		Command: "collection.request", Method: http.MethodPost, Path: "/v1/collection-runs",
		IdempotencyKey: "outbox-regression", Payload: []byte(`{"source_ids":["src_1"]}`),
		ReserveMicro: 1_000, MaxAttempts: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func TestCommandOutboxCompetingDispatchers(t *testing.T) {
	db := testDB(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	d := CommandDispatcher{DB: db, Engine: commandEngineFunc(func() (int, []byte, error) {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		}
		return 202, []byte(`{"operation_id":"engine_op"}`), nil
	})}
	row := enqueueTestCommand(t, d)
	done := make(chan error, 1)
	go func() { _, err := d.DispatchOne(context.Background(), row.ID); done <- err }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("first dispatcher never entered Engine")
	}
	other, err := d.DispatchOne(context.Background(), row.ID)
	close(release)
	firstErr := <-done
	if err != nil || firstErr != nil || calls.Load() != 1 || other.Status != "dispatching" {
		t.Fatalf("only lease holder may send: calls=%d other=%s errors=%v/%v", calls.Load(), other.Status, err, firstErr)
	}
}

func TestCommandOutboxConcurrentClaimFromSameVersion(t *testing.T) {
	db := testDB(t)
	var calls, reads atomic.Int32
	d := CommandDispatcher{DB: db, Engine: commandEngineFunc(func() (int, []byte, error) { calls.Add(1); return 202, nil, nil })}
	row := enqueueTestCommand(t, d)
	read, release := make(chan struct{}, 2), make(chan struct{})
	if err := db.Callback().Query().After("gorm:query").Register("hold_initial_reads", func(tx *gorm.DB) {
		if tx.Statement.Table == "flow_command_outbox" && reads.Add(1) <= 2 {
			read <- struct{}{}
			<-release
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Callback().Query().Remove("hold_initial_reads") })
	done := make(chan error, 2)
	for range 2 {
		go func() { _, err := d.DispatchOne(context.Background(), row.ID); done <- err }()
	}
	for range 2 {
		select {
		case <-read:
		case <-time.After(5 * time.Second):
			close(release)
			t.Fatal("dispatchers did not read the same initial version")
		}
	}
	close(release)
	for range 2 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("same-version claims sent %d times", calls.Load())
	}
}

func TestCommandOutboxLateReceiptCannotOverwrite(t *testing.T) {
	for _, status := range []int{202, 422, 503} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			db := testDB(t)
			d := CommandDispatcher{DB: db}
			row := enqueueTestCommand(t, d)
			d.Engine = commandEngineFunc(func() (int, []byte, error) {
				// A newer owner/result became durable while this external call was in flight.
				if err := db.Model(&model.FlowCommandOutbox{}).Where("id = ?", row.ID).
					Updates(map[string]any{"attempts": 2, "status": "dispatched", "response_status": 202, "response_body": "new receipt"}).Error; err != nil {
					t.Fatal(err)
				}
				return status, []byte(`{"operation_id":"old_op"}`), nil
			})
			_, _ = d.DispatchOne(context.Background(), row.ID)
			var stored model.FlowCommandOutbox
			if err := db.First(&stored, "id = ?", row.ID).Error; err != nil {
				t.Fatal(err)
			}
			var reservation model.FlowBudgetReservation
			db.First(&reservation, "id = ?", row.ReservationID)
			var projection model.FlowOperationProjection
			db.First(&projection, "operation_id = ?", row.OperationID)
			if stored.ResponseBody != "new receipt" || stored.Status != "dispatched" || reservation.Status != "reserved" || projection.Status != "queued" {
				t.Fatalf("stale writer changed durable result: outbox=%s/%s reservation=%s projection=%s", stored.Status, stored.ResponseBody, reservation.Status, projection.Status)
			}
		})
	}
}

func TestCommandOutboxPersistenceFailures(t *testing.T) {
	for _, tc := range []struct {
		name, trigger string
		status        int
	}{
		{"receipt", "BEFORE UPDATE ON flow_command_outbox WHEN NEW.status = 'dispatched'", 202},
		{"projection_delete", "BEFORE DELETE ON flow_operation_projections", 202},
		{"projection_create", "BEFORE INSERT ON flow_operation_projections", 202},
		{"projection_update", "BEFORE UPDATE ON flow_operation_projections", 422},
		{"release", "BEFORE UPDATE ON flow_budget_reservations WHEN NEW.status = 'released'", 422},
		{"failed_receipt", "BEFORE UPDATE ON flow_command_outbox WHEN NEW.status = 'failed'", 422},
		{"unknown", "BEFORE UPDATE ON flow_budget_reservations WHEN NEW.status = 'reconciliation_required'", 503},
		{"retry_receipt", "BEFORE UPDATE ON flow_command_outbox WHEN NEW.status = 'reconciliation_required'", 503},
		{"retry_wait_receipt", "BEFORE UPDATE ON flow_command_outbox WHEN NEW.status = 'retry_wait'", 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := testDB(t)
			var calls int
			d := CommandDispatcher{DB: db, Engine: commandEngineFunc(func() (int, []byte, error) {
				calls++
				return tc.status, []byte(`{"operation_id":"engine_op"}`), nil
			})}
			row := enqueueTestCommand(t, d)
			if tc.name == "retry_wait_receipt" {
				if err := db.Model(&row).Update("max_attempts", 2).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := db.Exec("CREATE TRIGGER fail_write " + tc.trigger + " BEGIN SELECT RAISE(ABORT, 'injected persistence failure'); END").Error; err != nil {
				t.Fatal(err)
			}
			_, err := d.DispatchOne(context.Background(), row.ID)
			if err == nil || !strings.Contains(err.Error(), "injected persistence failure") {
				t.Errorf("database failure must propagate, got %v", err)
			}
			var stored model.FlowCommandOutbox
			db.First(&stored, "id = ?", row.ID)
			var reservation model.FlowBudgetReservation
			db.First(&reservation, "id = ?", row.ReservationID)
			if stored.Status != "dispatching" || reservation.Status != "reserved" {
				t.Errorf("partial terminal transaction: outbox=%s reservation=%s", stored.Status, reservation.Status)
			}
			if err := db.Exec("DROP TRIGGER fail_write").Error; err != nil {
				t.Fatal(err)
			}
			_, _ = d.DispatchOne(context.Background(), row.ID)
			if calls != 1 {
				t.Errorf("unknown outcome resent: calls=%d", calls)
			}
			var account model.CreditAccount
			if err := db.First(&account, "user_id = ?", "u_alice").Error; err != nil {
				t.Fatal(err)
			}
			if account.ReservedMicrocredits != 1_000 || account.AvailableMicrocredits != 49_000 {
				t.Errorf("held funds changed after failure/replay: reserved=%d available=%d", account.ReservedMicrocredits, account.AvailableMicrocredits)
			}
		})
	}
}

func TestCommandOutboxExpiredLeaseReconcilesAtomically(t *testing.T) {
	for _, due := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "background"}[due], func(t *testing.T) {
			db := testDB(t)
			var calls int
			d := CommandDispatcher{DB: db, Engine: commandEngineFunc(func() (int, []byte, error) { calls++; return 202, nil, nil })}
			row := enqueueTestCommand(t, d)
			if err := db.Model(&row).Updates(map[string]any{"status": "dispatching", "attempts": 1, "lease_until": time.Now().UTC().Add(-time.Minute)}).Error; err != nil {
				t.Fatal(err)
			}
			if due {
				_, _ = d.DispatchDue(context.Background(), 1)
			} else {
				_, _ = d.DispatchOne(context.Background(), row.ID)
			}
			var stored model.FlowCommandOutbox
			db.First(&stored, "id = ?", row.ID)
			var reservation model.FlowBudgetReservation
			db.First(&reservation, "id = ?", row.ReservationID)
			var projection model.FlowOperationProjection
			db.First(&projection, "operation_id = ?", row.OperationID)
			if calls != 0 || stored.Status != StatusReconciliation || reservation.Status != StatusReconciliation || projection.Status != StatusReconciliation {
				t.Fatalf("expired lease must reconcile without resend: calls=%d states=%s/%s/%s", calls, stored.Status, reservation.Status, projection.Status)
			}
		})
	}
}

func TestCommandOutboxCallersPreserveReadFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, caller := range []string{"REST", "Agent"} {
		t.Run(caller, func(t *testing.T) {
			tools, _, _, conv, ss := setupAgent(t)
			var afterResponse atomic.Bool
			if err := tools.DB.Callback().Query().After("gorm:query").Register("fail_final_read", func(tx *gorm.DB) {
				if afterResponse.Load() && tx.Statement.Table == "flow_command_outbox" {
					tx.AddError(errors.New("injected final read failure"))
				}
			}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { tools.DB.Callback().Query().Remove("fail_final_read") })
			if caller == "Agent" {
				tools.Engine = commandEngineFunc(func() (int, []byte, error) {
					afterResponse.Store(true)
					return 202, []byte(`{"operation_id":"accepted_op"}`), nil
				})
				result, err := tools.Invoke(context.Background(), conv, ss, "u_alice", "read_fail", "intel_collect_once", map[string]any{"source_ids": []any{"src_1"}}, "")
				if err == nil && result.Status != "failed" {
					t.Fatalf("Agent hid persistence error: %+v", result)
				}
				return
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				afterResponse.Store(true)
				w.WriteHeader(202)
				_, _ = w.Write([]byte(`{"operation_id":"accepted_op"}`))
			}))
			t.Cleanup(srv.Close)
			cfg := Config{EngineURL: srv.URL, SigningKey: key, Issuer: "flow-platform", Audience: "loom-engine", TTL: MaxTokenTTL}
			r := gin.New()
			RegisterRoutes(r.Group("/api"), tools.DB, cfg, func(*gin.Context) (*model.User, error) { return &model.User{ID: "u_alice"}, nil })
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/intelligence/collection-runs", strings.NewReader(`{"source_ids":["src_1"]}`)))
			if w.Code < 500 {
				t.Fatalf("REST hid persistence error: %d %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestCommandOutboxDispatchDuePropagatesFailure(t *testing.T) {
	db := testDB(t)
	d := CommandDispatcher{DB: db, Engine: commandEngineFunc(func() (int, []byte, error) { return 202, nil, nil })}
	enqueueTestCommand(t, d)
	if err := db.Exec("CREATE TRIGGER fail_write BEFORE UPDATE ON flow_command_outbox WHEN NEW.status = 'dispatched' BEGIN SELECT RAISE(ABORT, 'injected background failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	if n, err := d.DispatchDue(context.Background(), 1); n != 0 || err == nil || !strings.Contains(err.Error(), "injected background failure") {
		t.Fatalf("background failure hidden: n=%d err=%v", n, err)
	}
}

func TestCommandOutboxDefersUntilOuterTransactionCommits(t *testing.T) {
	db := testDB(t)
	d := CommandDispatcher{DB: db}
	row := enqueueTestCommand(t, d)
	var calls int
	d.Engine = commandEngineFunc(func() (int, []byte, error) { calls++; return 202, nil, nil })
	// REST TenantMiddleware and background Agent UserTx both pass an open transaction.
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	d.DB = tx
	_, err := d.DispatchOne(context.Background(), row.ID)
	if rollbackErr := tx.Rollback().Error; rollbackErr != nil {
		t.Fatal(rollbackErr)
	}
	if err != nil || calls != 0 {
		t.Fatalf("external effect before outer commit: calls=%d err=%v", calls, err)
	}
	d.DB = db
	if _, err := d.DispatchOne(context.Background(), row.ID); err != nil || calls != 1 {
		t.Fatalf("committed background dispatch: calls=%d err=%v", calls, err)
	}
}

func TestCommandOutboxGeneratedKeySurvivesLostACK(t *testing.T) {
	db := testDB(t)
	keys := make(chan string, 2)
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		keys <- r.Header.Get("X-Idempotency-Key")
		if calls.Add(1) == 1 {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(202)
		_, _ = w.Write([]byte(`{"operation_id":"stable_op"}`))
	}))
	t.Cleanup(srv.Close)
	now := time.Now().UTC()
	d := CommandDispatcher{DB: db, Now: func() time.Time { return now }, Engine: EngineClient{Cfg: Config{EngineURL: srv.URL, SigningKey: key, Issuer: "flow-platform", Audience: "loom-engine", TTL: MaxTokenTTL}}}
	scope, err := ResolveScope(db, "u_alice", "")
	if err != nil {
		t.Fatal(err)
	}
	row, _, err := d.Enqueue(context.Background(), CommandInput{Scope: scope, Actor: Actor{Type: "user", ID: "u_alice"}, Command: "collection.request", Method: http.MethodPost, Path: "/v1/collection-runs", Payload: []byte(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = d.DispatchOne(context.Background(), row.ID)
	now = now.Add(time.Minute)
	if _, err := d.DispatchOne(context.Background(), row.ID); err != nil {
		t.Fatal(err)
	}
	firstKey, secondKey := <-keys, <-keys
	if firstKey == "" || firstKey != secondKey || firstKey != row.IdempotencyKey || calls.Load() != 2 {
		t.Fatalf("lost-ACK retries require stable generated key: key=%q calls=%d", firstKey, calls.Load())
	}
}

func TestCommandOutboxFinalReadFailure(t *testing.T) {
	db := testDB(t)
	d := CommandDispatcher{DB: db}
	row := enqueueTestCommand(t, d)
	readErr := errors.New("injected receipt read failure")
	d.Engine = commandEngineFunc(func() (int, []byte, error) {
		if err := db.Callback().Query().Before("gorm:query").Register("fail_receipt_read", func(tx *gorm.DB) {
			if tx.Statement.Table == "flow_command_outbox" {
				tx.AddError(readErr)
			}
		}); err != nil {
			t.Fatal(err)
		}
		return 202, nil, nil
	})
	t.Cleanup(func() { db.Callback().Query().Remove("fail_receipt_read") })
	if _, err := d.DispatchOne(context.Background(), row.ID); !errors.Is(err, readErr) {
		t.Fatalf("receipt read failure hidden: %v", err)
	}
}

func TestCommandOutboxTransactionalEnqueueAndDispatch(t *testing.T) {
	db := testDB(t)
	fe, client := newFakeEngine(t)
	scope, err := ResolveScope(db, "u_alice", "")
	if err != nil {
		t.Fatal(err)
	}
	fund(t, Billing{DB: db}, "u_alice", 50_000)

	now := time.Unix(1_800_000_000, 0).UTC()
	d := CommandDispatcher{
		DB:     db,
		Engine: client,
		Now:    func() time.Time { return now },
	}
	payload := []byte(`{"source_ids":["src_1"]}`)
	row, replayed, err := d.Enqueue(context.Background(), CommandInput{
		Scope:          scope,
		Actor:          Actor{Type: "user", ID: "u_alice"},
		Command:        "collection.request",
		Method:         http.MethodPost,
		Path:           "/v1/collection-runs",
		IdempotencyKey: "idem-collect-1",
		Payload:        payload,
		ReserveMicro:   1_000,
	})
	if err != nil || replayed {
		t.Fatalf("enqueue: err=%v replayed=%v", err, replayed)
	}
	if row.Status != "pending" || row.OperationID == "" || row.ReservationID == "" {
		t.Fatalf("unexpected outbox row: %+v", row)
	}

	// Verify initial operation projection and budget reservation were created in the same transaction.
	var proj model.FlowOperationProjection
	if err := db.First(&proj, "operation_id = ?", row.OperationID).Error; err != nil || proj.Status != "queued" {
		t.Fatalf("initial projection missing or wrong status: %+v err=%v", proj, err)
	}
	var rsv model.FlowBudgetReservation
	if err := db.First(&rsv, "id = ?", row.ReservationID).Error; err != nil || rsv.Status != "reserved" || rsv.ReservedMicro != 1_000 {
		t.Fatalf("reservation missing or wrong state: %+v err=%v", rsv, err)
	}

	// Dispatch the command to the Engine.
	dispatched, err := d.DispatchOne(context.Background(), row.ID)
	if err != nil {
		t.Fatalf("dispatch one: %v", err)
	}
	if dispatched.Status != "dispatched" || dispatched.ResponseStatus != 200 || dispatched.EngineOperationID != "op_idem-collect-1" {
		t.Fatalf("unexpected dispatched row: %+v", dispatched)
	}
	if fe.calls["POST /v1/collection-runs"] != 1 {
		t.Fatalf("want 1 engine call, got %d", fe.calls["POST /v1/collection-runs"])
	}

	// Idempotent replay with identical parameters returns the stored dispatched row without calling Engine again.
	replayRow, replayed2, err := d.EnqueueAndDispatch(context.Background(), CommandInput{
		Scope:          scope,
		Actor:          Actor{Type: "user", ID: "u_alice"},
		Command:        "collection.request",
		Method:         http.MethodPost,
		Path:           "/v1/collection-runs",
		IdempotencyKey: "idem-collect-1",
		Payload:        payload,
	})
	if err != nil || !replayed2 || replayRow.ID != row.ID || fe.calls["POST /v1/collection-runs"] != 1 {
		t.Fatalf("replay failed: err=%v replayed=%v calls=%d", err, replayed2, fe.calls["POST /v1/collection-runs"])
	}

	// Reusing the same idempotency key with different parameters returns ErrCommandConflict.
	_, _, err = d.Enqueue(context.Background(), CommandInput{
		Scope:          scope,
		Actor:          Actor{Type: "user", ID: "u_alice"},
		Command:        "collection.request",
		Method:         http.MethodPost,
		Path:           "/v1/collection-runs",
		IdempotencyKey: "idem-collect-1",
		Payload:        []byte(`{"source_ids":["src_2"]}`),
	})
	if !errors.Is(err, ErrCommandConflict) {
		t.Fatalf("want ErrCommandConflict, got %v", err)
	}
}

func TestCommandOutboxTransientFailureRetryAndReconciliation(t *testing.T) {
	db := testDB(t)
	scope, err := ResolveScope(db, "u_alice", "")
	if err != nil {
		t.Fatal(err)
	}
	fund(t, Billing{DB: db}, "u_alice", 50_000)

	var shouldFail atomic.Bool
	shouldFail.Store(true)
	var engineCalls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		engineCalls.Add(1)
		if shouldFail.Load() {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"code":502,"msg":"upstream unavailable"}`))
			return
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"code":0,"data":{"operation_id":"op_recovered","job_ids":["job_1"]},"msg":"accepted"}`))
	}))
	t.Cleanup(srv.Close)

	now := time.Unix(1_800_000_000, 0).UTC()
	client := EngineClient{Cfg: Config{EngineURL: srv.URL, SigningKey: key, Issuer: "flow-platform", Audience: "loom-engine", TTL: MaxTokenTTL}}
	d := CommandDispatcher{
		DB:      db,
		Engine:  client,
		Now:     func() time.Time { return now },
		Backoff: 5 * time.Second,
	}

	// 1. First attempt fails with 502 -> row enters retry_wait with backoff.
	row, _, err := d.EnqueueAndDispatch(context.Background(), CommandInput{
		Scope:          scope,
		Actor:          Actor{Type: "user", ID: "u_alice"},
		Command:        "collection.request",
		Method:         http.MethodPost,
		Path:           "/v1/collection-runs",
		IdempotencyKey: "idem-retry-1",
		Payload:        []byte(`{"source_ids":["src_1"]}`),
		MaxAttempts:    3,
	})
	if err == nil || row.Status != StatusRetryWait || row.Attempts != 1 {
		t.Fatalf("want retry_wait after 1st failure, got status=%s attempts=%d err=%v", row.Status, row.Attempts, err)
	}

	// Before backoff expires, DispatchDue does nothing.
	if n, err := d.DispatchDue(context.Background(), 10); err != nil || n != 0 {
		t.Fatalf("expected 0 dispatched before backoff, got %d err=%v", n, err)
	}

	// Advance clock past backoff and recover Engine -> DispatchDue delivers the command.
	now = now.Add(6 * time.Second)
	shouldFail.Store(false)
	if n, err := d.DispatchDue(context.Background(), 10); err != nil || n != 1 {
		t.Fatalf("expected 1 dispatched after backoff, got %d err=%v", n, err)
	}
	var updated model.FlowCommandOutbox
	db.First(&updated, "id = ?", row.ID)
	if updated.Status != "dispatched" || updated.EngineOperationID != "op_recovered" {
		t.Fatalf("expected dispatched after recovery, got %+v", updated)
	}

	// 2. Exhausting MaxAttempts transitions row and reservation to reconciliation_required (no blind resend).
	shouldFail.Store(true)
	row2, _, _ := d.Enqueue(context.Background(), CommandInput{
		Scope:          scope,
		Actor:          Actor{Type: "user", ID: "u_alice"},
		Command:        "collection.request",
		Method:         http.MethodPost,
		Path:           "/v1/collection-runs",
		IdempotencyKey: "idem-exhaust-1",
		Payload:        []byte(`{"source_ids":["src_1"]}`),
		ReserveMicro:   2_000,
		MaxAttempts:    2,
	})
	_, _ = d.DispatchOne(context.Background(), row2.ID)
	now = now.Add(60 * time.Second)
	exhausted, _ := d.DispatchOne(context.Background(), row2.ID)
	if exhausted.Status != StatusReconciliation || exhausted.Attempts != 2 {
		t.Fatalf("expected reconciliation_required after MaxAttempts, got %+v", exhausted)
	}
	var rsv2 model.FlowBudgetReservation
	db.First(&rsv2, "id = ?", row2.ReservationID)
	if rsv2.Status != StatusReconciliation {
		t.Fatalf("expected reservation in reconciliation_required, got %+v", rsv2)
	}
}

func TestCommandOutboxRESTAsyncAndConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testDB(t)
	fe, client := newFakeEngine(t)
	cfg := client.Cfg
	r := gin.New()
	RegisterRoutes(r.Group("/api"), db, cfg, func(*gin.Context) (*model.User, error) {
		return &model.User{ID: "u_alice"}, nil
	})

	// 1. Async outbox mode enqueues without calling Engine immediately and returns 202 Accepted.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/intelligence/collection-runs", strings.NewReader(`{"source_ids":["src_1"]}`))
	req.Header.Set("X-Idempotency-Key", "rest-async-1")
	req.Header.Set("X-Flow-Outbox-Mode", "async")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("async enqueue status=%d body=%s", w.Code, w.Body.String())
	}
	if fe.calls["POST /v1/collection-runs"] != 0 {
		t.Fatalf("async mode must not call engine synchronously, calls=%d", fe.calls["POST /v1/collection-runs"])
	}
	var env struct {
		Data struct {
			OperationID string `json:"operation_id"`
			OutboxID    string `json:"outbox_id"`
			Status      string `json:"status"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &env)
	if env.Data.OperationID == "" || env.Data.OutboxID == "" || env.Data.Status != "pending" {
		t.Fatalf("unexpected async response: %s", w.Body.String())
	}

	// Background dispatcher delivers the queued command to Engine.
	d := CommandDispatcher{DB: db, Engine: client}
	if n, err := d.DispatchDue(context.Background(), 10); err != nil || n != 1 {
		t.Fatalf("DispatchDue: n=%d err=%v", n, err)
	}
	if fe.calls["POST /v1/collection-runs"] != 1 {
		t.Fatalf("expected engine call after DispatchDue, calls=%d", fe.calls["POST /v1/collection-runs"])
	}

	// 2. Reusing the same idempotency key with different payload returns 409 Conflict before hitting Engine.
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/api/intelligence/collection-runs", strings.NewReader(`{"source_ids":["src_other"]}`))
	req2.Header.Set("X-Idempotency-Key", "rest-async-1")
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusConflict || fe.calls["POST /v1/collection-runs"] != 1 {
		t.Fatalf("conflict check failed: status=%d calls=%d", w2.Code, fe.calls["POST /v1/collection-runs"])
	}
}
