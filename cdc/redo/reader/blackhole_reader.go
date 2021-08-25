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
// See the License for the specific language governing permissions and
// limitations under the License.

package reader

import (
	"context"

	"github.com/pingcap/ticdc/cdc/model"
)

// BlackholeReader is a blockhole storage which implements LogReader interface
type BlackholeReader struct {
}

// NewBlackholeReader creates a new BlackholeReader
func NewBlackholeReader() *BlackholeReader {
	return &BlackholeReader{}
}

// ResetReader implements LogReader.ReadLog
func (br *BlackholeReader) ResetReader(ctx context.Context, startTs, endTs uint64) error {
	return nil
}

// ReadLog implements LogReader.ReadLog
func (br *BlackholeReader) ReadLog(ctx context.Context, maxNumberOfMessages int) ([]*model.RedoRowChangedEvent, error) {
	return nil, nil
}

// ReadDDL implements LogReader.ReadDDL
func (br *BlackholeReader) ReadDDL(ctx context.Context, maxNumberOfDDLs int) ([]*model.RedoDDLEvent, error) {
	return nil, nil
}

// ReadMeta implements LogReader.ReadMeta
func (br *BlackholeReader) ReadMeta(ctx context.Context) (resolvedTs, checkpointTs uint64, err error) {
	return 1, 0, nil
}