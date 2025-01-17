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

package pipeline

import (
	"github.com/pingcap/check"
	"github.com/pingcap/ticdc/cdc/model"
	"github.com/pingcap/ticdc/cdc/puller/sorter"
	"github.com/pingcap/ticdc/pkg/config"
	cdcContext "github.com/pingcap/ticdc/pkg/context"
	"github.com/pingcap/ticdc/pkg/pipeline"
	"github.com/pingcap/ticdc/pkg/util/testleak"
)

type sorterSuite struct{}

var _ = check.Suite(&sorterSuite{})

func (s *sorterSuite) TestUnifiedSorterFileLockConflict(c *check.C) {
	defer testleak.AfterTest(c)()
	defer sorter.UnifiedSorterCleanUp()

	dir := c.MkDir()
	captureAddr := "0.0.0.0:0"

	// GlobalServerConfig overrides dir parameter in NewUnifiedSorter.
	config.GetGlobalServerConfig().Sorter.SortDir = dir
	_, err := sorter.NewUnifiedSorter(dir, "test-cf", "test", 0, captureAddr)
	c.Assert(err, check.IsNil)

	sorter.ResetGlobalPoolWithoutCleanup()
	ctx := cdcContext.NewBackendContext4Test(true)
	ctx.ChangefeedVars().Info.Engine = model.SortUnified
	ctx.ChangefeedVars().Info.SortDir = dir
	sorter := sorterNode{}
	err = sorter.Init(pipeline.MockNodeContext4Test(ctx, pipeline.Message{}, nil))
	c.Assert(err, check.ErrorMatches, ".*file lock conflict.*")
}
