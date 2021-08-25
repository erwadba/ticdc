// Copyright 2021 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing pemissions and
// limitations under the License.

package applier

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/phayes/freeport"
	"github.com/pingcap/check"
	"github.com/pingcap/ticdc/cdc/model"
	"github.com/pingcap/ticdc/cdc/redo"
	"github.com/pingcap/ticdc/cdc/sink"
	"github.com/pingcap/ticdc/pkg/util/testleak"
)

func Test(t *testing.T) { check.TestingT(t) }

type redoApplierSuite struct{}

var _ = check.Suite(&redoApplierSuite{})

// MockReader is a mock redo log reader that implements LogReader interface
type MockReader struct {
	checkpointTs uint64
	resolvedTs   uint64
	redoLogCh    chan *redo.RowRedoLog
	ddlEventCh   chan *model.DDLEvent
}

// NewMockReader creates a new MockReader
func NewMockReader(
	checkpointTs uint64,
	resolvedTs uint64,
	redoLogCh chan *redo.RowRedoLog,
	ddlEventCh chan *model.DDLEvent,
) *MockReader {
	return &MockReader{
		checkpointTs: checkpointTs,
		resolvedTs:   resolvedTs,
		redoLogCh:    redoLogCh,
		ddlEventCh:   ddlEventCh,
	}
}

// ResetReader implements LogReader.ReadLog
func (br *MockReader) ResetReader(ctx context.Context, startTs, endTs uint64) error {
	return nil
}

// ReadLog implements LogReader.ReadLog
func (br *MockReader) ReadLog(ctx context.Context, maxNumberOfMessages int) ([]*redo.RowRedoLog, error) {
	cached := make([]*redo.RowRedoLog, 0)
	for {
		select {
		case <-ctx.Done():
			return cached, nil
		case redoLog, ok := <-br.redoLogCh:
			if !ok {
				return cached, nil
			}
			cached = append(cached, redoLog)
			if len(cached) >= maxNumberOfMessages {
				return cached, nil
			}
		}
	}
}

// ReadDDL implements LogReader.ReadDDL
func (br *MockReader) ReadDDL(ctx context.Context, maxNumberOfDDLs int) ([]*model.DDLEvent, error) {
	cached := make([]*model.DDLEvent, 0)
	for {
		select {
		case <-ctx.Done():
			return cached, nil
		case ddl, ok := <-br.ddlEventCh:
			if !ok {
				return cached, nil
			}
			cached = append(cached, ddl)
			if len(cached) >= maxNumberOfDDLs {
				return cached, nil
			}
		}
	}
}

// ReadMeta implements LogReader.ReadMeta
func (br *MockReader) ReadMeta(ctx context.Context) (resolvedTs, checkpointTs uint64, err error) {
	return br.resolvedTs, br.checkpointTs, nil
}

func (s *redoApplierSuite) TestApplyDMLs(c *check.C) {
	defer testleak.AfterTest(c)()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	checkpointTs := uint64(1000)
	resolvedTs := uint64(2000)
	redoLogCh := make(chan *redo.RowRedoLog, 1024)
	ddlEventCh := make(chan *model.DDLEvent, 1024)
	createMockReader := func(cfg *RedoApplierConfig) (redo.LogReader, error) {
		return NewMockReader(checkpointTs, resolvedTs, redoLogCh, ddlEventCh), nil
	}

	dbIndex := 0
	mockGetDBConn := func(ctx context.Context, dsnStr string) (*sql.DB, error) {
		defer func() {
			dbIndex++
		}()
		if dbIndex == 0 {
			// mock for test db, which is used querying TiDB session variable
			db, mock, err := sqlmock.New()
			if err != nil {
				return nil, err
			}
			columns := []string{"Variable_name", "Value"}
			mock.ExpectQuery("show session variables like 'allow_auto_random_explicit_insert';").WillReturnRows(
				sqlmock.NewRows(columns).AddRow("allow_auto_random_explicit_insert", "0"),
			)
			mock.ExpectQuery("show session variables like 'tidb_txn_mode';").WillReturnRows(
				sqlmock.NewRows(columns).AddRow("tidb_txn_mode", "pessimistic"),
			)
			mock.ExpectClose()
			return db, nil
		}
		// normal db
		db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
		c.Assert(err, check.IsNil)
		mock.ExpectBegin()
		mock.ExpectExec("REPLACE INTO `test`.`t1`(`a`,`b`) VALUES (?,?)").
			WithArgs(1, "2").
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		mock.ExpectBegin()
		mock.ExpectExec("DELETE FROM `test`.`t1` WHERE `a` = ? LIMIT 1;").
			WithArgs(1).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectExec("REPLACE INTO `test`.`t1`(`a`,`b`) VALUES (?,?)").
			WithArgs(2, "3").
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()
		mock.ExpectClose()
		return db, nil
	}

	getDBConnBak := sink.GetDBConnImpl
	sink.GetDBConnImpl = mockGetDBConn
	createRedoReaderBak := createRedoReader
	createRedoReader = createMockReader
	defer func() {
		createRedoReader = createRedoReaderBak
		sink.GetDBConnImpl = getDBConnBak
	}()

	dmls := []*redo.RowRedoLog{
		{
			Row: &model.RowChangedEvent{
				StartTs:  1100,
				CommitTs: 1200,
				Table:    &model.TableName{Schema: "test", Table: "t1"},
				Columns: []*model.Column{
					{
						Name:  "a",
						Value: 1,
						Flag:  model.HandleKeyFlag,
					}, {
						Name:  "b",
						Value: "2",
						Flag:  0,
					},
				},
			},
		},
		{
			Row: &model.RowChangedEvent{
				StartTs:  1200,
				CommitTs: 1300,
				Table:    &model.TableName{Schema: "test", Table: "t1"},
				PreColumns: []*model.Column{
					{
						Name:  "a",
						Value: 1,
						Flag:  model.HandleKeyFlag,
					}, {
						Name:  "b",
						Value: "2",
						Flag:  0,
					},
				},
				Columns: []*model.Column{
					{
						Name:  "a",
						Value: 2,
						Flag:  model.HandleKeyFlag,
					}, {
						Name:  "b",
						Value: "3",
						Flag:  0,
					},
				},
			},
		},
	}
	for _, dml := range dmls {
		redoLogCh <- dml
	}
	close(redoLogCh)
	close(ddlEventCh)

	cfg := &RedoApplierConfig{SinkURI: "mysql://127.0.0.1:4000/?worker-count=1&max-txn-row=1"}
	ap := NewRedoApplier(cfg)
	err := ap.Apply(ctx)
	c.Assert(err, check.IsNil)
}

func (s *redoApplierSuite) TestApplyMeetSinkError(c *check.C) {
	defer testleak.AfterTest(c)()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	port, err := freeport.GetFreePort()
	c.Assert(err, check.IsNil)
	cfg := &RedoApplierConfig{
		Storage: "blackhole",
		SinkURI: fmt.Sprintf("mysql://127.0.0.1:%d/?read-timeout=1s&timeout=1s", port),
	}
	ap := NewRedoApplier(cfg)
	err = ap.Apply(ctx)
	c.Assert(err, check.ErrorMatches, "fail to open MySQL connection:.*connect: connection refused")
}